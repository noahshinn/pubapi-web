package main

import (
	"encoding/json"
	"errors"
	"flag"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/google/uuid"
)

// Domain Models
type Profile struct {
	Email          string    `json:"email"`
	Name           string    `json:"name"`
	StartDate      time.Time `json:"start_date"`
	StartingWeight float64   `json:"starting_weight"`
	CurrentWeight  float64   `json:"current_weight"`
	GoalWeight     float64   `json:"goal_weight"`
	DailyPoints    int       `json:"daily_points"`
	WeeklyPoints   int       `json:"weekly_points"`
	Height         float64   `json:"height"`
	ActivityLevel  string    `json:"activity_level"`
}

type Food struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Points      int     `json:"points"`
	ServingSize string  `json:"serving_size"`
	Calories    int     `json:"calories"`
	Protein     float64 `json:"protein"`
	Carbs       float64 `json:"carbs"`
	Fat         float64 `json:"fat"`
}

type FoodLogEntry struct {
	ID         string    `json:"id"`
	UserEmail  string    `json:"user_email"`
	Food       Food      `json:"food"`
	Servings   float64   `json:"servings"`
	MealType   string    `json:"meal_type"`
	PointsUsed int       `json:"points_used"`
	Date       time.Time `json:"date"`
	Time       time.Time `json:"time"`
}

type WeightLogEntry struct {
	ID        string    `json:"id"`
	UserEmail string    `json:"user_email"`
	Weight    float64   `json:"weight"`
	Date      time.Time `json:"date"`
	Notes     string    `json:"notes"`
}

// Database represents our in-memory database
type Database struct {
	Profiles   map[string]Profile          `json:"profiles"`
	Foods      map[string]Food             `json:"foods"`
	FoodLogs   map[string][]FoodLogEntry   `json:"food_logs"`
	WeightLogs map[string][]WeightLogEntry `json:"weight_logs"`
	mu         sync.RWMutex
}

// Custom errors
var (
	ErrUserNotFound = errors.New("user not found")
	ErrFoodNotFound = errors.New("food not found")
	ErrInvalidInput = errors.New("invalid input")
)

// Global database instance
var db *Database

// Database operations
func (d *Database) GetProfile(email string) (Profile, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	profile, exists := d.Profiles[email]
	if !exists {
		return Profile{}, ErrUserNotFound
	}
	return profile, nil
}

func (d *Database) GetFood(id string) (Food, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	food, exists := d.Foods[id]
	if !exists {
		return Food{}, ErrFoodNotFound
	}
	return food, nil
}

func (d *Database) AddFoodLogEntry(entry FoodLogEntry) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.FoodLogs[entry.UserEmail] = append(d.FoodLogs[entry.UserEmail], entry)
	return nil
}

func (d *Database) AddWeightLogEntry(entry WeightLogEntry) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.WeightLogs[entry.UserEmail] = append(d.WeightLogs[entry.UserEmail], entry)

	// Update current weight in profile
	if profile, exists := d.Profiles[entry.UserEmail]; exists {
		profile.CurrentWeight = entry.Weight
		d.Profiles[entry.UserEmail] = profile
	}

	return nil
}

// HTTP Handlers
func getProfile(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	profile, err := db.GetProfile(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(profile)
}

func searchFoods(c *fiber.Ctx) error {
	query := c.Query("query")
	if query == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "query parameter is required",
		})
	}

	var matchingFoods []Food
	db.mu.RLock()
	for _, food := range db.Foods {
		// Simple case-insensitive substring search
		if strings.Contains(strings.ToLower(food.Name), strings.ToLower(query)) {
			matchingFoods = append(matchingFoods, food)
		}
	}
	db.mu.RUnlock()

	return c.JSON(matchingFoods)
}

func getFoodLog(c *fiber.Ctx) error {
	email := c.Query("email")
	dateStr := c.Query("date")

	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	db.mu.RLock()
	entries := db.FoodLogs[email]
	db.mu.RUnlock()

	if dateStr != "" {
		date, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "invalid date format",
			})
		}

		var filteredEntries []FoodLogEntry
		for _, entry := range entries {
			if entry.Date.Format("2006-01-02") == date.Format("2006-01-02") {
				filteredEntries = append(filteredEntries, entry)
			}
		}
		entries = filteredEntries
	}

	return c.JSON(entries)
}

type NewFoodLogEntryRequest struct {
	FoodID    string    `json:"food_id"`
	UserEmail string    `json:"user_email"`
	Servings  float64   `json:"servings"`
	MealType  string    `json:"meal_type"`
	Date      time.Time `json:"date"`
	Time      time.Time `json:"time"`
}

func addFoodLogEntry(c *fiber.Ctx) error {
	var req NewFoodLogEntryRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user
	if _, err := db.GetProfile(req.UserEmail); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Validate food
	food, err := db.GetFood(req.FoodID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	entry := FoodLogEntry{
		ID:         uuid.New().String(),
		UserEmail:  req.UserEmail,
		Food:       food,
		Servings:   req.Servings,
		MealType:   req.MealType,
		PointsUsed: int(float64(food.Points) * req.Servings),
		Date:       req.Date,
		Time:       req.Time,
	}

	if err := db.AddFoodLogEntry(entry); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to add food log entry",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(entry)
}

func getWeightLog(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	db.mu.RLock()
	entries := db.WeightLogs[email]
	db.mu.RUnlock()

	return c.JSON(entries)
}

type NewWeightLogEntryRequest struct {
	UserEmail string    `json:"user_email"`
	Weight    float64   `json:"weight"`
	Date      time.Time `json:"date"`
	Notes     string    `json:"notes"`
}

func addWeightLogEntry(c *fiber.Ctx) error {
	var req NewWeightLogEntryRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user
	if _, err := db.GetProfile(req.UserEmail); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	entry := WeightLogEntry{
		ID:        uuid.New().String(),
		UserEmail: req.UserEmail,
		Weight:    req.Weight,
		Date:      req.Date,
		Notes:     req.Notes,
	}

	if err := db.AddWeightLogEntry(entry); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to add weight log entry",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(entry)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Profiles:   make(map[string]Profile),
		Foods:      make(map[string]Food),
		FoodLogs:   make(map[string][]FoodLogEntry),
		WeightLogs: make(map[string][]WeightLogEntry),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Profile routes
	api.Get("/profile", getProfile)

	// Food routes
	api.Get("/foods/search", searchFoods)

	// Food log routes
	api.Get("/food-log", getFoodLog)
	api.Post("/food-log", addFoodLogEntry)

	// Weight log routes
	api.Get("/weight-log", getWeightLog)
	api.Post("/weight-log", addWeightLogEntry)
}

func main() {
	// Command line flags
	port := flag.String("port", "3000", "Port to run the server on")
	flag.Parse()

	if err := loadDatabase(); err != nil {
		log.Fatal(err)
	}

	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			return c.Status(code).JSON(fiber.Map{
				"error": err.Error(),
			})
		},
	})

	// Middleware
	app.Use(logger.New())
	app.Use(recover.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE",
		AllowHeaders: "Origin, Content-Type, Accept",
	}))

	// Setup routes
	setupRoutes(app)

	// Start server
	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"log"
	"os"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/google/uuid"
)

// Domain Models
type User struct {
	Email         string    `json:"email"`
	Name          string    `json:"name"`
	Height        float64   `json:"height"`
	DateOfBirth   string    `json:"date_of_birth"`
	Gender        string    `json:"gender"`
	ActivityLevel string    `json:"activity_level"`
	CreatedAt     time.Time `json:"created_at"`
}

type Food struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Brand       string  `json:"brand"`
	ServingSize string  `json:"serving_size"`
	Calories    int     `json:"calories"`
	Protein     float64 `json:"protein"`
	Carbs       float64 `json:"carbs"`
	Fat         float64 `json:"fat"`
	Fiber       float64 `json:"fiber"`
	Sugar       float64 `json:"sugar"`
	Sodium      float64 `json:"sodium"`
	CreatedBy   string  `json:"created_by"`
	IsVerified  bool    `json:"is_verified"`
}

type MealType string

const (
	MealTypeBreakfast MealType = "breakfast"
	MealTypeLunch     MealType = "lunch"
	MealTypeDinner    MealType = "dinner"
	MealTypeSnack     MealType = "snack"
)

type FoodEntry struct {
	ID        string    `json:"id"`
	UserEmail string    `json:"user_email"`
	FoodID    string    `json:"food_id"`
	Date      string    `json:"date"`
	MealType  MealType  `json:"meal_type"`
	Servings  float64   `json:"servings"`
	CreatedAt time.Time `json:"created_at"`
}

type ProgressEntry struct {
	ID           string   `json:"id"`
	UserEmail    string   `json:"user_email"`
	Date         string   `json:"date"`
	Weight       float64  `json:"weight"`
	BodyFat      *float64 `json:"body_fat,omitempty"`
	Measurements struct {
		Waist float64 `json:"waist,omitempty"`
		Chest float64 `json:"chest,omitempty"`
		Arms  float64 `json:"arms,omitempty"`
	} `json:"measurements"`
	CreatedAt time.Time `json:"created_at"`
}

type Goals struct {
	UserEmail     string  `json:"user_email"`
	TargetWeight  float64 `json:"target_weight"`
	WeeklyGoal    string  `json:"weekly_goal"` // e.g., "lose_0.5kg", "maintain", "gain_0.5kg"
	ActivityLevel string  `json:"activity_level"`
	DailyCalories int     `json:"daily_calories"`
	Macros        struct {
		Protein int `json:"protein"`
		Carbs   int `json:"carbs"`
		Fat     int `json:"fat"`
	} `json:"macros"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Database represents our in-memory database
type Database struct {
	Users           map[string]User            `json:"users"`
	Foods           map[string]Food            `json:"foods"`
	FoodEntries     map[string][]FoodEntry     `json:"food_entries"`     // Keyed by user_email
	ProgressEntries map[string][]ProgressEntry `json:"progress_entries"` // Keyed by user_email
	Goals           map[string]Goals           `json:"goals"`            // Keyed by user_email
	mu              sync.RWMutex
}

var db *Database

// Database operations
func (d *Database) GetUser(email string) (User, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	user, exists := d.Users[email]
	if !exists {
		return User{}, errors.New("user not found")
	}
	return user, nil
}

func (d *Database) GetFoodDiary(email, date string) ([]FoodEntry, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	entries := d.FoodEntries[email]
	var dayEntries []FoodEntry

	for _, entry := range entries {
		if entry.Date == date {
			dayEntries = append(dayEntries, entry)
		}
	}

	return dayEntries, nil
}

func (d *Database) AddFoodEntry(entry FoodEntry) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	entries := d.FoodEntries[entry.UserEmail]
	entries = append(entries, entry)
	d.FoodEntries[entry.UserEmail] = entries

	return nil
}

func (d *Database) SearchFoods(query string) []Food {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var results []Food
	for _, food := range d.Foods {
		// Simple contains search - in production, use proper search indexing
		if contains(food.Name, query) || contains(food.Brand, query) {
			results = append(results, food)
		}
	}
	return results
}

func contains(s, substr string) bool {
	// Case-insensitive contains implementation
	return true // Simplified for example
}

// HTTP Handlers
func getFoodDiary(c *fiber.Ctx) error {
	email := c.Query("email")
	date := c.Query("date")

	if email == "" || date == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email and date are required",
		})
	}

	entries, err := db.GetFoodDiary(email, date)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Calculate nutrition totals
	type MealTotals struct {
		Calories int         `json:"calories"`
		Protein  float64     `json:"protein"`
		Carbs    float64     `json:"carbs"`
		Fat      float64     `json:"fat"`
		Entries  []FoodEntry `json:"entries"`
	}

	meals := map[MealType]*MealTotals{
		MealTypeBreakfast: {Entries: []FoodEntry{}},
		MealTypeLunch:     {Entries: []FoodEntry{}},
		MealTypeDinner:    {Entries: []FoodEntry{}},
		MealTypeSnack:     {Entries: []FoodEntry{}},
	}

	for _, entry := range entries {
		food := db.Foods[entry.FoodID]
		mealTotals := meals[entry.MealType]

		multiplier := entry.Servings
		mealTotals.Calories += int(float64(food.Calories) * multiplier)
		mealTotals.Protein += food.Protein * multiplier
		mealTotals.Carbs += food.Carbs * multiplier
		mealTotals.Fat += food.Fat * multiplier
		mealTotals.Entries = append(mealTotals.Entries, entry)
	}

	return c.JSON(meals)
}

func addFoodEntry(c *fiber.Ctx) error {
	var entry FoodEntry
	if err := c.BodyParser(&entry); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user exists
	if _, err := db.GetUser(entry.UserEmail); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// Validate food exists
	if _, exists := db.Foods[entry.FoodID]; !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Food not found",
		})
	}

	entry.ID = uuid.New().String()
	entry.CreatedAt = time.Now()

	if err := db.AddFoodEntry(entry); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to add food entry",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(entry)
}

func searchFoods(c *fiber.Ctx) error {
	query := c.Query("query")
	if query == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "query parameter is required",
		})
	}

	results := db.SearchFoods(query)
	return c.JSON(results)
}

func getProgress(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	entries := db.ProgressEntries[email]
	if entries == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "No progress entries found",
		})
	}

	return c.JSON(entries)
}

func addProgress(c *fiber.Ctx) error {
	var entry ProgressEntry
	if err := c.BodyParser(&entry); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user exists
	if _, err := db.GetUser(entry.UserEmail); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	entry.ID = uuid.New().String()
	entry.CreatedAt = time.Now()

	db.mu.Lock()
	entries := db.ProgressEntries[entry.UserEmail]
	entries = append(entries, entry)
	db.ProgressEntries[entry.UserEmail] = entries
	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(entry)
}

func getGoals(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	db.mu.RLock()
	goals, exists := db.Goals[email]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Goals not found",
		})
	}

	return c.JSON(goals)
}

func updateGoals(c *fiber.Ctx) error {
	var goals Goals
	if err := c.BodyParser(&goals); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user exists
	if _, err := db.GetUser(goals.UserEmail); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	goals.UpdatedAt = time.Now()

	db.mu.Lock()
	db.Goals[goals.UserEmail] = goals
	db.mu.Unlock()

	return c.JSON(goals)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:           make(map[string]User),
		Foods:           make(map[string]Food),
		FoodEntries:     make(map[string][]FoodEntry),
		ProgressEntries: make(map[string][]ProgressEntry),
		Goals:           make(map[string]Goals),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Food diary routes
	api.Get("/food-diary", getFoodDiary)
	api.Post("/food-diary", addFoodEntry)

	// Food search routes
	api.Get("/foods/search", searchFoods)

	// Progress routes
	api.Get("/progress", getProgress)
	api.Post("/progress", addProgress)

	// Goals routes
	api.Get("/goals", getGoals)
	api.Put("/goals", updateGoals)
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
	app.Use(cors.New())

	// Setup routes
	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

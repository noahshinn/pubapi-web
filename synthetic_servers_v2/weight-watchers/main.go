package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
)

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
	FoodID    string    `json:"food_id"`
	UserEmail string    `json:"user_email"`
	Date      string    `json:"date"`
	MealType  string    `json:"meal_type"`
	Servings  float64   `json:"servings"`
	CreatedAt time.Time `json:"created_at"`
}

type DailyLog struct {
	Date            string                    `json:"date"`
	TotalPoints     int                       `json:"total_points"`
	PointsRemaining int                       `json:"points_remaining"`
	Meals           map[string][]FoodLogEntry `json:"meals"`
}

type WeightEntry struct {
	UserEmail string    `json:"user_email"`
	Weight    float64   `json:"weight"`
	Date      string    `json:"date"`
	CreatedAt time.Time `json:"created_at"`
}

type Progress struct {
	StartingWeight        float64       `json:"starting_weight"`
	CurrentWeight         float64       `json:"current_weight"`
	GoalWeight            float64       `json:"goal_weight"`
	WeeklyPointsAllowance int           `json:"weekly_points_allowance"`
	PointsUsedThisWeek    int           `json:"points_used_this_week"`
	WeightHistory         []WeightEntry `json:"weight_history"`
}

type User struct {
	Email              string    `json:"email"`
	Name               string    `json:"name"`
	StartDate          time.Time `json:"start_date"`
	Height             float64   `json:"height"`
	DailyPointsTarget  int       `json:"daily_points_target"`
	WeeklyPointsTarget int       `json:"weekly_points_target"`
	GoalWeight         float64   `json:"goal_weight"`
}

type Database struct {
	Users      map[string]User           `json:"users"`
	Foods      map[string]Food           `json:"foods"`
	FoodLogs   map[string][]FoodLogEntry `json:"food_logs"`
	WeightLogs map[string][]WeightEntry  `json:"weight_logs"`
	mu         sync.RWMutex
}

var db *Database

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:      make(map[string]User),
		Foods:      make(map[string]Food),
		FoodLogs:   make(map[string][]FoodLogEntry),
		WeightLogs: make(map[string][]WeightEntry),
	}

	return json.Unmarshal(data, db)
}

func getDailyLog(c *fiber.Ctx) error {
	email := c.Query("email")
	date := c.Query("date")

	if email == "" || date == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email and date are required",
		})
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	user, exists := db.Users[email]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "user not found",
		})
	}

	logs := db.FoodLogs[email]
	dailyLogs := make(map[string][]FoodLogEntry)
	totalPoints := 0

	for _, log := range logs {
		if log.Date == date {
			dailyLogs[log.MealType] = append(dailyLogs[log.MealType], log)
			food := db.Foods[log.FoodID]
			totalPoints += int(log.Servings) * food.Points
		}
	}

	return c.JSON(DailyLog{
		Date:            date,
		TotalPoints:     totalPoints,
		PointsRemaining: user.DailyPointsTarget - totalPoints,
		Meals:           dailyLogs,
	})
}

func addFoodLogEntry(c *fiber.Ctx) error {
	var entry FoodLogEntry
	if err := c.BodyParser(&entry); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	if _, exists := db.Users[entry.UserEmail]; !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "user not found",
		})
	}

	if _, exists := db.Foods[entry.FoodID]; !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "food not found",
		})
	}

	entry.CreatedAt = time.Now()
	db.FoodLogs[entry.UserEmail] = append(db.FoodLogs[entry.UserEmail], entry)

	return c.Status(fiber.StatusCreated).JSON(entry)
}

func getProgress(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	user, exists := db.Users[email]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "user not found",
		})
	}

	weightLogs := db.WeightLogs[email]
	var currentWeight float64
	if len(weightLogs) > 0 {
		currentWeight = weightLogs[len(weightLogs)-1].Weight
	}

	// Calculate points used this week
	now := time.Now()
	weekStart := now.AddDate(0, 0, -int(now.Weekday()))
	pointsUsed := 0
	for _, log := range db.FoodLogs[email] {
		logDate, _ := time.Parse("2006-01-02", log.Date)
		if logDate.After(weekStart) {
			food := db.Foods[log.FoodID]
			pointsUsed += int(log.Servings) * food.Points
		}
	}

	progress := Progress{
		StartingWeight:        weightLogs[0].Weight,
		CurrentWeight:         currentWeight,
		GoalWeight:            user.GoalWeight,
		WeeklyPointsAllowance: user.WeeklyPointsTarget,
		PointsUsedThisWeek:    pointsUsed,
		WeightHistory:         weightLogs,
	}

	return c.JSON(progress)
}

func addWeightEntry(c *fiber.Ctx) error {
	var entry WeightEntry
	if err := c.BodyParser(&entry); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	if _, exists := db.Users[entry.UserEmail]; !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "user not found",
		})
	}

	entry.CreatedAt = time.Now()
	db.WeightLogs[entry.UserEmail] = append(db.WeightLogs[entry.UserEmail], entry)

	return c.Status(fiber.StatusCreated).JSON(entry)
}

func searchFoods(c *fiber.Ctx) error {
	query := c.Query("query")
	if query == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "query parameter is required",
		})
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	var results []Food
	for _, food := range db.Foods {
		// Simple substring search (could be improved with proper search algorithm)
		if contains(food.Name, query) {
			results = append(results, food)
		}
	}

	return c.JSON(results)
}

func contains(s, substr string) bool {
	for i := 0; i < len(s); i++ {
		if hasPrefix(s[i:], substr) {
			return true
		}
	}
	return false
}

func hasPrefix(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		if s[i] != prefix[i] {
			return false
		}
	}
	return true
}

func setupRoutes(app *fiber.App) {
	// Serve OpenAPI spec at root
	apiSpec, err := os.ReadFile("api_spec.json")
	if err != nil {
		log.Fatal(err)
	}

	var spec map[string]interface{}
	if err := json.Unmarshal(apiSpec, &spec); err != nil {
		log.Fatal(err)
	}

	app.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(spec)
	})

	api := app.Group("/api/v1")

	// Daily log routes
	api.Get("/daily-log", getDailyLog)
	api.Post("/daily-log", addFoodLogEntry)

	// Progress routes
	api.Get("/progress", getProgress)
	api.Post("/progress", addWeightEntry)

	// Food search route
	api.Get("/foods/search", searchFoods)
}

func main() {
	port := flag.String("port", "3000", "Port to run the server on")
	flag.Parse()

	if err := loadDatabase(); err != nil {
		log.Fatal(err)
	}

	app := fiber.New()

	app.Use(logger.New())
	app.Use(cors.New())

	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

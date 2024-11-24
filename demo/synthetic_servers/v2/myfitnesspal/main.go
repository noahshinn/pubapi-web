package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/google/uuid"
)

// Models
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
}

type DiaryEntry struct {
	ID        string    `json:"id"`
	UserEmail string    `json:"user_email"`
	Food      Food      `json:"food"`
	MealType  string    `json:"meal_type"`
	Servings  float64   `json:"servings"`
	Date      time.Time `json:"date"`
	Notes     string    `json:"notes"`
}

type NutritionTotals struct {
	Calories int     `json:"calories"`
	Protein  float64 `json:"protein"`
	Carbs    float64 `json:"carbs"`
	Fat      float64 `json:"fat"`
}

type DiaryDay struct {
	Date    string                  `json:"date"`
	Entries map[string][]DiaryEntry `json:"entries"`
	Totals  NutritionTotals         `json:"totals"`
}

type WeightEntry struct {
	ID        string    `json:"id"`
	UserEmail string    `json:"user_email"`
	Weight    float64   `json:"weight"`
	Date      time.Time `json:"date"`
}

type User struct {
	Email         string    `json:"email"`
	Name          string    `json:"name"`
	Height        float64   `json:"height"`
	Gender        string    `json:"gender"`
	DateOfBirth   time.Time `json:"date_of_birth"`
	GoalWeight    float64   `json:"goal_weight"`
	ActivityLevel string    `json:"activity_level"`
	DailyCalGoal  int       `json:"daily_cal_goal"`
	MacroGoals    struct {
		Protein float64 `json:"protein"`
		Carbs   float64 `json:"carbs"`
		Fat     float64 `json:"fat"`
	} `json:"macro_goals"`
}

// Database
type Database struct {
	Users         map[string]User          `json:"users"`
	Foods         map[string]Food          `json:"foods"`
	DiaryEntries  map[string][]DiaryEntry  `json:"diary_entries"`
	WeightEntries map[string][]WeightEntry `json:"weight_entries"`
	mu            sync.RWMutex
}

var db *Database

// Database operations
func (d *Database) GetUser(email string) (User, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	user, exists := d.Users[email]
	if !exists {
		return User{}, fmt.Errorf("user not found")
	}
	return user, nil
}

func (d *Database) SearchFoods(query string) []Food {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var results []Food
	for _, food := range d.Foods {
		// Simple case-insensitive substring search
		if contains(food.Name, query) || contains(food.Brand, query) {
			results = append(results, food)
		}
	}
	return results
}

func contains(s, substr string) bool {
	return true // Implement proper string search
}

func (d *Database) GetDiaryEntries(email string, date time.Time) []DiaryEntry {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var entries []DiaryEntry
	allEntries := d.DiaryEntries[email]
	for _, entry := range allEntries {
		if isSameDate(entry.Date, date) {
			entries = append(entries, entry)
		}
	}
	return entries
}

func (d *Database) AddDiaryEntry(entry DiaryEntry) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	entries := d.DiaryEntries[entry.UserEmail]
	entries = append(entries, entry)
	d.DiaryEntries[entry.UserEmail] = entries
	return nil
}

func (d *Database) AddWeightEntry(entry WeightEntry) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	entries := d.WeightEntries[entry.UserEmail]
	entries = append(entries, entry)
	d.WeightEntries[entry.UserEmail] = entries
	return nil
}

func (d *Database) GetWeightEntries(email string, startDate, endDate time.Time) []WeightEntry {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var entries []WeightEntry
	allEntries := d.WeightEntries[email]
	for _, entry := range allEntries {
		if (entry.Date.After(startDate) || entry.Date.Equal(startDate)) &&
			(entry.Date.Before(endDate) || entry.Date.Equal(endDate)) {
			entries = append(entries, entry)
		}
	}
	return entries
}

// Handlers
func getDiaryHandler(c *fiber.Ctx) error {
	date, err := time.Parse("2006-01-02", c.Params("date"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid date format",
		})
	}

	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email is required",
		})
	}

	entries := db.GetDiaryEntries(email, date)

	// Organize entries by meal type
	diaryDay := DiaryDay{
		Date:    date.Format("2006-01-02"),
		Entries: make(map[string][]DiaryEntry),
		Totals:  NutritionTotals{},
	}

	for _, entry := range entries {
		diaryDay.Entries[entry.MealType] = append(diaryDay.Entries[entry.MealType], entry)

		// Calculate totals
		diaryDay.Totals.Calories += int(float64(entry.Food.Calories) * entry.Servings)
		diaryDay.Totals.Protein += entry.Food.Protein * entry.Servings
		diaryDay.Totals.Carbs += entry.Food.Carbs * entry.Servings
		diaryDay.Totals.Fat += entry.Food.Fat * entry.Servings
	}

	return c.JSON(diaryDay)
}

func addDiaryEntryHandler(c *fiber.Ctx) error {
	var req struct {
		FoodID    string  `json:"food_id"`
		MealType  string  `json:"meal_type"`
		Servings  float64 `json:"servings"`
		Date      string  `json:"date"`
		UserEmail string  `json:"user_email"`
		Notes     string  `json:"notes"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	date, err := time.Parse("2006-01-02", req.Date)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid date format",
		})
	}

	food, exists := db.Foods[req.FoodID]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Food not found",
		})
	}

	entry := DiaryEntry{
		ID:        uuid.New().String(),
		UserEmail: req.UserEmail,
		Food:      food,
		MealType:  req.MealType,
		Servings:  req.Servings,
		Date:      date,
		Notes:     req.Notes,
	}

	if err := db.AddDiaryEntry(entry); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to add diary entry",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(entry)
}

func searchFoodsHandler(c *fiber.Ctx) error {
	query := c.Query("query")
	if query == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Search query is required",
		})
	}

	foods := db.SearchFoods(query)
	return c.JSON(foods)
}

func getProgressHandler(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email is required",
		})
	}

	startDate, err := time.Parse("2006-01-02", c.Query("start_date"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid start date format",
		})
	}

	endDate, err := time.Parse("2006-01-02", c.Query("end_date"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid end date format",
		})
	}

	weightEntries := db.GetWeightEntries(email, startDate, endDate)

	// Calculate nutrition averages
	var totalCals, totalProtein, totalCarbs, totalFat float64
	days := 0
	currentDate := startDate

	for !currentDate.After(endDate) {
		entries := db.GetDiaryEntries(email, currentDate)
		if len(entries) > 0 {
			days++
			for _, entry := range entries {
				totalCals += float64(entry.Food.Calories) * entry.Servings
				totalProtein += entry.Food.Protein * entry.Servings
				totalCarbs += entry.Food.Carbs * entry.Servings
				totalFat += entry.Food.Fat * entry.Servings
			}
		}
		currentDate = currentDate.AddDate(0, 0, 1)
	}

	var averages struct {
		Calories float64 `json:"calories"`
		Protein  float64 `json:"protein"`
		Carbs    float64 `json:"carbs"`
		Fat      float64 `json:"fat"`
	}

	if days > 0 {
		averages.Calories = totalCals / float64(days)
		averages.Protein = totalProtein / float64(days)
		averages.Carbs = totalCarbs / float64(days)
		averages.Fat = totalFat / float64(days)
	}

	return c.JSON(fiber.Map{
		"weight_entries":     weightEntries,
		"nutrition_averages": averages,
	})
}

func logWeightHandler(c *fiber.Ctx) error {
	var req WeightEntry
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	req.ID = uuid.New().String()

	if err := db.AddWeightEntry(req); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to log weight entry",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(req)
}

func isSameDate(date1, date2 time.Time) bool {
	y1, m1, d1 := date1.Date()
	y2, m2, d2 := date2.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:         make(map[string]User),
		Foods:         make(map[string]Food),
		DiaryEntries:  make(map[string][]DiaryEntry),
		WeightEntries: make(map[string][]WeightEntry),
	}

	return json.Unmarshal(data, db)
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

	// API routes
	api := app.Group("/api/v1")
	api.Get("/diary/:date", getDiaryHandler)
	api.Post("/diary/entries", addDiaryEntryHandler)
	api.Get("/foods/search", searchFoodsHandler)
	api.Get("/progress", getProgressHandler)
	api.Post("/weight", logWeightHandler)

	log.Printf("Server starting on port %s", *port)
	log.Fatal(app.Listen(":" + *port))
}

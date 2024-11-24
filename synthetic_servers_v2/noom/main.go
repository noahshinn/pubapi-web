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
	"github.com/google/uuid"
)

// Data models
type FoodItem struct {
	Name          string  `json:"name"`
	Portion       float64 `json:"portion"`
	Unit          string  `json:"unit"`
	Calories      float64 `json:"calories"`
	ColorCategory string  `json:"color_category"` // green, yellow, or red
}

type MealLog struct {
	ID            string     `json:"id"`
	UserEmail     string     `json:"user_email"`
	MealType      string     `json:"meal_type"` // breakfast, lunch, dinner, snack
	Foods         []FoodItem `json:"foods"`
	TotalCalories float64    `json:"total_calories"`
	ColorCategory string     `json:"color_category"`
	Timestamp     time.Time  `json:"timestamp"`
	Notes         string     `json:"notes"`
}

type WeightLog struct {
	Weight float64   `json:"weight"`
	Date   time.Time `json:"date"`
	Notes  string    `json:"notes"`
}

type Goals struct {
	TargetWeight float64   `json:"target_weight"`
	WeeklyGoal   float64   `json:"weekly_goal"`
	TargetDate   time.Time `json:"target_date"`
}

type Progress struct {
	UserEmail       string      `json:"user_email"`
	WeightLogs      []WeightLog `json:"weight_logs"`
	Goals           Goals       `json:"goals"`
	StreakDays      int         `json:"streak_days"`
	TotalWeightLoss float64     `json:"total_weight_loss"`
}

type CoachingMessage struct {
	ID          string    `json:"id"`
	UserEmail   string    `json:"user_email"`
	CoachID     string    `json:"coach_id"`
	Content     string    `json:"content"`
	Timestamp   time.Time `json:"timestamp"`
	IsFromCoach bool      `json:"is_from_coach"`
	Read        bool      `json:"read"`
}

type User struct {
	Email            string    `json:"email"`
	Name             string    `json:"name"`
	StartDate        time.Time `json:"start_date"`
	StartingWeight   float64   `json:"starting_weight"`
	CurrentWeight    float64   `json:"current_weight"`
	CoachID          string    `json:"coach_id"`
	SubscriptionTier string    `json:"subscription_tier"`
}

// Database struct
type Database struct {
	Users    map[string]User              `json:"users"`
	MealLogs map[string][]MealLog         `json:"meal_logs"` // key: user_email
	Progress map[string]Progress          `json:"progress"`  // key: user_email
	Messages map[string][]CoachingMessage `json:"messages"`  // key: user_email
	mu       sync.RWMutex
}

var db *Database

// Handlers
func getMealLogs(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	dateStr := c.Query("date")
	var targetDate time.Time
	var err error
	if dateStr != "" {
		targetDate, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "invalid date format",
			})
		}
	}

	db.mu.RLock()
	logs := db.MealLogs[email]
	db.mu.RUnlock()

	if dateStr != "" {
		var filteredLogs []MealLog
		for _, log := range logs {
			if log.Timestamp.Format("2006-01-02") == targetDate.Format("2006-01-02") {
				filteredLogs = append(filteredLogs, log)
			}
		}
		logs = filteredLogs
	}

	return c.JSON(logs)
}

func logMeal(c *fiber.Ctx) error {
	var newMeal MealLog
	if err := c.BodyParser(&newMeal); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	newMeal.ID = uuid.New().String()
	newMeal.Timestamp = time.Now()

	// Calculate total calories and determine color category
	var totalCals float64
	var greenCount, yellowCount, redCount int
	for _, food := range newMeal.Foods {
		totalCals += food.Calories
		switch food.ColorCategory {
		case "green":
			greenCount++
		case "yellow":
			yellowCount++
		case "red":
			redCount++
		}
	}
	newMeal.TotalCalories = totalCals

	// Determine overall meal color category
	if redCount > yellowCount && redCount > greenCount {
		newMeal.ColorCategory = "red"
	} else if yellowCount > greenCount {
		newMeal.ColorCategory = "yellow"
	} else {
		newMeal.ColorCategory = "green"
	}

	db.mu.Lock()
	db.MealLogs[newMeal.UserEmail] = append(db.MealLogs[newMeal.UserEmail], newMeal)
	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(newMeal)
}

func getProgress(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	db.mu.RLock()
	progress, exists := db.Progress[email]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "user progress not found",
		})
	}

	return c.JSON(progress)
}

func updateProgress(c *fiber.Ctx) error {
	var update struct {
		UserEmail string  `json:"user_email"`
		Weight    float64 `json:"weight"`
		Notes     string  `json:"notes"`
	}

	if err := c.BodyParser(&update); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	progress, exists := db.Progress[update.UserEmail]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "user progress not found",
		})
	}

	// Add new weight log
	newLog := WeightLog{
		Weight: update.Weight,
		Date:   time.Now(),
		Notes:  update.Notes,
	}
	progress.WeightLogs = append(progress.WeightLogs, newLog)

	// Update total weight loss
	if len(progress.WeightLogs) > 0 {
		startWeight := progress.WeightLogs[0].Weight
		progress.TotalWeightLoss = startWeight - update.Weight
	}

	// Update streak days
	lastLog := progress.WeightLogs[len(progress.WeightLogs)-2]
	if newLog.Date.Sub(lastLog.Date).Hours() <= 24 {
		progress.StreakDays++
	} else {
		progress.StreakDays = 1
	}

	db.Progress[update.UserEmail] = progress

	return c.JSON(progress)
}

func getCoachingMessages(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	db.mu.RLock()
	messages := db.Messages[email]
	db.mu.RUnlock()

	return c.JSON(messages)
}

func sendMessage(c *fiber.Ctx) error {
	var newMsg struct {
		UserEmail string `json:"user_email"`
		Content   string `json:"content"`
	}

	if err := c.BodyParser(&newMsg); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	message := CoachingMessage{
		ID:          uuid.New().String(),
		UserEmail:   newMsg.UserEmail,
		Content:     newMsg.Content,
		Timestamp:   time.Now(),
		IsFromCoach: false,
		Read:        false,
	}

	db.mu.Lock()
	// Get user's coach ID
	user, exists := db.Users[newMsg.UserEmail]
	if !exists {
		db.mu.Unlock()
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "user not found",
		})
	}
	message.CoachID = user.CoachID

	db.Messages[newMsg.UserEmail] = append(db.Messages[newMsg.UserEmail], message)
	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(message)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:    make(map[string]User),
		MealLogs: make(map[string][]MealLog),
		Progress: make(map[string]Progress),
		Messages: make(map[string][]CoachingMessage),
	}

	return json.Unmarshal(data, db)
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

	// Meal logging routes
	api.Get("/meals", getMealLogs)
	api.Post("/meals", logMeal)

	// Progress tracking routes
	api.Get("/progress", getProgress)
	api.Post("/progress", updateProgress)

	// Coaching message routes
	api.Get("/coaching/messages", getCoachingMessages)
	api.Post("/coaching/messages", sendMessage)
}

func main() {
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

	app.Use(logger.New())
	app.Use(cors.New())

	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

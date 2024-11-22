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
	"github.com/google/uuid"
)

// Domain Models
type User struct {
	Email           string    `json:"email"`
	Name            string    `json:"name"`
	StartWeight     float64   `json:"start_weight"`
	GoalWeight      float64   `json:"goal_weight"`
	Height          float64   `json:"height"`
	Birthday        string    `json:"birthday"`
	Gender          string    `json:"gender"`
	ActivityLevel   string    `json:"activity_level"`
	DietPreferences []string  `json:"diet_preferences"`
	JoinedAt        time.Time `json:"joined_at"`
	CoachID         string    `json:"coach_id"`
}

type FoodItem struct {
	Name     string  `json:"name"`
	Portion  float64 `json:"portion"`
	Unit     string  `json:"unit"`
	Calories float64 `json:"calories"`
	Protein  float64 `json:"protein"`
	Carbs    float64 `json:"carbs"`
	Fat      float64 `json:"fat"`
}

type MealLog struct {
	ID            string     `json:"id"`
	UserEmail     string     `json:"user_email"`
	MealType      string     `json:"meal_type"`
	Foods         []FoodItem `json:"foods"`
	TotalCalories float64    `json:"total_calories"`
	LoggedAt      time.Time  `json:"logged_at"`
}

type WeightLog struct {
	ID        string    `json:"id"`
	UserEmail string    `json:"user_email"`
	Weight    float64   `json:"weight"`
	LoggedAt  time.Time `json:"logged_at"`
}

type Coach struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Specialties []string `json:"specialties"`
}

type CoachingMessage struct {
	ID          string    `json:"id"`
	UserEmail   string    `json:"user_email"`
	CoachID     string    `json:"coach_id"`
	Content     string    `json:"content"`
	SentAt      time.Time `json:"sent_at"`
	IsFromCoach bool      `json:"is_from_coach"`
}

// Database represents our in-memory database
type Database struct {
	Users            map[string]User              `json:"users"`
	MealLogs         map[string][]MealLog         `json:"meal_logs"`
	WeightLogs       map[string][]WeightLog       `json:"weight_logs"`
	Coaches          map[string]Coach             `json:"coaches"`
	CoachingMessages map[string][]CoachingMessage `json:"coaching_messages"`
	mu               sync.RWMutex
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

func (d *Database) GetMealLogs(email string, date time.Time) []MealLog {
	d.mu.RLock()
	defer d.mu.RUnlock()

	logs := d.MealLogs[email]
	if date.IsZero() {
		return logs
	}

	var filteredLogs []MealLog
	for _, log := range logs {
		if log.LoggedAt.Format("2006-01-02") == date.Format("2006-01-02") {
			filteredLogs = append(filteredLogs, log)
		}
	}
	return filteredLogs
}

func (d *Database) AddMealLog(log MealLog) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.Users[log.UserEmail]; !exists {
		return errors.New("user not found")
	}

	d.MealLogs[log.UserEmail] = append(d.MealLogs[log.UserEmail], log)
	return nil
}

func (d *Database) GetWeightLogs(email string) []WeightLog {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.WeightLogs[email]
}

func (d *Database) AddWeightLog(log WeightLog) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.Users[log.UserEmail]; !exists {
		return errors.New("user not found")
	}

	d.WeightLogs[log.UserEmail] = append(d.WeightLogs[log.UserEmail], log)
	return nil
}

func (d *Database) GetCoachingMessages(email string) []CoachingMessage {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.CoachingMessages[email]
}

func (d *Database) AddCoachingMessage(message CoachingMessage) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.Users[message.UserEmail]; !exists {
		return errors.New("user not found")
	}

	d.CoachingMessages[message.UserEmail] = append(d.CoachingMessages[message.UserEmail], message)
	return nil
}

// HTTP Handlers
func getMealLogs(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	dateStr := c.Query("date")
	var date time.Time
	var err error
	if dateStr != "" {
		date, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "invalid date format",
			})
		}
	}

	logs := db.GetMealLogs(email, date)
	return c.JSON(logs)
}

func addMealLog(c *fiber.Ctx) error {
	var log MealLog
	if err := c.BodyParser(&log); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	log.ID = uuid.New().String()
	log.LoggedAt = time.Now()

	// Calculate total calories
	for _, food := range log.Foods {
		log.TotalCalories += food.Calories
	}

	if err := db.AddMealLog(log); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(log)
}

func getWeightLogs(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	logs := db.GetWeightLogs(email)
	return c.JSON(logs)
}

func addWeightLog(c *fiber.Ctx) error {
	var log WeightLog
	if err := c.BodyParser(&log); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	log.ID = uuid.New().String()
	log.LoggedAt = time.Now()

	if err := db.AddWeightLog(log); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(log)
}

func getCoachingMessages(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	messages := db.GetCoachingMessages(email)
	return c.JSON(messages)
}

func sendCoachingMessage(c *fiber.Ctx) error {
	var message CoachingMessage
	if err := c.BodyParser(&message); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	message.ID = uuid.New().String()
	message.SentAt = time.Now()

	if err := db.AddCoachingMessage(message); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(message)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:            make(map[string]User),
		MealLogs:         make(map[string][]MealLog),
		WeightLogs:       make(map[string][]WeightLog),
		Coaches:          make(map[string]Coach),
		CoachingMessages: make(map[string][]CoachingMessage),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Meal logging routes
	api.Get("/meals", getMealLogs)
	api.Post("/meals", addMealLog)

	// Weight logging routes
	api.Get("/weight-logs", getWeightLogs)
	api.Post("/weight-logs", addWeightLog)

	// Coaching routes
	api.Get("/coaching/messages", getCoachingMessages)
	api.Post("/coaching/messages", sendCoachingMessage)

	// User routes
	api.Get("/users/:email", func(c *fiber.Ctx) error {
		email := c.Params("email")
		user, err := db.GetUser(email)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.JSON(user)
	})
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
	app.Use(cors.New())

	// Setup routes
	setupRoutes(app)

	// Start server
	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

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

type Location struct {
	Street    string  `json:"street"`
	City      string  `json:"city"`
	State     string  `json:"state"`
	ZipCode   string  `json:"zip_code"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type Category struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	BaseRate    float64 `json:"base_rate"`
	IconURL     string  `json:"icon_url"`
}

type Tasker struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	PhotoURL     string   `json:"photo_url"`
	Rating       float64  `json:"rating"`
	ReviewsCount int      `json:"reviews_count"`
	HourlyRate   float64  `json:"hourly_rate"`
	Skills       []string `json:"skills"`
	Availability []string `json:"availability"`
}

type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusAssigned   TaskStatus = "assigned"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusCancelled  TaskStatus = "cancelled"
)

type Task struct {
	ID            string     `json:"id"`
	UserEmail     string     `json:"user_email"`
	Category      Category   `json:"category"`
	Description   string     `json:"description"`
	Status        TaskStatus `json:"status"`
	Date          time.Time  `json:"date"`
	DurationHours float64    `json:"duration_hours"`
	Location      Location   `json:"location"`
	Tasker        *Tasker    `json:"tasker,omitempty"`
	TotalCost     float64    `json:"total_cost"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type Database struct {
	Categories map[string]Category `json:"categories"`
	Taskers    map[string]Tasker   `json:"taskers"`
	Tasks      map[string]Task     `json:"tasks"`
	mu         sync.RWMutex
}

var db *Database

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Categories: make(map[string]Category),
		Taskers:    make(map[string]Tasker),
		Tasks:      make(map[string]Task),
	}

	return json.Unmarshal(data, db)
}

func getCategories(c *fiber.Ctx) error {
	db.mu.RLock()
	categories := make([]Category, 0, len(db.Categories))
	for _, category := range db.Categories {
		categories = append(categories, category)
	}
	db.mu.RUnlock()

	return c.JSON(categories)
}

func getTaskers(c *fiber.Ctx) error {
	categoryID := c.Query("category")
	date := c.Query("date")
	zipCode := c.Query("zip_code")

	if categoryID == "" || date == "" || zipCode == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "category, date, and zip_code are required",
		})
	}

	taskDate, err := time.Parse("2006-01-02", date)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid date format",
		})
	}

	db.mu.RLock()
	var availableTaskers []Tasker
	for _, tasker := range db.Taskers {
		// Check if tasker has the required skill
		hasSkill := false
		for _, skill := range tasker.Skills {
			if skill == categoryID {
				hasSkill = true
				break
			}
		}

		if !hasSkill {
			continue
		}

		// Check availability
		isAvailable := false
		for _, availableDate := range tasker.Availability {
			available, _ := time.Parse(time.RFC3339, availableDate)
			if available.Format("2006-01-02") == taskDate.Format("2006-01-02") {
				isAvailable = true
				break
			}
		}

		if isAvailable {
			availableTaskers = append(availableTaskers, tasker)
		}
	}
	db.mu.RUnlock()

	return c.JSON(availableTaskers)
}

func getUserTasks(c *fiber.Ctx) error {
	email := c.Query("email")
	status := c.Query("status")

	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	db.mu.RLock()
	var userTasks []Task
	for _, task := range db.Tasks {
		if task.UserEmail == email {
			if status == "" || string(task.Status) == status {
				userTasks = append(userTasks, task)
			}
		}
	}
	db.mu.RUnlock()

	return c.JSON(userTasks)
}

type CreateTaskRequest struct {
	CategoryID    string   `json:"category_id"`
	Description   string   `json:"description"`
	Date          string   `json:"date"`
	DurationHours float64  `json:"duration_hours"`
	Location      Location `json:"location"`
	TaskerID      string   `json:"tasker_id"`
	UserEmail     string   `json:"user_email"`
}

func createTask(c *fiber.Ctx) error {
	var req CreateTaskRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate required fields
	if req.CategoryID == "" || req.Description == "" || req.Date == "" || req.DurationHours <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Missing required fields",
		})
	}

	db.mu.RLock()
	category, exists := db.Categories[req.CategoryID]
	if !exists {
		db.mu.RUnlock()
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid category",
		})
	}

	var tasker *Tasker
	if req.TaskerID != "" {
		if t, exists := db.Taskers[req.TaskerID]; exists {
			tasker = &t
		}
	}
	db.mu.RUnlock()

	taskDate, err := time.Parse("2006-01-02T15:04:05Z", req.Date)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid date format",
		})
	}

	// Calculate total cost
	var totalCost float64
	if tasker != nil {
		totalCost = tasker.HourlyRate * req.DurationHours
	} else {
		totalCost = category.BaseRate * req.DurationHours
	}

	task := Task{
		ID:            uuid.New().String(),
		UserEmail:     req.UserEmail,
		Category:      category,
		Description:   req.Description,
		Status:        TaskStatusPending,
		Date:          taskDate,
		DurationHours: req.DurationHours,
		Location:      req.Location,
		Tasker:        tasker,
		TotalCost:     totalCost,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	db.mu.Lock()
	db.Tasks[task.ID] = task
	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(task)
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

	api.Get("/categories", getCategories)
	api.Get("/taskers", getTaskers)
	api.Get("/tasks", getUserTasks)
	api.Post("/tasks", createTask)
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

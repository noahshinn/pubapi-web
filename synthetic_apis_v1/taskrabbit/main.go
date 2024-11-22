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
type Location struct {
	Address   string  `json:"address"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type User struct {
	Email    string    `json:"email"`
	Name     string    `json:"name"`
	Phone    string    `json:"phone"`
	Location Location  `json:"location"`
	JoinedAt time.Time `json:"joined_at"`
}

type Category struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	BaseRate    float64 `json:"base_rate"`
}

type Tasker struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Categories     []string  `json:"categories"`
	HourlyRate     float64   `json:"hourly_rate"`
	Rating         float64   `json:"rating"`
	ReviewsCount   int       `json:"reviews_count"`
	CompletedTasks int       `json:"completed_tasks"`
	Location       Location  `json:"location"`
	Available      bool      `json:"available"`
	JoinedAt       time.Time `json:"joined_at"`
}

type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusAccepted   TaskStatus = "accepted"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusCancelled  TaskStatus = "cancelled"
)

type Task struct {
	ID             string     `json:"id"`
	Category       string     `json:"category"`
	Description    string     `json:"description"`
	Status         TaskStatus `json:"status"`
	ClientEmail    string     `json:"client_email"`
	TaskerID       string     `json:"tasker_id"`
	Location       Location   `json:"location"`
	ScheduledTime  time.Time  `json:"scheduled_time"`
	EstimatedHours float64    `json:"estimated_hours"`
	HourlyRate     float64    `json:"hourly_rate"`
	TotalCost      float64    `json:"total_cost"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// Database represents our in-memory database
type Database struct {
	Users      map[string]User     `json:"users"`
	Taskers    map[string]Tasker   `json:"taskers"`
	Tasks      map[string]Task     `json:"tasks"`
	Categories map[string]Category `json:"categories"`
	mu         sync.RWMutex
}

// Global database instance
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

func (d *Database) GetTasker(id string) (Tasker, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	tasker, exists := d.Taskers[id]
	if !exists {
		return Tasker{}, errors.New("tasker not found")
	}
	return tasker, nil
}

func (d *Database) GetTask(id string) (Task, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	task, exists := d.Tasks[id]
	if !exists {
		return Task{}, errors.New("task not found")
	}
	return task, nil
}

func (d *Database) CreateTask(task Task) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Tasks[task.ID] = task
	return nil
}

// HTTP Handlers
func getAvailableTaskers(c *fiber.Ctx) error {
	category := c.Query("category")
	lat := c.QueryFloat("latitude", 0)
	lon := c.QueryFloat("longitude", 0)

	if category == "" || lat == 0 || lon == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "category, latitude, and longitude are required",
		})
	}

	var availableTaskers []Tasker
	maxDistance := 20.0 // Maximum distance in km

	db.mu.RLock()
	for _, tasker := range db.Taskers {
		if !tasker.Available {
			continue
		}

		// Check if tasker serves this category
		categoryMatch := false
		for _, cat := range tasker.Categories {
			if cat == category {
				categoryMatch = true
				break
			}
		}
		if !categoryMatch {
			continue
		}

		// Check distance
		distance := calculateDistance(lat, lon,
			tasker.Location.Latitude,
			tasker.Location.Longitude)

		if distance <= maxDistance {
			availableTaskers = append(availableTaskers, tasker)
		}
	}
	db.mu.RUnlock()

	return c.JSON(availableTaskers)
}

func getUserTasks(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	var userTasks []Task
	db.mu.RLock()
	for _, task := range db.Tasks {
		if task.ClientEmail == email {
			userTasks = append(userTasks, task)
		}
	}
	db.mu.RUnlock()

	return c.JSON(userTasks)
}

type CreateTaskRequest struct {
	CategoryID     string   `json:"category_id"`
	Description    string   `json:"description"`
	Location       Location `json:"location"`
	ScheduledTime  string   `json:"scheduled_time"`
	EstimatedHours float64  `json:"estimated_hours"`
	TaskerID       string   `json:"tasker_id"`
	UserEmail      string   `json:"user_email"`
}

func createTask(c *fiber.Ctx) error {
	var req CreateTaskRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user
	user, err := db.GetUser(req.UserEmail)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// Validate tasker
	tasker, err := db.GetTasker(req.TaskerID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Tasker not found",
		})
	}

	// Validate category
	category, exists := db.Categories[req.CategoryID]
	if !exists {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid category",
		})
	}

	// Parse scheduled time
	scheduledTime, err := time.Parse(time.RFC3339, req.ScheduledTime)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid scheduled time format",
		})
	}

	// Calculate total cost
	totalCost := tasker.HourlyRate * req.EstimatedHours

	task := Task{
		ID:             uuid.New().String(),
		Category:       category.Name,
		Description:    req.Description,
		Status:         TaskStatusPending,
		ClientEmail:    user.Email,
		TaskerID:       tasker.ID,
		Location:       req.Location,
		ScheduledTime:  scheduledTime,
		EstimatedHours: req.EstimatedHours,
		HourlyRate:     tasker.HourlyRate,
		TotalCost:      totalCost,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := db.CreateTask(task); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create task",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(task)
}

func getCategories(c *fiber.Ctx) error {
	var categories []Category
	db.mu.RLock()
	for _, category := range db.Categories {
		categories = append(categories, category)
	}
	db.mu.RUnlock()

	return c.JSON(categories)
}

func getTaskDetails(c *fiber.Ctx) error {
	taskId := c.Params("taskId")
	if taskId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Task ID is required",
		})
	}

	task, err := db.GetTask(taskId)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Task not found",
		})
	}

	return c.JSON(task)
}

// Helper functions
func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	// Simplified distance calculation
	return ((lat2 - lat1) * (lat2 - lat1)) + ((lon2 - lon1) * (lon2 - lon1))
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:      make(map[string]User),
		Taskers:    make(map[string]Tasker),
		Tasks:      make(map[string]Task),
		Categories: make(map[string]Category),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Tasker routes
	api.Get("/taskers", getAvailableTaskers)

	// Task routes
	api.Get("/tasks", getUserTasks)
	api.Post("/tasks", createTask)
	api.Get("/tasks/:taskId", getTaskDetails)

	// Category routes
	api.Get("/categories", getCategories)
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

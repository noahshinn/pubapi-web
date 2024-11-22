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
type Course struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	Category      string    `json:"category"`
	Difficulty    string    `json:"difficulty"`
	Instructor    string    `json:"instructor"`
	DurationWeeks int       `json:"duration_weeks"`
	Rating        float64   `json:"rating"`
	Modules       []Module  `json:"modules"`
	CreatedAt     time.Time `json:"created_at"`
}

type Module struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Content     string `json:"content"`
	Duration    int    `json:"duration_minutes"`
	Order       int    `json:"order"`
	QuizID      string `json:"quiz_id,omitempty"`
}

type Quiz struct {
	ID        string     `json:"id"`
	Questions []Question `json:"questions"`
	PassScore float64    `json:"pass_score"`
}

type Question struct {
	ID      string   `json:"id"`
	Text    string   `json:"text"`
	Options []string `json:"options"`
	Answer  int      `json:"answer"`
	Points  int      `json:"points"`
}

type User struct {
	Email          string    `json:"email"`
	Name           string    `json:"name"`
	JoinDate       time.Time `json:"join_date"`
	Interests      []string  `json:"interests"`
	Certifications []string  `json:"certifications"`
}

type Enrollment struct {
	ID           string    `json:"id"`
	CourseID     string    `json:"course_id"`
	UserEmail    string    `json:"user_email"`
	Status       string    `json:"status"` // active, completed, dropped
	EnrolledAt   time.Time `json:"enrolled_at"`
	LastAccessed time.Time `json:"last_accessed"`
	Progress     Progress  `json:"progress"`
}

type Progress struct {
	CompletedModules     []string  `json:"completed_modules"`
	CompletionPercentage float64   `json:"completion_percentage"`
	CurrentModule        string    `json:"current_module"`
	LastQuizScore        float64   `json:"last_quiz_score"`
	QuizAttempts         []Attempt `json:"quiz_attempts"`
}

type Attempt struct {
	QuizID    string    `json:"quiz_id"`
	Score     float64   `json:"score"`
	Timestamp time.Time `json:"timestamp"`
}

// Database represents our in-memory database
type Database struct {
	Users       map[string]User       `json:"users"`
	Courses     map[string]Course     `json:"courses"`
	Enrollments map[string]Enrollment `json:"enrollments"`
	Quizzes     map[string]Quiz       `json:"quizzes"`
	mu          sync.RWMutex
}

// Global database instance
var db *Database

// Error definitions
var (
	ErrUserNotFound       = errors.New("user not found")
	ErrCourseNotFound     = errors.New("course not found")
	ErrEnrollmentNotFound = errors.New("enrollment not found")
	ErrInvalidInput       = errors.New("invalid input")
)

// Database operations
func (d *Database) GetUser(email string) (User, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	user, exists := d.Users[email]
	if !exists {
		return User{}, ErrUserNotFound
	}
	return user, nil
}

func (d *Database) GetCourse(id string) (Course, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	course, exists := d.Courses[id]
	if !exists {
		return Course{}, ErrCourseNotFound
	}
	return course, nil
}

func (d *Database) CreateEnrollment(enrollment Enrollment) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Enrollments[enrollment.ID] = enrollment
	return nil
}

func (d *Database) UpdateProgress(enrollmentID string, progress Progress) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	enrollment, exists := d.Enrollments[enrollmentID]
	if !exists {
		return ErrEnrollmentNotFound
	}

	enrollment.Progress = progress
	enrollment.LastAccessed = time.Now()
	d.Enrollments[enrollmentID] = enrollment
	return nil
}

// HTTP Handlers
func getCourses(c *fiber.Ctx) error {
	category := c.Query("category")
	difficulty := c.Query("difficulty")

	var filteredCourses []Course

	db.mu.RLock()
	for _, course := range db.Courses {
		if (category == "" || course.Category == category) &&
			(difficulty == "" || course.Difficulty == difficulty) {
			filteredCourses = append(filteredCourses, course)
		}
	}
	db.mu.RUnlock()

	return c.JSON(filteredCourses)
}

func getEnrollments(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	var userEnrollments []Enrollment
	db.mu.RLock()
	for _, enrollment := range db.Enrollments {
		if enrollment.UserEmail == email {
			userEnrollments = append(userEnrollments, enrollment)
		}
	}
	db.mu.RUnlock()

	return c.JSON(userEnrollments)
}

func createEnrollment(c *fiber.Ctx) error {
	var req struct {
		CourseID  string `json:"course_id"`
		UserEmail string `json:"user_email"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Verify user exists
	if _, err := db.GetUser(req.UserEmail); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Verify course exists
	course, err := db.GetCourse(req.CourseID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Check if already enrolled
	db.mu.RLock()
	for _, enrollment := range db.Enrollments {
		if enrollment.UserEmail == req.UserEmail &&
			enrollment.CourseID == req.CourseID &&
			enrollment.Status == "active" {
			db.mu.RUnlock()
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "Already enrolled in this course",
			})
		}
	}
	db.mu.RUnlock()

	// Create new enrollment
	enrollment := Enrollment{
		ID:           uuid.New().String(),
		CourseID:     req.CourseID,
		UserEmail:    req.UserEmail,
		Status:       "active",
		EnrolledAt:   time.Now(),
		LastAccessed: time.Now(),
		Progress: Progress{
			CompletedModules:     []string{},
			CompletionPercentage: 0,
			CurrentModule:        course.Modules[0].ID,
			LastQuizScore:        0,
			QuizAttempts:         []Attempt{},
		},
	}

	if err := db.CreateEnrollment(enrollment); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create enrollment",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(enrollment)
}

func getProgress(c *fiber.Ctx) error {
	enrollmentID := c.Params("enrollmentId")

	db.mu.RLock()
	enrollment, exists := db.Enrollments[enrollmentID]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Enrollment not found",
		})
	}

	return c.JSON(enrollment.Progress)
}

func updateProgress(c *fiber.Ctx) error {
	enrollmentID := c.Params("enrollmentId")

	var req struct {
		CompletedModule string  `json:"completed_module"`
		QuizScore       float64 `json:"quiz_score"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	enrollment, exists := db.Enrollments[enrollmentID]
	if !exists {
		db.mu.Unlock()
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Enrollment not found",
		})
	}

	// Update progress
	if req.CompletedModule != "" {
		enrollment.Progress.CompletedModules = append(
			enrollment.Progress.CompletedModules,
			req.CompletedModule,
		)

		// Calculate new completion percentage
		course, _ := db.GetCourse(enrollment.CourseID)
		completion := float64(len(enrollment.Progress.CompletedModules)) /
			float64(len(course.Modules)) * 100
		enrollment.Progress.CompletionPercentage = completion

		// Update current module if there are more modules
		for i, module := range course.Modules {
			if module.ID == req.CompletedModule && i < len(course.Modules)-1 {
				enrollment.Progress.CurrentModule = course.Modules[i+1].ID
				break
			}
		}
	}

	if req.QuizScore > 0 {
		enrollment.Progress.LastQuizScore = req.QuizScore
		enrollment.Progress.QuizAttempts = append(
			enrollment.Progress.QuizAttempts,
			Attempt{
				QuizID:    enrollment.Progress.CurrentModule,
				Score:     req.QuizScore,
				Timestamp: time.Now(),
			},
		)
	}

	// Check if course is completed
	if enrollment.Progress.CompletionPercentage >= 100 {
		enrollment.Status = "completed"
	}

	db.Enrollments[enrollmentID] = enrollment
	db.mu.Unlock()

	return c.JSON(enrollment.Progress)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:       make(map[string]User),
		Courses:     make(map[string]Course),
		Enrollments: make(map[string]Enrollment),
		Quizzes:     make(map[string]Quiz),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Course routes
	api.Get("/courses", getCourses)
	api.Get("/courses/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		course, err := db.GetCourse(id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.JSON(course)
	})

	// Enrollment routes
	api.Get("/enrollments", getEnrollments)
	api.Post("/enrollments", createEnrollment)

	// Progress routes
	api.Get("/progress/:enrollmentId", getProgress)
	api.Put("/progress/:enrollmentId", updateProgress)

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

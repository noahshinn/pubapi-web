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

type Course struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	Instructor    string    `json:"instructor"`
	Category      string    `json:"category"`
	Difficulty    string    `json:"difficulty"`
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
}

type User struct {
	Email          string    `json:"email"`
	Name           string    `json:"name"`
	JoinDate       time.Time `json:"join_date"`
	Interests      []string  `json:"interests"`
	Certifications []string  `json:"certifications"`
	PreferredLang  string    `json:"preferred_language"`
}

type Enrollment struct {
	ID          string    `json:"id"`
	CourseID    string    `json:"course_id"`
	UserEmail   string    `json:"user_email"`
	Status      string    `json:"status"`
	Progress    Progress  `json:"progress"`
	EnrolledAt  time.Time `json:"enrolled_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

type Progress struct {
	EnrollmentID     string    `json:"enrollment_id"`
	CompletedModules int       `json:"completed_modules"`
	TotalModules     int       `json:"total_modules"`
	CurrentGrade     float64   `json:"current_grade"`
	LastActivity     time.Time `json:"last_activity"`
}

type Database struct {
	Users       map[string]User       `json:"users"`
	Courses     map[string]Course     `json:"courses"`
	Enrollments map[string]Enrollment `json:"enrollments"`
	mu          sync.RWMutex
}

var db *Database

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:       make(map[string]User),
		Courses:     make(map[string]Course),
		Enrollments: make(map[string]Enrollment),
	}

	return json.Unmarshal(data, db)
}

func getCourses(c *fiber.Ctx) error {
	category := c.Query("category")
	difficulty := c.Query("difficulty")

	db.mu.RLock()
	defer db.mu.RUnlock()

	var courses []Course
	for _, course := range db.Courses {
		if (category == "" || course.Category == category) &&
			(difficulty == "" || course.Difficulty == difficulty) {
			courses = append(courses, course)
		}
	}

	return c.JSON(courses)
}

func getEnrollments(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	var userEnrollments []Enrollment
	for _, enrollment := range db.Enrollments {
		if enrollment.UserEmail == email {
			userEnrollments = append(userEnrollments, enrollment)
		}
	}

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

	db.mu.Lock()
	defer db.mu.Unlock()

	// Verify user exists
	if _, exists := db.Users[req.UserEmail]; !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// Verify course exists
	course, exists := db.Courses[req.CourseID]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Course not found",
		})
	}

	// Check if already enrolled
	for _, enrollment := range db.Enrollments {
		if enrollment.CourseID == req.CourseID && enrollment.UserEmail == req.UserEmail {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "Already enrolled in this course",
			})
		}
	}

	enrollment := Enrollment{
		ID:         uuid.New().String(),
		CourseID:   req.CourseID,
		UserEmail:  req.UserEmail,
		Status:     "active",
		EnrolledAt: time.Now(),
		Progress: Progress{
			CompletedModules: 0,
			TotalModules:     len(course.Modules),
			CurrentGrade:     0,
			LastActivity:     time.Now(),
		},
	}

	db.Enrollments[enrollment.ID] = enrollment

	return c.Status(fiber.StatusCreated).JSON(enrollment)
}

func getProgress(c *fiber.Ctx) error {
	enrollmentID := c.Params("enrollmentId")

	db.mu.RLock()
	defer db.mu.RUnlock()

	enrollment, exists := db.Enrollments[enrollmentID]
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
		CompletedModules int     `json:"completed_modules"`
		CurrentGrade     float64 `json:"current_grade"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	enrollment, exists := db.Enrollments[enrollmentID]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Enrollment not found",
		})
	}

	course := db.Courses[enrollment.CourseID]
	if req.CompletedModules > len(course.Modules) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Completed modules cannot exceed total modules",
		})
	}

	enrollment.Progress.CompletedModules = req.CompletedModules
	enrollment.Progress.CurrentGrade = req.CurrentGrade
	enrollment.Progress.LastActivity = time.Now()

	if req.CompletedModules == len(course.Modules) {
		enrollment.Status = "completed"
		enrollment.CompletedAt = time.Now()
	}

	db.Enrollments[enrollmentID] = enrollment

	return c.JSON(enrollment.Progress)
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

	// Course routes
	api.Get("/courses", getCourses)

	// Enrollment routes
	api.Get("/enrollments", getEnrollments)
	api.Post("/enrollments", createEnrollment)

	// Progress routes
	api.Get("/progress/:enrollmentId", getProgress)
	api.Put("/progress/:enrollmentId", updateProgress)
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
	log.Fatal(app.Listen(":" + *port))
}

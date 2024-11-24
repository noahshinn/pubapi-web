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
type User struct {
	Email            string    `json:"email"`
	Name             string    `json:"name"`
	Bio              string    `json:"bio"`
	JoinedAt         time.Time `json:"joined_at"`
	Interests        []string  `json:"interests"`
	SubscriptionTier string    `json:"subscription_tier"`
}

type Instructor struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Bio         string    `json:"bio"`
	Rating      float64   `json:"rating"`
	CourseCount int       `json:"course_count"`
	Students    int       `json:"students"`
	JoinedAt    time.Time `json:"joined_at"`
}

type Lesson struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Duration    int    `json:"duration"` // in minutes
	VideoURL    string `json:"video_url"`
	Order       int    `json:"order"`
}

type Course struct {
	ID            string     `json:"id"`
	Title         string     `json:"title"`
	Description   string     `json:"description"`
	Instructor    Instructor `json:"instructor"`
	Category      string     `json:"category"`
	Subcategory   string     `json:"subcategory"`
	Level         string     `json:"level"`
	Duration      int        `json:"duration"` // total minutes
	Lessons       []Lesson   `json:"lessons"`
	Rating        float64    `json:"rating"`
	EnrolledCount int        `json:"enrolled_count"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type Enrollment struct {
	ID         string    `json:"id"`
	UserEmail  string    `json:"user_email"`
	CourseID   string    `json:"course_id"`
	Progress   int       `json:"progress"` // percentage
	Completed  bool      `json:"completed"`
	EnrolledAt time.Time `json:"enrolled_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type LessonProgress struct {
	EnrollmentID string    `json:"enrollment_id"`
	LessonID     string    `json:"lesson_id"`
	Completed    bool      `json:"completed"`
	Progress     int       `json:"progress"` // percentage
	LastWatched  time.Time `json:"last_watched"`
}

// Database represents our in-memory database
type Database struct {
	Users          map[string]User           `json:"users"`
	Courses        map[string]Course         `json:"courses"`
	Enrollments    map[string]Enrollment     `json:"enrollments"`
	LessonProgress map[string]LessonProgress `json:"lesson_progress"`
	mu             sync.RWMutex
}

// Custom errors
var (
	ErrUserNotFound       = errors.New("user not found")
	ErrCourseNotFound     = errors.New("course not found")
	ErrEnrollmentNotFound = errors.New("enrollment not found")
	ErrInvalidInput       = errors.New("invalid input")
)

// Global database instance
var db *Database

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

func (d *Database) UpdateProgress(progress LessonProgress) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.LessonProgress[progress.EnrollmentID+":"+progress.LessonID] = progress
	return nil
}

// HTTP Handlers
func getCourses(c *fiber.Ctx) error {
	category := c.Query("category")
	search := c.Query("search")

	var courses []Course
	db.mu.RLock()
	for _, course := range db.Courses {
		if category != "" && course.Category != category {
			continue
		}
		// Simple search implementation
		if search != "" && !contains(course.Title, search) && !contains(course.Description, search) {
			continue
		}
		courses = append(courses, course)
	}
	db.mu.RUnlock()

	return c.JSON(courses)
}

func getCourseDetails(c *fiber.Ctx) error {
	courseId := c.Params("courseId")

	course, err := db.GetCourse(courseId)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(course)
}

func getEnrollments(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	var enrollments []Enrollment
	db.mu.RLock()
	for _, enrollment := range db.Enrollments {
		if enrollment.UserEmail == email {
			enrollments = append(enrollments, enrollment)
		}
	}
	db.mu.RUnlock()

	return c.JSON(enrollments)
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
	_, err := db.GetUser(req.UserEmail)
	if err != nil {
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
		if enrollment.UserEmail == req.UserEmail && enrollment.CourseID == req.CourseID {
			db.mu.RUnlock()
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "Already enrolled in this course",
			})
		}
	}
	db.mu.RUnlock()

	enrollment := Enrollment{
		ID:         uuid.New().String(),
		UserEmail:  req.UserEmail,
		CourseID:   req.CourseID,
		Progress:   0,
		Completed:  false,
		EnrolledAt: time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := db.CreateEnrollment(enrollment); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create enrollment",
		})
	}

	// Update course enrollment count
	course.EnrolledCount++
	db.mu.Lock()
	db.Courses[course.ID] = course
	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(enrollment)
}

func updateProgress(c *fiber.Ctx) error {
	var req struct {
		EnrollmentID string `json:"enrollment_id"`
		LessonID     string `json:"lesson_id"`
		Progress     int    `json:"progress"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate progress value
	if req.Progress < 0 || req.Progress > 100 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Progress must be between 0 and 100",
		})
	}

	progress := LessonProgress{
		EnrollmentID: req.EnrollmentID,
		LessonID:     req.LessonID,
		Progress:     req.Progress,
		Completed:    req.Progress == 100,
		LastWatched:  time.Now(),
	}

	if err := db.UpdateProgress(progress); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update progress",
		})
	}

	// Update overall course progress
	db.mu.Lock()
	enrollment, exists := db.Enrollments[req.EnrollmentID]
	if exists {
		var totalProgress int
		var completedLessons int
		course, _ := db.GetCourse(enrollment.CourseID)

		for _, lesson := range course.Lessons {
			key := req.EnrollmentID + ":" + lesson.ID
			if progress, exists := db.LessonProgress[key]; exists {
				totalProgress += progress.Progress
				if progress.Completed {
					completedLessons++
				}
			}
		}

		enrollment.Progress = totalProgress / len(course.Lessons)
		enrollment.Completed = completedLessons == len(course.Lessons)
		enrollment.UpdatedAt = time.Now()
		db.Enrollments[req.EnrollmentID] = enrollment
	}
	db.mu.Unlock()

	return c.JSON(progress)
}

// Utility functions
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:          make(map[string]User),
		Courses:        make(map[string]Course),
		Enrollments:    make(map[string]Enrollment),
		LessonProgress: make(map[string]LessonProgress),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Course routes
	api.Get("/courses", getCourses)
	api.Get("/courses/:courseId", getCourseDetails)

	// Enrollment routes
	api.Get("/enrollments", getEnrollments)
	api.Post("/enrollments", createEnrollment)

	// Progress routes
	api.Post("/progress", updateProgress)

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
	app.Use(recover.New())
	app.Use(cors.New())

	// Setup routes
	setupRoutes(app)

	// Start server
	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

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
)

// Domain Models
type Lesson struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Duration    int       `json:"duration"` // in minutes
	Description string    `json:"description"`
	VideoURL    string    `json:"video_url"`
	Resources   []string  `json:"resources"`
	CreatedAt   time.Time `json:"created_at"`
}

type Course struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Instructor  string    `json:"instructor"`
	Category    string    `json:"category"`
	Description string    `json:"description"`
	Duration    int       `json:"duration"` // total minutes
	Lessons     []Lesson  `json:"lessons"`
	Price       float64   `json:"price"`
	Rating      float64   `json:"rating"`
	Students    int       `json:"students"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type User struct {
	Email           string               `json:"email"`
	Name            string               `json:"name"`
	EnrolledCourses map[string]time.Time `json:"enrolled_courses"` // courseId -> enrollmentDate
	Preferences     []string             `json:"preferences"`      // preferred categories
}

type CourseProgress struct {
	UserEmail        string    `json:"user_email"`
	CourseID         string    `json:"course_id"`
	CompletedLessons []string  `json:"completed_lessons"` // lesson IDs
	Progress         float64   `json:"progress"`          // percentage
	LastAccessed     time.Time `json:"last_accessed"`
}

// Database represents our in-memory database
type Database struct {
	Users          map[string]User                      `json:"users"`
	Courses        map[string]Course                    `json:"courses"`
	CourseProgress map[string]map[string]CourseProgress `json:"course_progress"` // userEmail -> courseId -> progress
	mu             sync.RWMutex
}

// Custom errors
var (
	ErrUserNotFound   = errors.New("user not found")
	ErrCourseNotFound = errors.New("course not found")
	ErrLessonNotFound = errors.New("lesson not found")
	ErrNotEnrolled    = errors.New("user not enrolled in course")
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

func (d *Database) GetUserProgress(email, courseId string) (CourseProgress, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	userProgress, exists := d.CourseProgress[email]
	if !exists {
		return CourseProgress{}, ErrUserNotFound
	}

	progress, exists := userProgress[courseId]
	if !exists {
		return CourseProgress{}, ErrNotEnrolled
	}

	return progress, nil
}

func (d *Database) UpdateProgress(email, courseId, lessonId string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Initialize maps if they don't exist
	if d.CourseProgress[email] == nil {
		d.CourseProgress[email] = make(map[string]CourseProgress)
	}

	progress := d.CourseProgress[email][courseId]

	// Add lesson to completed lessons if not already completed
	found := false
	for _, id := range progress.CompletedLessons {
		if id == lessonId {
			found = true
			break
		}
	}

	if !found {
		progress.CompletedLessons = append(progress.CompletedLessons, lessonId)
	}

	// Update progress percentage
	course, exists := d.Courses[courseId]
	if !exists {
		return ErrCourseNotFound
	}

	progress.Progress = float64(len(progress.CompletedLessons)) / float64(len(course.Lessons)) * 100
	progress.LastAccessed = time.Now()

	d.CourseProgress[email][courseId] = progress
	return nil
}

// HTTP Handlers
func getCourses(c *fiber.Ctx) error {
	category := c.Query("category")
	instructor := c.Query("instructor")

	var filteredCourses []Course

	db.mu.RLock()
	for _, course := range db.Courses {
		if (category == "" || course.Category == category) &&
			(instructor == "" || course.Instructor == instructor) {
			filteredCourses = append(filteredCourses, course)
		}
	}
	db.mu.RUnlock()

	return c.JSON(filteredCourses)
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

func getUserProgress(c *fiber.Ctx) error {
	email := c.Params("email")

	user, err := db.GetUser(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	var progress []CourseProgress
	db.mu.RLock()
	for courseId := range user.EnrolledCourses {
		if p, exists := db.CourseProgress[email][courseId]; exists {
			progress = append(progress, p)
		}
	}
	db.mu.RUnlock()

	return c.JSON(progress)
}

type CompleteLessonRequest struct {
	Email    string `json:"email"`
	CourseId string `json:"courseId"`
}

func completeLesson(c *fiber.Ctx) error {
	lessonId := c.Params("lessonId")

	var req CompleteLessonRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Verify user exists and is enrolled in course
	user, err := db.GetUser(req.Email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if _, enrolled := user.EnrolledCourses[req.CourseId]; !enrolled {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": ErrNotEnrolled.Error(),
		})
	}

	// Update progress
	if err := db.UpdateProgress(req.Email, req.CourseId, lessonId); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Get updated progress
	progress, _ := db.GetUserProgress(req.Email, req.CourseId)
	return c.JSON(progress)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:          make(map[string]User),
		Courses:        make(map[string]Course),
		CourseProgress: make(map[string]map[string]CourseProgress),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Course routes
	api.Get("/courses", getCourses)
	api.Get("/courses/:courseId", getCourseDetails)

	// Progress routes
	api.Get("/users/:email/progress", getUserProgress)
	api.Post("/lessons/:lessonId/complete", completeLesson)
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

package main

import (
	"encoding/json"
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
type Lecture struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Duration int    `json:"duration"` // in minutes
	Type     string `json:"type"`     // video, article, quiz
	Content  string `json:"content"`  // URL or content text
}

type Section struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Lectures []Lecture `json:"lectures"`
}

type Course struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	Instructor    string    `json:"instructor"`
	Category      string    `json:"category"`
	Price         float64   `json:"price"`
	Rating        float64   `json:"rating"`
	StudentsCount int       `json:"students_count"`
	Sections      []Section `json:"sections"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type User struct {
	Email             string    `json:"email"`
	Name              string    `json:"name"`
	EnrolledCourses   []string  `json:"enrolled_courses"`   // Course IDs
	CompletedLectures []string  `json:"completed_lectures"` // Lecture IDs
	Certificates      []string  `json:"certificates"`
	CreatedAt         time.Time `json:"created_at"`
}

type Progress struct {
	UserEmail         string    `json:"user_email"`
	CourseID          string    `json:"course_id"`
	CompletedLectures []string  `json:"completed_lectures"`
	LastAccessed      time.Time `json:"last_accessed"`
	Progress          float64   `json:"progress"` // 0-100
}

type Certificate struct {
	ID        string    `json:"id"`
	UserEmail string    `json:"user_email"`
	CourseID  string    `json:"course_id"`
	IssuedAt  time.Time `json:"issued_at"`
	URL       string    `json:"url"`
}

// Database represents our in-memory database
type Database struct {
	Users        map[string]User        `json:"users"`
	Courses      map[string]Course      `json:"courses"`
	Progress     map[string]Progress    `json:"progress"`
	Certificates map[string]Certificate `json:"certificates"`
	mu           sync.RWMutex
}

var db *Database

// Helper functions
func calculateProgress(courseID string, completedLectures []string) float64 {
	course, exists := db.Courses[courseID]
	if !exists {
		return 0
	}

	var totalLectures int
	for _, section := range course.Sections {
		totalLectures += len(section.Lectures)
	}

	if totalLectures == 0 {
		return 0
	}

	return float64(len(completedLectures)) / float64(totalLectures) * 100
}

func generateCertificate(userEmail, courseID string) Certificate {
	return Certificate{
		ID:        uuid.New().String(),
		UserEmail: userEmail,
		CourseID:  courseID,
		IssuedAt:  time.Now(),
		URL:       "https://udemy.com/certificates/" + uuid.New().String(),
	}
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
		if search != "" && !strings.Contains(strings.ToLower(course.Title), strings.ToLower(search)) {
			continue
		}
		courses = append(courses, course)
	}
	db.mu.RUnlock()

	return c.JSON(courses)
}

func getCourseDetails(c *fiber.Ctx) error {
	courseID := c.Params("courseId")

	db.mu.RLock()
	course, exists := db.Courses[courseID]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Course not found",
		})
	}

	return c.JSON(course)
}

func getUserCourses(c *fiber.Ctx) error {
	email := c.Params("email")

	db.mu.RLock()
	user, exists := db.Users[email]
	if !exists {
		db.mu.RUnlock()
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	var enrolledCourses []map[string]interface{}
	for _, courseID := range user.EnrolledCourses {
		course, exists := db.Courses[courseID]
		if !exists {
			continue
		}

		progress, exists := db.Progress[email+"-"+courseID]
		if !exists {
			progress = Progress{
				UserEmail: email,
				CourseID:  courseID,
				Progress:  0,
			}
		}

		enrolledCourse := map[string]interface{}{
			"course":        course,
			"progress":      progress.Progress,
			"last_accessed": progress.LastAccessed,
		}

		// Add certificate if course is completed
		if progress.Progress == 100 {
			for _, cert := range db.Certificates {
				if cert.UserEmail == email && cert.CourseID == courseID {
					enrolledCourse["completion_certificate"] = cert.URL
					break
				}
			}
		}

		enrolledCourses = append(enrolledCourses, enrolledCourse)
	}
	db.mu.RUnlock()

	return c.JSON(enrolledCourses)
}

func enrollInCourse(c *fiber.Ctx) error {
	courseID := c.Params("courseId")

	var req struct {
		UserEmail     string `json:"user_email"`
		PaymentMethod string `json:"payment_method"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	user, exists := db.Users[req.UserEmail]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	course, exists := db.Courses[courseID]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Course not found",
		})
	}

	// Check if already enrolled
	for _, enrolledCourseID := range user.EnrolledCourses {
		if enrolledCourseID == courseID {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Already enrolled in this course",
			})
		}
	}

	// Update user's enrolled courses
	user.EnrolledCourses = append(user.EnrolledCourses, courseID)
	db.Users[req.UserEmail] = user

	// Initialize progress
	db.Progress[req.UserEmail+"-"+courseID] = Progress{
		UserEmail:    req.UserEmail,
		CourseID:     courseID,
		LastAccessed: time.Now(),
		Progress:     0,
	}

	// Update course statistics
	course.StudentsCount++
	db.Courses[courseID] = course

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Successfully enrolled in course",
	})
}

func updateProgress(c *fiber.Ctx) error {
	courseID := c.Params("courseId")

	var req struct {
		UserEmail string `json:"user_email"`
		LectureID string `json:"lecture_id"`
		Completed bool   `json:"completed"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	// Verify user is enrolled
	user, exists := db.Users[req.UserEmail]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	isEnrolled := false
	for _, enrolledCourseID := range user.EnrolledCourses {
		if enrolledCourseID == courseID {
			isEnrolled = true
			break
		}
	}

	if !isEnrolled {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Not enrolled in this course",
		})
	}

	// Update progress
	progressKey := req.UserEmail + "-" + courseID
	progress, exists := db.Progress[progressKey]
	if !exists {
		progress = Progress{
			UserEmail: req.UserEmail,
			CourseID:  courseID,
		}
	}

	if req.Completed {
		// Add lecture to completed lectures if not already present
		found := false
		for _, lectureID := range progress.CompletedLectures {
			if lectureID == req.LectureID {
				found = true
				break
			}
		}
		if !found {
			progress.CompletedLectures = append(progress.CompletedLectures, req.LectureID)
		}
	} else {
		// Remove lecture from completed lectures
		var updatedLectures []string
		for _, lectureID := range progress.CompletedLectures {
			if lectureID != req.LectureID {
				updatedLectures = append(updatedLectures, lectureID)
			}
		}
		progress.CompletedLectures = updatedLectures
	}

	progress.LastAccessed = time.Now()
	progress.Progress = calculateProgress(courseID, progress.CompletedLectures)
	db.Progress[progressKey] = progress

	// If course is completed (100%), generate certificate if not already issued
	if progress.Progress == 100 {
		certificateExists := false
		for _, cert := range db.Certificates {
			if cert.UserEmail == req.UserEmail && cert.CourseID == courseID {
				certificateExists = true
				break
			}
		}
		if !certificateExists {
			certificate := generateCertificate(req.UserEmail, courseID)
			db.Certificates[certificate.ID] = certificate
		}
	}

	return c.JSON(progress)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:        make(map[string]User),
		Courses:      make(map[string]Course),
		Progress:     make(map[string]Progress),
		Certificates: make(map[string]Certificate),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Course routes
	api.Get("/courses", getCourses)
	api.Get("/courses/:courseId", getCourseDetails)
	api.Post("/courses/:courseId/enroll", enrollInCourse)
	api.Put("/courses/:courseId/progress", updateProgress)

	// User routes
	api.Get("/users/:email/courses", getUserCourses)
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

	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

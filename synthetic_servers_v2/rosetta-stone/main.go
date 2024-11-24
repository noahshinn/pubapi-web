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

type Language struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	NativeName      string   `json:"native_name"`
	Difficulty      string   `json:"difficulty"`
	AvailableLevels []string `json:"available_levels"`
}

type Course struct {
	ID           string    `json:"id"`
	LanguageID   string    `json:"language_id"`
	Level        string    `json:"level"`
	TotalUnits   int       `json:"total_units"`
	TotalLessons int       `json:"total_lessons"`
	EnrolledAt   time.Time `json:"enrolled_at"`
}

type Progress struct {
	UserEmail        string    `json:"user_email"`
	CourseID         string    `json:"course_id"`
	CurrentUnit      int       `json:"current_unit"`
	CurrentLesson    int       `json:"current_lesson"`
	CompletedLessons int       `json:"completed_lessons"`
	Accuracy         float64   `json:"accuracy"`
	StreakDays       int       `json:"streak_days"`
	LastActivity     time.Time `json:"last_activity"`
}

type User struct {
	Email           string     `json:"email"`
	Name            string     `json:"name"`
	NativeLanguage  string     `json:"native_language"`
	EnrolledCourses []Course   `json:"enrolled_courses"`
	Progress        []Progress `json:"progress"`
	JoinedAt        time.Time  `json:"joined_at"`
}

type Database struct {
	Users     map[string]User     `json:"users"`
	Languages map[string]Language `json:"languages"`
	mu        sync.RWMutex
}

var db *Database

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:     make(map[string]User),
		Languages: make(map[string]Language),
	}

	return json.Unmarshal(data, db)
}

func getLanguages(c *fiber.Ctx) error {
	db.mu.RLock()
	defer db.mu.RUnlock()

	languages := make([]Language, 0, len(db.Languages))
	for _, lang := range db.Languages {
		languages = append(languages, lang)
	}

	return c.JSON(languages)
}

func getUserCourses(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	db.mu.RLock()
	user, exists := db.Users[email]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "user not found",
		})
	}

	return c.JSON(user.EnrolledCourses)
}

type EnrollmentRequest struct {
	UserEmail  string `json:"user_email"`
	LanguageID string `json:"language_id"`
	Level      string `json:"level"`
}

func enrollInCourse(c *fiber.Ctx) error {
	var req EnrollmentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	user, exists := db.Users[req.UserEmail]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "user not found",
		})
	}

	language, exists := db.Languages[req.LanguageID]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "language not found",
		})
	}

	// Check if level is available
	levelValid := false
	for _, level := range language.AvailableLevels {
		if level == req.Level {
			levelValid = true
			break
		}
	}
	if !levelValid {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid level for this language",
		})
	}

	// Check if already enrolled
	for _, course := range user.EnrolledCourses {
		if course.LanguageID == req.LanguageID && course.Level == req.Level {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "already enrolled in this course",
			})
		}
	}

	// Create new course
	newCourse := Course{
		ID:           uuid.New().String(),
		LanguageID:   req.LanguageID,
		Level:        req.Level,
		TotalUnits:   10,
		TotalLessons: 50,
		EnrolledAt:   time.Now(),
	}

	// Initialize progress
	newProgress := Progress{
		UserEmail:        req.UserEmail,
		CourseID:         newCourse.ID,
		CurrentUnit:      1,
		CurrentLesson:    1,
		CompletedLessons: 0,
		Accuracy:         0,
		StreakDays:       0,
		LastActivity:     time.Now(),
	}

	user.EnrolledCourses = append(user.EnrolledCourses, newCourse)
	user.Progress = append(user.Progress, newProgress)
	db.Users[req.UserEmail] = user

	return c.Status(fiber.StatusCreated).JSON(newCourse)
}

func getProgress(c *fiber.Ctx) error {
	email := c.Query("email")
	courseID := c.Query("course_id")

	if email == "" || courseID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email and course_id parameters are required",
		})
	}

	db.mu.RLock()
	user, exists := db.Users[email]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "user not found",
		})
	}

	for _, progress := range user.Progress {
		if progress.CourseID == courseID {
			return c.JSON(progress)
		}
	}

	return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
		"error": "progress not found",
	})
}

type ProgressUpdate struct {
	UserEmail string  `json:"user_email"`
	CourseID  string  `json:"course_id"`
	LessonID  string  `json:"lesson_id"`
	Completed bool    `json:"completed"`
	Score     float64 `json:"score"`
}

func updateProgress(c *fiber.Ctx) error {
	var req ProgressUpdate
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	user, exists := db.Users[req.UserEmail]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "user not found",
		})
	}

	var userProgress *Progress
	for i := range user.Progress {
		if user.Progress[i].CourseID == req.CourseID {
			userProgress = &user.Progress[i]
			break
		}
	}

	if userProgress == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "progress not found",
		})
	}

	if req.Completed {
		userProgress.CompletedLessons++
		userProgress.CurrentLesson++
		if userProgress.CurrentLesson > 5 { // Assuming 5 lessons per unit
			userProgress.CurrentUnit++
			userProgress.CurrentLesson = 1
		}
	}

	// Update accuracy
	if req.Score > 0 {
		userProgress.Accuracy = (userProgress.Accuracy*float64(userProgress.CompletedLessons-1) + req.Score) / float64(userProgress.CompletedLessons)
	}

	userProgress.LastActivity = time.Now()

	// Update streak
	if time.Since(userProgress.LastActivity) < 24*time.Hour {
		userProgress.StreakDays++
	} else {
		userProgress.StreakDays = 1
	}

	db.Users[req.UserEmail] = user

	return c.JSON(userProgress)
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

	api.Get("/languages", getLanguages)
	api.Get("/courses", getUserCourses)
	api.Post("/courses", enrollInCourse)
	api.Get("/progress", getProgress)
	api.Post("/progress", updateProgress)
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

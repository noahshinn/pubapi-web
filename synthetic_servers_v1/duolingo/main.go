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
type LanguageProgress struct {
	Language      string `json:"language"`
	Level         int    `json:"level"`
	XP            int    `json:"xp"`
	FluencyScore  int    `json:"fluency_score"`
	UnitsComplete int    `json:"units_complete"`
}

type UserProfile struct {
	Email             string             `json:"email"`
	Username          string             `json:"username"`
	TotalXP           int                `json:"total_xp"`
	CurrentStreak     int                `json:"current_streak"`
	LongestStreak     int                `json:"longest_streak"`
	LearningLanguages []LanguageProgress `json:"learning_languages"`
	LastPracticeDate  time.Time          `json:"last_practice_date"`
	FreezeRemaining   int                `json:"freeze_remaining"`
}

type Course struct {
	ID           string   `json:"id"`
	Language     string   `json:"language"`
	FromLanguage string   `json:"from_language"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Difficulty   string   `json:"difficulty"`
	TotalUnits   int      `json:"total_units"`
	Skills       []string `json:"skills"`
}

type Lesson struct {
	ID        string     `json:"id"`
	CourseID  string     `json:"course_id"`
	Unit      int        `json:"unit"`
	Title     string     `json:"title"`
	Type      string     `json:"type"`
	XPReward  int        `json:"xp_reward"`
	Skills    []string   `json:"skills"`
	Questions []Question `json:"questions"`
}

type Question struct {
	ID      string   `json:"id"`
	Type    string   `json:"type"`
	Prompt  string   `json:"prompt"`
	Answer  string   `json:"answer"`
	Options []string `json:"options,omitempty"`
}

type LessonProgress struct {
	UserEmail   string    `json:"user_email"`
	LessonID    string    `json:"lesson_id"`
	Score       int       `json:"score"`
	Mistakes    int       `json:"mistakes"`
	Completed   bool      `json:"completed"`
	CompletedAt time.Time `json:"completed_at"`
}

// Database represents our in-memory database
type Database struct {
	Users    map[string]UserProfile    `json:"users"`
	Courses  map[string]Course         `json:"courses"`
	Lessons  map[string]Lesson         `json:"lessons"`
	Progress map[string]LessonProgress `json:"progress"`
	mu       sync.RWMutex
}

// Global database instance
var db *Database

// Error definitions
var (
	ErrUserNotFound    = errors.New("user not found")
	ErrCourseNotFound  = errors.New("course not found")
	ErrLessonNotFound  = errors.New("lesson not found")
	ErrInvalidProgress = errors.New("invalid progress submission")
)

// Database operations
func (d *Database) GetUser(email string) (UserProfile, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	user, exists := d.Users[email]
	if !exists {
		return UserProfile{}, ErrUserNotFound
	}
	return user, nil
}

func (d *Database) UpdateUserProgress(email string, lessonProgress LessonProgress) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	user, exists := d.Users[email]
	if !exists {
		return ErrUserNotFound
	}

	lesson, exists := d.Lessons[lessonProgress.LessonID]
	if !exists {
		return ErrLessonNotFound
	}

	// Update XP
	xpEarned := calculateXP(lessonProgress.Score, lesson.XPReward)
	user.TotalXP += xpEarned

	// Update streak
	if time.Since(user.LastPracticeDate) > 24*time.Hour {
		if user.FreezeRemaining > 0 {
			user.FreezeRemaining--
		} else {
			user.CurrentStreak = 0
		}
	}
	user.CurrentStreak++
	if user.CurrentStreak > user.LongestStreak {
		user.LongestStreak = user.CurrentStreak
	}
	user.LastPracticeDate = time.Now()

	// Update language progress
	course := d.Courses[lesson.CourseID]
	for i, lang := range user.LearningLanguages {
		if lang.Language == course.Language {
			user.LearningLanguages[i].XP += xpEarned
			user.LearningLanguages[i].Level = calculateLevel(user.LearningLanguages[i].XP)
			break
		}
	}

	d.Users[email] = user
	d.Progress[lessonProgress.LessonID] = lessonProgress
	return nil
}

// Helper functions
func calculateXP(score int, baseXP int) int {
	multiplier := float64(score) / 100.0
	return int(float64(baseXP) * multiplier)
}

func calculateLevel(xp int) int {
	// Simple level calculation: every 1000 XP is a new level
	return (xp / 1000) + 1
}

// HTTP Handlers
func getUserProfile(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	user, err := db.GetUser(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(user)
}

func getCourses(c *fiber.Ctx) error {
	fromLanguage := c.Query("from_language")
	if fromLanguage == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "from_language parameter is required",
		})
	}

	var availableCourses []Course
	db.mu.RLock()
	for _, course := range db.Courses {
		if course.FromLanguage == fromLanguage {
			availableCourses = append(availableCourses, course)
		}
	}
	db.mu.RUnlock()

	return c.JSON(availableCourses)
}

func getLessons(c *fiber.Ctx) error {
	courseID := c.Query("course_id")
	if courseID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "course_id parameter is required",
		})
	}

	var courseLessons []Lesson
	db.mu.RLock()
	for _, lesson := range db.Lessons {
		if lesson.CourseID == courseID {
			// Remove answers from questions before sending to client
			lessonCopy := lesson
			for i := range lessonCopy.Questions {
				lessonCopy.Questions[i].Answer = ""
			}
			courseLessons = append(courseLessons, lessonCopy)
		}
	}
	db.mu.RUnlock()

	return c.JSON(courseLessons)
}

func submitProgress(c *fiber.Ctx) error {
	var progress LessonProgress
	if err := c.BodyParser(&progress); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if progress.UserEmail == "" || progress.LessonID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "user_email and lesson_id are required",
		})
	}

	if err := db.UpdateUserProgress(progress.UserEmail, progress); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Progress recorded successfully",
	})
}

func getStreak(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	user, err := db.GetUser(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"current_streak":     user.CurrentStreak,
		"longest_streak":     user.LongestStreak,
		"last_practice_date": user.LastPracticeDate,
		"freeze_remaining":   user.FreezeRemaining,
	})
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:    make(map[string]UserProfile),
		Courses:  make(map[string]Course),
		Lessons:  make(map[string]Lesson),
		Progress: make(map[string]LessonProgress),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Profile routes
	api.Get("/profile", getUserProfile)

	// Course routes
	api.Get("/courses", getCourses)

	// Lesson routes
	api.Get("/lessons", getLessons)

	// Progress routes
	api.Post("/progress", submitProgress)

	// Streak routes
	api.Get("/streaks", getStreak)
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

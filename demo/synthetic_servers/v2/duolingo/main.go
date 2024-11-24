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
	"github.com/gofiber/fiber/v2/middleware/recover"
)

type Profile struct {
	Email          string    `json:"email"`
	Username       string    `json:"username"`
	DisplayName    string    `json:"display_name"`
	LearningLang   string    `json:"learning_language"`
	NativeLang     string    `json:"native_language"`
	TotalXP        int       `json:"total_xp"`
	Level          int       `json:"level"`
	Streak         int       `json:"streak"`
	Premium        bool      `json:"premium"`
	CreatedAt      time.Time `json:"created_at"`
	LastPracticeAt time.Time `json:"last_practice_at"`
}

type Course struct {
	ID           string `json:"id"`
	Language     string `json:"language"`
	FromLanguage string `json:"from_language"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	Difficulty   string `json:"difficulty"`
	TotalUnits   int    `json:"total_units"`
	ImageURL     string `json:"image_url"`
}

type Lesson struct {
	ID            string `json:"id"`
	CourseID      string `json:"course_id"`
	Unit          int    `json:"unit"`
	Title         string `json:"title"`
	Type          string `json:"type"`
	XPReward      int    `json:"xp_reward"`
	RequiredLevel int    `json:"required_level"`
	Completed     bool   `json:"completed"`
}

type Progress struct {
	Email     string `json:"email"`
	LessonID  string `json:"lesson_id"`
	Score     int    `json:"score"`
	Mistakes  int    `json:"mistakes"`
	TimeSpent int    `json:"time_spent"`
}

type ProgressResponse struct {
	XPGained       int      `json:"xp_gained"`
	LevelUp        bool     `json:"level_up"`
	StreakExtended bool     `json:"streak_extended"`
	Achievements   []string `json:"achievements"`
}

type Streak struct {
	CurrentStreak   int       `json:"current_streak"`
	LongestStreak   int       `json:"longest_streak"`
	LastPractice    time.Time `json:"last_practice"`
	FreezeRemaining int       `json:"freeze_remaining"`
}

type Database struct {
	Profiles map[string]Profile    `json:"profiles"`
	Courses  map[string]Course     `json:"courses"`
	Lessons  map[string]Lesson     `json:"lessons"`
	Progress map[string][]Progress `json:"progress"`
	Streaks  map[string]Streak     `json:"streaks"`
	mu       sync.RWMutex
}

var db *Database

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Profiles: make(map[string]Profile),
		Courses:  make(map[string]Course),
		Lessons:  make(map[string]Lesson),
		Progress: make(map[string][]Progress),
		Streaks:  make(map[string]Streak),
	}

	return json.Unmarshal(data, db)
}

func getProfile(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	db.mu.RLock()
	profile, exists := db.Profiles[email]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "profile not found",
		})
	}

	return c.JSON(profile)
}

func getCourses(c *fiber.Ctx) error {
	fromLang := c.Query("from_language")
	if fromLang == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "from_language parameter is required",
		})
	}

	var courses []Course
	db.mu.RLock()
	for _, course := range db.Courses {
		if course.FromLanguage == fromLang {
			courses = append(courses, course)
		}
	}
	db.mu.RUnlock()

	return c.JSON(courses)
}

func getLessons(c *fiber.Ctx) error {
	courseID := c.Query("course_id")
	if courseID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "course_id parameter is required",
		})
	}

	var lessons []Lesson
	db.mu.RLock()
	for _, lesson := range db.Lessons {
		if lesson.CourseID == courseID {
			lessons = append(lessons, lesson)
		}
	}
	db.mu.RUnlock()

	return c.JSON(lessons)
}

func submitProgress(c *fiber.Ctx) error {
	var progress Progress
	if err := c.BodyParser(&progress); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	// Update user progress
	db.Progress[progress.Email] = append(db.Progress[progress.Email], progress)

	// Update profile XP and check for level up
	profile := db.Profiles[progress.Email]
	oldLevel := profile.Level
	xpGained := calculateXP(progress)
	profile.TotalXP += xpGained
	profile.Level = calculateLevel(profile.TotalXP)
	db.Profiles[progress.Email] = profile

	// Update streak
	streak := db.Streaks[progress.Email]
	streakExtended := updateStreak(&streak, time.Now())
	db.Streaks[progress.Email] = streak

	// Check for achievements
	achievements := checkAchievements(progress, profile)

	response := ProgressResponse{
		XPGained:       xpGained,
		LevelUp:        profile.Level > oldLevel,
		StreakExtended: streakExtended,
		Achievements:   achievements,
	}

	return c.Status(fiber.StatusCreated).JSON(response)
}

func getStreak(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	db.mu.RLock()
	streak, exists := db.Streaks[email]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "streak not found",
		})
	}

	return c.JSON(streak)
}

func calculateXP(progress Progress) int {
	baseXP := 10
	if progress.Score >= 90 {
		baseXP += 5
	}
	if progress.Mistakes == 0 {
		baseXP += 3
	}
	return baseXP
}

func calculateLevel(totalXP int) int {
	return (totalXP / 1000) + 1
}

func updateStreak(streak *Streak, now time.Time) bool {
	lastPractice := streak.LastPractice
	if now.Sub(lastPractice) > 24*time.Hour {
		if streak.FreezeRemaining > 0 {
			streak.FreezeRemaining--
		} else {
			streak.CurrentStreak = 0
		}
	}

	if now.Day() != lastPractice.Day() {
		streak.CurrentStreak++
		if streak.CurrentStreak > streak.LongestStreak {
			streak.LongestStreak = streak.CurrentStreak
		}
		streak.LastPractice = now
		return true
	}

	return false
}

func checkAchievements(progress Progress, profile Profile) []string {
	var achievements []string

	// First lesson completion
	if len(db.Progress[profile.Email]) == 1 {
		achievements = append(achievements, "first_lesson_complete")
	}

	// Perfect score
	if progress.Score == 100 && progress.Mistakes == 0 {
		achievements = append(achievements, "perfect_lesson")
	}

	// Streak achievements
	streak := db.Streaks[profile.Email]
	if streak.CurrentStreak == 7 {
		achievements = append(achievements, "week_streak")
	} else if streak.CurrentStreak == 30 {
		achievements = append(achievements, "month_streak")
	}

	return achievements
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

	api.Get("/profile", getProfile)
	api.Get("/courses", getCourses)
	api.Get("/lessons", getLessons)
	api.Post("/progress", submitProgress)
	api.Get("/streaks", getStreak)
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

	app.Use(logger.New())
	app.Use(recover.New())
	app.Use(cors.New())

	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

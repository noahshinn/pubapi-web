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
type Course struct {
	ID            string    `json:"id"`
	Language      string    `json:"language"`
	Level         string    `json:"level"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	DurationWeeks int       `json:"duration_weeks"`
	Lessons       []Lesson  `json:"lessons"`
	CreatedAt     time.Time `json:"created_at"`
}

type Lesson struct {
	ID              string    `json:"id"`
	Title           string    `json:"title"`
	Type            string    `json:"type"` // vocabulary, grammar, pronunciation, etc.
	DurationMinutes int       `json:"duration_minutes"`
	Order           int       `json:"order"`
	Content         []Content `json:"content"`
}

type Content struct {
	Type     string `json:"type"` // text, audio, image, video
	Data     string `json:"data"`
	Solution string `json:"solution,omitempty"`
}

type UserProgress struct {
	UserEmail        string    `json:"user_email"`
	CourseID         string    `json:"course_id"`
	CompletedLessons []string  `json:"completed_lessons"`
	CurrentStreak    int       `json:"current_streak"`
	TotalPoints      int       `json:"total_points"`
	ProficiencyLevel string    `json:"proficiency_level"`
	LastActivity     time.Time `json:"last_activity"`
}

type LessonCompletion struct {
	UserEmail   string    `json:"user_email"`
	LessonID    string    `json:"lesson_id"`
	Score       int       `json:"score"`
	TimeSpent   int       `json:"time_spent"`
	CompletedAt time.Time `json:"completed_at"`
}

type PracticeSession struct {
	ID           string    `json:"id"`
	UserEmail    string    `json:"user_email"`
	CourseID     string    `json:"course_id"`
	LessonID     string    `json:"lesson_id"`
	PracticeType string    `json:"practice_type"`
	StartedAt    time.Time `json:"started_at"`
	EndedAt      time.Time `json:"ended_at,omitempty"`
	Score        int       `json:"score,omitempty"`
}

type User struct {
	Email            string    `json:"email"`
	Name             string    `json:"name"`
	PreferredLangs   []string  `json:"preferred_langs"`
	LearningGoals    []string  `json:"learning_goals"`
	DailyGoalMins    int       `json:"daily_goal_mins"`
	SubscriptionType string    `json:"subscription_type"`
	JoinedAt         time.Time `json:"joined_at"`
}

// Database represents our in-memory database
type Database struct {
	Users            map[string]User            `json:"users"`
	Courses          map[string]Course          `json:"courses"`
	UserProgress     map[string][]UserProgress  `json:"user_progress"`
	PracticeSessions map[string]PracticeSession `json:"practice_sessions"`
	mu               sync.RWMutex
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

func (d *Database) GetCourse(id string) (Course, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	course, exists := d.Courses[id]
	if !exists {
		return Course{}, errors.New("course not found")
	}
	return course, nil
}

func (d *Database) GetUserProgress(email string) ([]UserProgress, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	progress, exists := d.UserProgress[email]
	if !exists {
		return nil, errors.New("no progress found")
	}
	return progress, nil
}

func (d *Database) UpdateUserProgress(progress UserProgress) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	currentProgress := d.UserProgress[progress.UserEmail]
	for i, p := range currentProgress {
		if p.CourseID == progress.CourseID {
			currentProgress[i] = progress
			d.UserProgress[progress.UserEmail] = currentProgress
			return nil
		}
	}

	d.UserProgress[progress.UserEmail] = append(d.UserProgress[progress.UserEmail], progress)
	return nil
}

// HTTP Handlers
func getCourses(c *fiber.Ctx) error {
	var courses []Course
	db.mu.RLock()
	for _, course := range db.Courses {
		courses = append(courses, course)
	}
	db.mu.RUnlock()

	return c.JSON(courses)
}

func getUserProgress(c *fiber.Ctx) error {
	email := c.Params("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	progress, err := db.GetUserProgress(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(progress)
}

func completedLesson(c *fiber.Ctx) error {
	lessonId := c.Params("lessonId")
	var completion LessonCompletion

	if err := c.BodyParser(&completion); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user exists
	user, err := db.GetUser(completion.UserEmail)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// Find the course for this lesson
	var targetCourse Course
	var foundLesson bool
	db.mu.RLock()
	for _, course := range db.Courses {
		for _, lesson := range course.Lessons {
			if lesson.ID == lessonId {
				targetCourse = course
				foundLesson = true
				break
			}
		}
		if foundLesson {
			break
		}
	}
	db.mu.RUnlock()

	if !foundLesson {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Lesson not found",
		})
	}

	// Update user progress
	progress, _ := db.GetUserProgress(user.Email)
	var courseProgress UserProgress

	for _, p := range progress {
		if p.CourseID == targetCourse.ID {
			courseProgress = p
			break
		}
	}

	if courseProgress.UserEmail == "" {
		courseProgress = UserProgress{
			UserEmail:        user.Email,
			CourseID:         targetCourse.ID,
			CompletedLessons: []string{},
			CurrentStreak:    1,
			TotalPoints:      0,
			ProficiencyLevel: "Beginner",
		}
	}

	// Add completed lesson if not already completed
	lessonCompleted := false
	for _, completedLesson := range courseProgress.CompletedLessons {
		if completedLesson == lessonId {
			lessonCompleted = true
			break
		}
	}

	if !lessonCompleted {
		courseProgress.CompletedLessons = append(courseProgress.CompletedLessons, lessonId)
		courseProgress.TotalPoints += completion.Score
		courseProgress.LastActivity = time.Now()

		// Update proficiency level based on progress
		completionPercentage := float64(len(courseProgress.CompletedLessons)) / float64(len(targetCourse.Lessons))
		switch {
		case completionPercentage >= 0.8:
			courseProgress.ProficiencyLevel = "Advanced"
		case completionPercentage >= 0.4:
			courseProgress.ProficiencyLevel = "Intermediate"
		default:
			courseProgress.ProficiencyLevel = "Beginner"
		}

		if err := db.UpdateUserProgress(courseProgress); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to update progress",
			})
		}
	}

	return c.JSON(courseProgress)
}

func startPracticeSession(c *fiber.Ctx) error {
	var session PracticeSession
	if err := c.BodyParser(&session); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user exists
	if _, err := db.GetUser(session.UserEmail); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// Validate course exists
	if _, err := db.GetCourse(session.CourseID); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Course not found",
		})
	}

	session.ID = uuid.New().String()
	session.StartedAt = time.Now()

	db.mu.Lock()
	db.PracticeSessions[session.ID] = session
	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(session)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:            make(map[string]User),
		Courses:          make(map[string]Course),
		UserProgress:     make(map[string][]UserProgress),
		PracticeSessions: make(map[string]PracticeSession),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	api.Get("/courses", getCourses)
	api.Get("/users/:email/progress", getUserProgress)
	api.Post("/lessons/:lessonId/complete", completedLesson)
	api.Post("/practice-sessions", startPracticeSession)
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

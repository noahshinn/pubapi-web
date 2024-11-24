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

type Instructor struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Title    string `json:"title"`
	Bio      string `json:"bio"`
	ImageURL string `json:"image_url"`
}

type Lesson struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Duration    int    `json:"duration"` // in minutes
	Description string `json:"description"`
	VideoURL    string `json:"video_url"`
}

type Class struct {
	ID            string     `json:"id"`
	Title         string     `json:"title"`
	Description   string     `json:"description"`
	Category      string     `json:"category"`
	Instructor    Instructor `json:"instructor"`
	Lessons       []Lesson   `json:"lessons"`
	Duration      int        `json:"duration"` // total minutes
	Rating        float64    `json:"rating"`
	StudentsCount int        `json:"students_count"`
}

type Progress struct {
	ClassID              string    `json:"class_id"`
	UserEmail            string    `json:"user_email"`
	CompletedLessons     []string  `json:"completed_lessons"`
	LastWatched          string    `json:"last_watched"`
	CompletionPercentage float64   `json:"completion_percentage"`
	LastUpdated          time.Time `json:"last_updated"`
}

type User struct {
	Email              string              `json:"email"`
	Name               string              `json:"name"`
	SubscriptionStatus string              `json:"subscription_status"`
	BookmarkedClasses  []string            `json:"bookmarked_classes"`
	Progress           map[string]Progress `json:"progress"`
}

type Database struct {
	Users   map[string]User  `json:"users"`
	Classes map[string]Class `json:"classes"`
	mu      sync.RWMutex
}

var db *Database

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:   make(map[string]User),
		Classes: make(map[string]Class),
	}

	return json.Unmarshal(data, db)
}

// Handlers
func getClasses(c *fiber.Ctx) error {
	category := c.Query("category")
	instructor := c.Query("instructor")

	db.mu.RLock()
	defer db.mu.RUnlock()

	var classes []Class
	for _, class := range db.Classes {
		if (category == "" || class.Category == category) &&
			(instructor == "" || class.Instructor.Name == instructor) {
			classes = append(classes, class)
		}
	}

	return c.JSON(classes)
}

func getClassDetails(c *fiber.Ctx) error {
	classID := c.Params("classId")

	db.mu.RLock()
	class, exists := db.Classes[classID]
	db.mu.RUnlock()

	if !exists {
		return c.Status(404).JSON(fiber.Map{
			"error": "Class not found",
		})
	}

	return c.JSON(class)
}

func getUserProgress(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Email is required",
		})
	}

	db.mu.RLock()
	user, exists := db.Users[email]
	db.mu.RUnlock()

	if !exists {
		return c.Status(404).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	var progress []Progress
	for _, p := range user.Progress {
		progress = append(progress, p)
	}

	return c.JSON(progress)
}

type ProgressUpdateRequest struct {
	ClassID   string `json:"class_id"`
	UserEmail string `json:"user_email"`
	LessonID  string `json:"lesson_id"`
	Completed bool   `json:"completed"`
}

func updateProgress(c *fiber.Ctx) error {
	var req ProgressUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	user, exists := db.Users[req.UserEmail]
	if !exists {
		return c.Status(404).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	class, exists := db.Classes[req.ClassID]
	if !exists {
		return c.Status(404).JSON(fiber.Map{
			"error": "Class not found",
		})
	}

	progress := user.Progress[req.ClassID]
	if req.Completed {
		// Add lesson to completed lessons if not already present
		found := false
		for _, lessonID := range progress.CompletedLessons {
			if lessonID == req.LessonID {
				found = true
				break
			}
		}
		if !found {
			progress.CompletedLessons = append(progress.CompletedLessons, req.LessonID)
		}
	} else {
		// Remove lesson from completed lessons
		var newCompleted []string
		for _, lessonID := range progress.CompletedLessons {
			if lessonID != req.LessonID {
				newCompleted = append(newCompleted, lessonID)
			}
		}
		progress.CompletedLessons = newCompleted
	}

	// Update completion percentage
	progress.CompletionPercentage = float64(len(progress.CompletedLessons)) / float64(len(class.Lessons)) * 100
	progress.LastWatched = req.LessonID
	progress.LastUpdated = time.Now()

	user.Progress[req.ClassID] = progress
	db.Users[req.UserEmail] = user

	return c.JSON(progress)
}

func getBookmarks(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Email is required",
		})
	}

	db.mu.RLock()
	user, exists := db.Users[email]
	db.mu.RUnlock()

	if !exists {
		return c.Status(404).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	var bookmarkedClasses []Class
	for _, classID := range user.BookmarkedClasses {
		if class, exists := db.Classes[classID]; exists {
			bookmarkedClasses = append(bookmarkedClasses, class)
		}
	}

	return c.JSON(bookmarkedClasses)
}

type BookmarkUpdateRequest struct {
	ClassID    string `json:"class_id"`
	UserEmail  string `json:"user_email"`
	Bookmarked bool   `json:"bookmarked"`
}

func updateBookmark(c *fiber.Ctx) error {
	var req BookmarkUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	user, exists := db.Users[req.UserEmail]
	if !exists {
		return c.Status(404).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	if _, exists := db.Classes[req.ClassID]; !exists {
		return c.Status(404).JSON(fiber.Map{
			"error": "Class not found",
		})
	}

	if req.Bookmarked {
		// Add bookmark if not already present
		found := false
		for _, classID := range user.BookmarkedClasses {
			if classID == req.ClassID {
				found = true
				break
			}
		}
		if !found {
			user.BookmarkedClasses = append(user.BookmarkedClasses, req.ClassID)
		}
	} else {
		// Remove bookmark
		var newBookmarks []string
		for _, classID := range user.BookmarkedClasses {
			if classID != req.ClassID {
				newBookmarks = append(newBookmarks, classID)
			}
		}
		user.BookmarkedClasses = newBookmarks
	}

	db.Users[req.UserEmail] = user

	return c.JSON(fiber.Map{
		"success":            true,
		"bookmarked_classes": user.BookmarkedClasses,
	})
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

	// Class routes
	api.Get("/classes", getClasses)
	api.Get("/classes/:classId", getClassDetails)

	// Progress routes
	api.Get("/progress", getUserProgress)
	api.Post("/progress", updateProgress)

	// Bookmark routes
	api.Get("/bookmarks", getBookmarks)
	api.Post("/bookmarks", updateBookmark)
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
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE",
		AllowHeaders: "Origin, Content-Type, Accept",
	}))

	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

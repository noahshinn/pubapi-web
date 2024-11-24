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

type Content struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Type         string `json:"type"` // movie or series
	Genre        string `json:"genre"`
	ReleaseYear  int    `json:"release_year"`
	Rating       string `json:"rating"`
	Duration     int    `json:"duration"` // in minutes
	Description  string `json:"description"`
	ThumbnailURL string `json:"thumbnail_url"`
	StreamURL    string `json:"stream_url"`
}

type User struct {
	Email        string         `json:"email"`
	Name         string         `json:"name"`
	Subscription string         `json:"subscription"`
	WatchList    []string       `json:"watchlist"` // Content IDs
	WatchHistory []WatchHistory `json:"watch_history"`
	Preferences  Preferences    `json:"preferences"`
}

type WatchHistory struct {
	ContentID string    `json:"content_id"`
	WatchedAt time.Time `json:"watched_at"`
	Completed bool      `json:"completed"`
}

type WatchProgress struct {
	ContentID       string    `json:"content_id"`
	ProgressSeconds int       `json:"progress_seconds"`
	LastWatched     time.Time `json:"last_watched"`
}

type Preferences struct {
	PreferredGenres []string `json:"preferred_genres"`
	Language        string   `json:"language"`
	Subtitles       bool     `json:"subtitles"`
}

type Database struct {
	Users         map[string]User          `json:"users"`
	Content       map[string]Content       `json:"content"`
	WatchProgress map[string]WatchProgress `json:"watch_progress"` // key: email:content_id
	mu            sync.RWMutex
}

var db *Database

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:         make(map[string]User),
		Content:       make(map[string]Content),
		WatchProgress: make(map[string]WatchProgress),
	}

	return json.Unmarshal(data, db)
}

func getContent(c *fiber.Ctx) error {
	contentType := c.Query("type", "all")
	genre := c.Query("genre")

	var filteredContent []Content
	db.mu.RLock()
	for _, content := range db.Content {
		if (contentType == "all" || content.Type == contentType) &&
			(genre == "" || content.Genre == genre) {
			filteredContent = append(filteredContent, content)
		}
	}
	db.mu.RUnlock()

	return c.JSON(filteredContent)
}

func getWatchlist(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	db.mu.RLock()
	user, exists := db.Users[email]
	if !exists {
		db.mu.RUnlock()
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "user not found",
		})
	}

	var watchlist []Content
	for _, contentID := range user.WatchList {
		if content, exists := db.Content[contentID]; exists {
			watchlist = append(watchlist, content)
		}
	}
	db.mu.RUnlock()

	return c.JSON(watchlist)
}

func addToWatchlist(c *fiber.Ctx) error {
	var req struct {
		Email     string `json:"email"`
		ContentID string `json:"content_id"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	user, exists := db.Users[req.Email]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "user not found",
		})
	}

	if _, exists := db.Content[req.ContentID]; !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "content not found",
		})
	}

	// Check if content is already in watchlist
	for _, id := range user.WatchList {
		if id == req.ContentID {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "content already in watchlist",
			})
		}
	}

	user.WatchList = append(user.WatchList, req.ContentID)
	db.Users[req.Email] = user

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "added to watchlist",
	})
}

func getWatchHistory(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	db.mu.RLock()
	user, exists := db.Users[email]
	if !exists {
		db.mu.RUnlock()
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "user not found",
		})
	}

	var history []map[string]interface{}
	for _, watch := range user.WatchHistory {
		if content, exists := db.Content[watch.ContentID]; exists {
			history = append(history, map[string]interface{}{
				"content":    content,
				"watched_at": watch.WatchedAt,
				"completed":  watch.Completed,
			})
		}
	}
	db.mu.RUnlock()

	return c.JSON(history)
}

func getContinueWatching(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	if _, exists := db.Users[email]; !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "user not found",
		})
	}

	var inProgress []map[string]interface{}
	for key, progress := range db.WatchProgress {
		if key[:len(email)] == email {
			contentID := key[len(email)+1:]
			if content, exists := db.Content[contentID]; exists {
				inProgress = append(inProgress, map[string]interface{}{
					"content":          content,
					"progress_seconds": progress.ProgressSeconds,
					"last_watched":     progress.LastWatched,
				})
			}
		}
	}

	return c.JSON(inProgress)
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

	api.Get("/content", getContent)
	api.Get("/watchlist", getWatchlist)
	api.Post("/watchlist", addToWatchlist)
	api.Get("/watch-history", getWatchHistory)
	api.Get("/continue-watching", getContinueWatching)
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

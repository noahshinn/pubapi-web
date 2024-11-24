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
)

type Content struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Type         string `json:"type"`
	Genre        string `json:"genre"`
	Description  string `json:"description"`
	ReleaseDate  string `json:"release_date"`
	Rating       string `json:"rating"`
	Duration     int    `json:"duration"`
	ThumbnailURL string `json:"thumbnail_url"`
	StreamURL    string `json:"stream_url"`
}

type WatchProgress struct {
	Email           string    `json:"email"`
	ContentID       string    `json:"content_id"`
	ProgressSeconds int       `json:"progress_seconds"`
	TotalSeconds    int       `json:"total_seconds"`
	LastWatched     time.Time `json:"last_watched"`
}

type User struct {
	Email           string          `json:"email"`
	Name            string          `json:"name"`
	Subscription    string          `json:"subscription"`
	WatchList       []string        `json:"watchlist"`
	WatchProgress   []WatchProgress `json:"watch_progress"`
	PreferredGenres []string        `json:"preferred_genres"`
}

type Database struct {
	Users   map[string]User    `json:"users"`
	Content map[string]Content `json:"content"`
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
		Content: make(map[string]Content),
	}

	return json.Unmarshal(data, db)
}

func getCatalog(c *fiber.Ctx) error {
	contentType := c.Query("type")
	genre := c.Query("genre")

	db.mu.RLock()
	defer db.mu.RUnlock()

	var filteredContent []Content
	for _, content := range db.Content {
		if (contentType == "" || content.Type == contentType) &&
			(genre == "" || content.Genre == genre) {
			filteredContent = append(filteredContent, content)
		}
	}

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

func getContinueWatching(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	user, exists := db.Users[email]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "user not found",
		})
	}

	var continueWatching []map[string]interface{}
	for _, progress := range user.WatchProgress {
		if content, exists := db.Content[progress.ContentID]; exists {
			if progress.ProgressSeconds < progress.TotalSeconds {
				continueWatching = append(continueWatching, map[string]interface{}{
					"content":  content,
					"progress": progress,
				})
			}
		}
	}

	return c.JSON(continueWatching)
}

func recordWatchProgress(c *fiber.Ctx) error {
	var progress WatchProgress
	if err := c.BodyParser(&progress); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	user, exists := db.Users[progress.Email]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "user not found",
		})
	}

	if _, exists := db.Content[progress.ContentID]; !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "content not found",
		})
	}

	progress.LastWatched = time.Now()

	// Update or add new progress
	found := false
	for i, p := range user.WatchProgress {
		if p.ContentID == progress.ContentID {
			user.WatchProgress[i] = progress
			found = true
			break
		}
	}

	if !found {
		user.WatchProgress = append(user.WatchProgress, progress)
	}

	db.Users[progress.Email] = user

	return c.JSON(progress)
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

	// Content routes
	api.Get("/catalog", getCatalog)

	// Watchlist routes
	api.Get("/watchlist", getWatchlist)
	api.Post("/watchlist", addToWatchlist)

	// Continue watching routes
	api.Get("/continue-watching", getContinueWatching)
	api.Post("/watch", recordWatchProgress)
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

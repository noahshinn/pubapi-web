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
)

type Content struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Type         string `json:"type"` // movie or series
	Description  string `json:"description"`
	ReleaseYear  int    `json:"release_year"`
	Rating       string `json:"rating"`
	Duration     int    `json:"duration"` // in minutes
	Genre        string `json:"genre"`
	ThumbnailURL string `json:"thumbnail_url"`
	StreamURL    string `json:"stream_url"`
	Seasons      int    `json:"seasons,omitempty"`
}

type WatchProgress struct {
	ContentID       string    `json:"content_id"`
	ProgressSeconds int       `json:"progress_seconds"`
	TotalSeconds    int       `json:"total_seconds"`
	LastWatched     time.Time `json:"last_watched"`
	Season          int       `json:"season,omitempty"`
	Episode         int       `json:"episode,omitempty"`
}

type Profile struct {
	Email       string `json:"email"`
	Name        string `json:"name"`
	Avatar      string `json:"avatar"`
	Preferences struct {
		Autoplay         bool   `json:"autoplay"`
		SubtitleLanguage string `json:"subtitle_language"`
		AudioLanguage    string `json:"audio_language"`
	} `json:"preferences"`
	Watchlist     []string        `json:"watchlist"`
	WatchProgress []WatchProgress `json:"watch_progress"`
}

type Database struct {
	Profiles map[string]Profile `json:"profiles"`
	Content  map[string]Content `json:"content"`
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
		Content:  make(map[string]Content),
	}

	return json.Unmarshal(data, db)
}

// Handlers
func getFeaturedContent(c *fiber.Ctx) error {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var featured []Content
	for _, content := range db.Content {
		// In a real implementation, this would use more sophisticated logic
		// to determine featured content
		featured = append(featured, content)
		if len(featured) >= 10 {
			break
		}
	}

	return c.JSON(featured)
}

func searchContent(c *fiber.Ctx) error {
	query := c.Query("query")
	if query == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Search query is required",
		})
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	var results []Content
	for _, content := range db.Content {
		// Simple case-insensitive substring search
		// In a real implementation, this would use more sophisticated search
		if containsIgnoreCase(content.Title, query) ||
			containsIgnoreCase(content.Description, query) {
			results = append(results, content)
		}
	}

	return c.JSON(results)
}

func getWatchlist(c *fiber.Ctx) error {
	email := c.Params("email")

	db.mu.RLock()
	profile, exists := db.Profiles[email]
	if !exists {
		db.mu.RUnlock()
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Profile not found",
		})
	}

	var watchlist []Content
	for _, contentID := range profile.Watchlist {
		if content, exists := db.Content[contentID]; exists {
			watchlist = append(watchlist, content)
		}
	}
	db.mu.RUnlock()

	return c.JSON(watchlist)
}

func addToWatchlist(c *fiber.Ctx) error {
	email := c.Params("email")

	var req struct {
		ContentID string `json:"content_id"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	profile, exists := db.Profiles[email]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Profile not found",
		})
	}

	if _, exists := db.Content[req.ContentID]; !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Content not found",
		})
	}

	// Check if content is already in watchlist
	for _, id := range profile.Watchlist {
		if id == req.ContentID {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Content already in watchlist",
			})
		}
	}

	profile.Watchlist = append(profile.Watchlist, req.ContentID)
	db.Profiles[email] = profile

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Added to watchlist",
	})
}

func getContinueWatching(c *fiber.Ctx) error {
	email := c.Params("email")

	db.mu.RLock()
	defer db.mu.RUnlock()

	profile, exists := db.Profiles[email]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Profile not found",
		})
	}

	var continueWatching []map[string]interface{}
	for _, progress := range profile.WatchProgress {
		if content, exists := db.Content[progress.ContentID]; exists {
			if progress.ProgressSeconds < progress.TotalSeconds {
				item := map[string]interface{}{
					"content":  content,
					"progress": progress,
				}
				continueWatching = append(continueWatching, item)
			}
		}
	}

	return c.JSON(continueWatching)
}

func updateWatchProgress(c *fiber.Ctx) error {
	email := c.Params("email")

	var progress WatchProgress
	if err := c.BodyParser(&progress); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	profile, exists := db.Profiles[email]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Profile not found",
		})
	}

	if _, exists := db.Content[progress.ContentID]; !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Content not found",
		})
	}

	// Update or add progress
	found := false
	for i, p := range profile.WatchProgress {
		if p.ContentID == progress.ContentID {
			profile.WatchProgress[i] = progress
			found = true
			break
		}
	}

	if !found {
		profile.WatchProgress = append(profile.WatchProgress, progress)
	}

	db.Profiles[email] = profile

	return c.JSON(progress)
}

func containsIgnoreCase(s, substr string) bool {
	s, substr = strings.ToLower(s), strings.ToLower(substr)
	return strings.Contains(s, substr)
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
	api.Get("/content/featured", getFeaturedContent)
	api.Get("/content/search", searchContent)

	// Profile routes
	api.Get("/profiles/:email/watchlist", getWatchlist)
	api.Post("/profiles/:email/watchlist", addToWatchlist)
	api.Get("/profiles/:email/continue-watching", getContinueWatching)
	api.Post("/profiles/:email/watch-progress", updateWatchProgress)
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

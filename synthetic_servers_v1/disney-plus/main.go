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

// Domain Models
type ContentType string
type Category string

const (
	MovieType  ContentType = "movie"
	SeriesType ContentType = "series"

	DisneyCategory   Category = "disney"
	PixarCategory    Category = "pixar"
	MarvelCategory   Category = "marvel"
	StarWarsCategory Category = "starwars"
	NatGeoCategory   Category = "national-geographic"
)

type Content struct {
	ID           string      `json:"id"`
	Title        string      `json:"title"`
	Type         ContentType `json:"type"`
	Category     Category    `json:"category"`
	ReleaseYear  int         `json:"release_year"`
	Rating       string      `json:"rating"`
	Duration     int         `json:"duration"` // in seconds
	Description  string      `json:"description"`
	ThumbnailURL string      `json:"thumbnail_url"`
	StreamURL    string      `json:"stream_url"`
}

type Profile struct {
	ID        string `json:"id"`
	UserEmail string `json:"user_email"`
	Name      string `json:"name"`
	Avatar    string `json:"avatar"`
	KidsMode  bool   `json:"kids_mode"`
}

type WatchProgress struct {
	ContentID       string    `json:"content_id"`
	ProfileID       string    `json:"profile_id"`
	ProgressSeconds int       `json:"progress_seconds"`
	TotalSeconds    int       `json:"total_seconds"`
	LastWatched     time.Time `json:"last_watched"`
}

type User struct {
	Email            string    `json:"email"`
	Name             string    `json:"name"`
	SubscriptionTier string    `json:"subscription_tier"`
	SubscriptionEnd  time.Time `json:"subscription_end"`
}

// Database represents our in-memory database
type Database struct {
	Users         map[string]User          `json:"users"`
	Content       map[string]Content       `json:"content"`
	Profiles      map[string]Profile       `json:"profiles"`
	WatchProgress map[string]WatchProgress `json:"watch_progress"`
	Watchlist     map[string][]string      `json:"watchlist"` // profile_id -> []content_id
	mu            sync.RWMutex
}

var db *Database

// Handlers
func getContent(c *fiber.Ctx) error {
	category := c.Query("category")
	page := c.QueryInt("page", 1)
	itemsPerPage := 20

	var filteredContent []Content
	db.mu.RLock()
	for _, content := range db.Content {
		if category == "" || Category(category) == content.Category {
			filteredContent = append(filteredContent, content)
		}
	}
	db.mu.RUnlock()

	// Calculate pagination
	start := (page - 1) * itemsPerPage
	end := start + itemsPerPage
	if end > len(filteredContent) {
		end = len(filteredContent)
	}
	if start >= len(filteredContent) {
		return c.JSON([]Content{})
	}

	return c.JSON(filteredContent[start:end])
}

func getProfiles(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	var userProfiles []Profile
	db.mu.RLock()
	for _, profile := range db.Profiles {
		if profile.UserEmail == email {
			userProfiles = append(userProfiles, profile)
		}
	}
	db.mu.RUnlock()

	return c.JSON(userProfiles)
}

func getWatchlist(c *fiber.Ctx) error {
	profileID := c.Query("profile_id")
	if profileID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "profile_id is required",
		})
	}

	db.mu.RLock()
	contentIDs, exists := db.Watchlist[profileID]
	if !exists {
		db.mu.RUnlock()
		return c.JSON([]Content{})
	}

	var watchlist []Content
	for _, contentID := range contentIDs {
		if content, exists := db.Content[contentID]; exists {
			watchlist = append(watchlist, content)
		}
	}
	db.mu.RUnlock()

	return c.JSON(watchlist)
}

func addToWatchlist(c *fiber.Ctx) error {
	var req struct {
		ProfileID string `json:"profile_id"`
		ContentID string `json:"content_id"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	// Verify content exists
	if _, exists := db.Content[req.ContentID]; !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Content not found",
		})
	}

	// Verify profile exists
	if _, exists := db.Profiles[req.ProfileID]; !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Profile not found",
		})
	}

	// Add to watchlist if not already present
	for _, contentID := range db.Watchlist[req.ProfileID] {
		if contentID == req.ContentID {
			return c.Status(fiber.StatusOK).JSON(fiber.Map{
				"message": "Content already in watchlist",
			})
		}
	}

	db.Watchlist[req.ProfileID] = append(db.Watchlist[req.ProfileID], req.ContentID)
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Added to watchlist",
	})
}

func getContinueWatching(c *fiber.Ctx) error {
	profileID := c.Query("profile_id")
	if profileID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "profile_id is required",
		})
	}

	var progress []struct {
		Content       Content       `json:"content"`
		WatchProgress WatchProgress `json:"progress"`
	}

	db.mu.RLock()
	for _, wp := range db.WatchProgress {
		if wp.ProfileID == profileID {
			if content, exists := db.Content[wp.ContentID]; exists {
				progress = append(progress, struct {
					Content       Content       `json:"content"`
					WatchProgress WatchProgress `json:"progress"`
				}{
					Content:       content,
					WatchProgress: wp,
				})
			}
		}
	}
	db.mu.RUnlock()

	return c.JSON(progress)
}

func updateWatchProgress(c *fiber.Ctx) error {
	var req struct {
		ProfileID       string `json:"profile_id"`
		ContentID       string `json:"content_id"`
		ProgressSeconds int    `json:"progress_seconds"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	// Verify content exists
	content, exists := db.Content[req.ContentID]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Content not found",
		})
	}

	progressKey := req.ProfileID + ":" + req.ContentID
	progress := WatchProgress{
		ContentID:       req.ContentID,
		ProfileID:       req.ProfileID,
		ProgressSeconds: req.ProgressSeconds,
		TotalSeconds:    content.Duration,
		LastWatched:     time.Now(),
	}

	db.WatchProgress[progressKey] = progress
	return c.JSON(progress)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:         make(map[string]User),
		Content:       make(map[string]Content),
		Profiles:      make(map[string]Profile),
		WatchProgress: make(map[string]WatchProgress),
		Watchlist:     make(map[string][]string),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Content routes
	api.Get("/content", getContent)

	// Profile routes
	api.Get("/profiles", getProfiles)

	// Watchlist routes
	api.Get("/watchlist", getWatchlist)
	api.Post("/watchlist", addToWatchlist)

	// Continue watching routes
	api.Get("/continue-watching", getContinueWatching)
	api.Post("/watch-progress", updateWatchProgress)
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

	// Start server
	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

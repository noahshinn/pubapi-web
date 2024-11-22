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
type ContentType string

const (
	ContentTypeMovie ContentType = "movie"
	ContentTypeShow  ContentType = "show"
	ContentTypeSport ContentType = "sport"
	ContentTypeNews  ContentType = "news"
)

type Content struct {
	ID           string      `json:"id"`
	Title        string      `json:"title"`
	Type         ContentType `json:"type"`
	Description  string      `json:"description"`
	Duration     int         `json:"duration"`
	ReleaseYear  int         `json:"release_year"`
	Rating       string      `json:"rating"`
	Genres       []string    `json:"genres"`
	ThumbnailURL string      `json:"thumbnail_url"`
	StreamURL    string      `json:"stream_url"`
	Episodes     []Episode   `json:"episodes,omitempty"`
}

type Episode struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	SeasonNum    int    `json:"season_num"`
	EpisodeNum   int    `json:"episode_num"`
	Duration     int    `json:"duration"`
	Description  string `json:"description"`
	ThumbnailURL string `json:"thumbnail_url"`
	StreamURL    string `json:"stream_url"`
}

type WatchProgress struct {
	ContentID       string    `json:"content_id"`
	EpisodeID       string    `json:"episode_id,omitempty"`
	Email           string    `json:"email"`
	ProgressSeconds int       `json:"progress_seconds"`
	TotalSeconds    int       `json:"total_seconds"`
	LastWatched     time.Time `json:"last_watched"`
}

type User struct {
	Email            string          `json:"email"`
	Name             string          `json:"name"`
	SubscriptionTier string          `json:"subscription_tier"`
	Watchlist        []string        `json:"watchlist"`
	WatchProgress    []WatchProgress `json:"watch_progress"`
}

// Database represents our in-memory database
type Database struct {
	Users    map[string]User          `json:"users"`
	Content  map[string]Content       `json:"content"`
	Progress map[string]WatchProgress `json:"progress"`
	mu       sync.RWMutex
}

// Custom errors
var (
	ErrUserNotFound    = errors.New("user not found")
	ErrContentNotFound = errors.New("content not found")
	ErrInvalidInput    = errors.New("invalid input")
)

// Global database instance
var db *Database

// Database operations
func (d *Database) GetUser(email string) (User, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	user, exists := d.Users[email]
	if !exists {
		return User{}, ErrUserNotFound
	}
	return user, nil
}

func (d *Database) GetContent(id string) (Content, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	content, exists := d.Content[id]
	if !exists {
		return Content{}, ErrContentNotFound
	}
	return content, nil
}

func (d *Database) AddToWatchlist(email, contentID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	user, exists := d.Users[email]
	if !exists {
		return ErrUserNotFound
	}

	// Check if content exists
	if _, exists := d.Content[contentID]; !exists {
		return ErrContentNotFound
	}

	// Check if already in watchlist
	for _, id := range user.Watchlist {
		if id == contentID {
			return nil
		}
	}

	user.Watchlist = append(user.Watchlist, contentID)
	d.Users[email] = user
	return nil
}

func (d *Database) UpdateWatchProgress(progress WatchProgress) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := progress.Email + ":" + progress.ContentID
	if progress.EpisodeID != "" {
		key += ":" + progress.EpisodeID
	}

	d.Progress[key] = progress
	return nil
}

// HTTP Handlers
func getCatalog(c *fiber.Ctx) error {
	category := c.Query("category")
	genre := c.Query("genre")

	var filtered []Content

	db.mu.RLock()
	for _, content := range db.Content {
		if category != "" && string(content.Type) != category {
			continue
		}

		if genre != "" {
			hasGenre := false
			for _, g := range content.Genres {
				if g == genre {
					hasGenre = true
					break
				}
			}
			if !hasGenre {
				continue
			}
		}

		filtered = append(filtered, content)
	}
	db.mu.RUnlock()

	return c.JSON(filtered)
}

func getWatchlist(c *fiber.Ctx) error {
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

	var watchlistContent []Content
	for _, contentID := range user.Watchlist {
		content, err := db.GetContent(contentID)
		if err == nil {
			watchlistContent = append(watchlistContent, content)
		}
	}

	return c.JSON(watchlistContent)
}

func addToWatchlist(c *fiber.Ctx) error {
	var req struct {
		Email     string `json:"email"`
		ContentID string `json:"content_id"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if err := db.AddToWatchlist(req.Email, req.ContentID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.SendStatus(fiber.StatusCreated)
}

func getContinueWatching(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	var progress []WatchProgress
	db.mu.RLock()
	for _, p := range db.Progress {
		if p.Email == email {
			progress = append(progress, p)
		}
	}
	db.mu.RUnlock()

	return c.JSON(progress)
}

func updateWatchProgress(c *fiber.Ctx) error {
	var progress WatchProgress
	if err := c.BodyParser(&progress); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	progress.LastWatched = time.Now()

	if err := db.UpdateWatchProgress(progress); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.SendStatus(fiber.StatusOK)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:    make(map[string]User),
		Content:  make(map[string]Content),
		Progress: make(map[string]WatchProgress),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	api.Get("/catalog", getCatalog)
	api.Get("/watchlist", getWatchlist)
	api.Post("/watchlist", addToWatchlist)
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

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

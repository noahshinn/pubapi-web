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
	ContentTypeMovie  ContentType = "movie"
	ContentTypeSeries ContentType = "series"
)

type Content struct {
	ID           string      `json:"id"`
	Title        string      `json:"title"`
	Type         ContentType `json:"type"`
	Description  string      `json:"description"`
	ReleaseYear  int         `json:"releaseYear"`
	Genre        string      `json:"genre"`
	Rating       string      `json:"rating"`
	Duration     int         `json:"duration"` // in minutes
	ThumbnailURL string      `json:"thumbnailUrl"`
	StreamURL    string      `json:"streamUrl"`
	Episodes     []Episode   `json:"episodes,omitempty"`
}

type Episode struct {
	ID           string    `json:"id"`
	Season       int       `json:"season"`
	Episode      int       `json:"episode"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	Duration     int       `json:"duration"`
	StreamURL    string    `json:"streamUrl"`
	ThumbnailURL string    `json:"thumbnailUrl"`
	AirDate      time.Time `json:"airDate"`
}

type User struct {
	Email            string          `json:"email"`
	Name             string          `json:"name"`
	SubscriptionPlan string          `json:"subscriptionPlan"`
	Watchlist        []string        `json:"watchlist"` // Content IDs
	WatchProgress    []WatchProgress `json:"watchProgress"`
	Preferences      UserPreferences `json:"preferences"`
}

type UserPreferences struct {
	SubtitleLanguage string   `json:"subtitleLanguage"`
	AudioLanguage    string   `json:"audioLanguage"`
	MaturityRating   string   `json:"maturityRating"`
	FavoriteGenres   []string `json:"favoriteGenres"`
}

type WatchProgress struct {
	ContentID   string    `json:"contentId"`
	Progress    int       `json:"progress"` // in seconds
	LastWatched time.Time `json:"lastWatched"`
	Season      int       `json:"season,omitempty"`
	Episode     int       `json:"episode,omitempty"`
}

// Database represents our in-memory database
type Database struct {
	Users   map[string]User    `json:"users"`
	Content map[string]Content `json:"content"`
	mu      sync.RWMutex
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

func (d *Database) GetContent(id string) (Content, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	content, exists := d.Content[id]
	if !exists {
		return Content{}, errors.New("content not found")
	}
	return content, nil
}

func (d *Database) AddToWatchlist(email, contentId string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	user, exists := d.Users[email]
	if !exists {
		return errors.New("user not found")
	}

	// Check if content exists
	if _, exists := d.Content[contentId]; !exists {
		return errors.New("content not found")
	}

	// Check if already in watchlist
	for _, id := range user.Watchlist {
		if id == contentId {
			return nil
		}
	}

	user.Watchlist = append(user.Watchlist, contentId)
	d.Users[email] = user
	return nil
}

func (d *Database) UpdateWatchProgress(email string, progress WatchProgress) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	user, exists := d.Users[email]
	if !exists {
		return errors.New("user not found")
	}

	// Update or add progress
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

	d.Users[email] = user
	return nil
}

// HTTP Handlers
func browseContent(c *fiber.Ctx) error {
	category := c.Query("category")
	genre := c.Query("genre")
	page := c.QueryInt("page", 1)
	pageSize := 20

	var filteredContent []Content
	db.mu.RLock()
	for _, content := range db.Content {
		if (category == "" || string(content.Type) == category) &&
			(genre == "" || content.Genre == genre) {
			filteredContent = append(filteredContent, content)
		}
	}
	db.mu.RUnlock()

	// Simple pagination
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > len(filteredContent) {
		end = len(filteredContent)
	}
	if start >= len(filteredContent) {
		return c.JSON([]Content{})
	}

	return c.JSON(filteredContent[start:end])
}

func getContentDetails(c *fiber.Ctx) error {
	contentId := c.Params("contentId")

	content, err := db.GetContent(contentId)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Content not found",
		})
	}

	return c.JSON(content)
}

func getWatchlist(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email is required",
		})
	}

	user, err := db.GetUser(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	var watchlistContent []Content
	for _, contentId := range user.Watchlist {
		if content, err := db.GetContent(contentId); err == nil {
			watchlistContent = append(watchlistContent, content)
		}
	}

	return c.JSON(watchlistContent)
}

func addToWatchlist(c *fiber.Ctx) error {
	var req struct {
		Email     string `json:"email"`
		ContentID string `json:"contentId"`
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
			"error": "Email is required",
		})
	}

	user, err := db.GetUser(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	var continueWatching []map[string]interface{}
	for _, progress := range user.WatchProgress {
		content, err := db.GetContent(progress.ContentID)
		if err != nil {
			continue
		}

		watching := map[string]interface{}{
			"content":     content,
			"progress":    progress.Progress,
			"lastWatched": progress.LastWatched,
		}

		if content.Type == ContentTypeSeries {
			watching["episode"] = map[string]interface{}{
				"season":  progress.Season,
				"episode": progress.Episode,
			}
		}

		continueWatching = append(continueWatching, watching)
	}

	return c.JSON(continueWatching)
}

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

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Content routes
	api.Get("/content/browse", browseContent)
	api.Get("/content/:contentId", getContentDetails)

	// Watchlist routes
	api.Get("/watchlist", getWatchlist)
	api.Post("/watchlist", addToWatchlist)

	// Continue watching routes
	api.Get("/continue-watching", getContinueWatching)
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

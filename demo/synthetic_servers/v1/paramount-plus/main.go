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
type User struct {
	Email            string    `json:"email"`
	Name             string    `json:"name"`
	SubscriptionTier string    `json:"subscription_tier"`
	JoinDate         time.Time `json:"join_date"`
}

type Content struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Type         string    `json:"type"` // movie, show, sports, news
	Genre        string    `json:"genre"`
	Description  string    `json:"description"`
	Duration     int       `json:"duration"` // in seconds
	Rating       string    `json:"rating"`
	ReleaseYear  int       `json:"release_year"`
	ThumbnailURL string    `json:"thumbnail_url"`
	StreamURL    string    `json:"stream_url"`
	Episodes     []Episode `json:"episodes,omitempty"`
}

type Episode struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	SeasonNum   int       `json:"season_num"`
	EpisodeNum  int       `json:"episode_num"`
	Duration    int       `json:"duration"`
	Description string    `json:"description"`
	StreamURL   string    `json:"stream_url"`
	AirDate     time.Time `json:"air_date"`
}

type WatchlistItem struct {
	ContentID string    `json:"content_id"`
	UserEmail string    `json:"user_email"`
	AddedAt   time.Time `json:"added_at"`
}

type WatchProgress struct {
	ContentID   string    `json:"content_id"`
	UserEmail   string    `json:"user_email"`
	Position    int       `json:"position"` // in seconds
	Duration    int       `json:"duration"`
	LastWatched time.Time `json:"last_watched"`
}

// Database represents our in-memory database
type Database struct {
	Users         map[string]User            `json:"users"`
	Content       map[string]Content         `json:"content"`
	Watchlist     map[string][]WatchlistItem `json:"watchlist"`
	WatchProgress map[string][]WatchProgress `json:"watch_progress"`
	mu            sync.RWMutex
}

var db *Database

// Database operations
func (d *Database) GetUser(email string) (User, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	user, exists := d.Users[email]
	if !exists {
		return User{}, fiber.NewError(fiber.StatusNotFound, "User not found")
	}
	return user, nil
}

func (d *Database) GetContent(contentId string) (Content, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	content, exists := d.Content[contentId]
	if !exists {
		return Content{}, fiber.NewError(fiber.StatusNotFound, "Content not found")
	}
	return content, nil
}

func (d *Database) GetWatchlist(email string) []WatchlistItem {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.Watchlist[email]
}

func (d *Database) AddToWatchlist(item WatchlistItem) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Watchlist[item.UserEmail] = append(d.Watchlist[item.UserEmail], item)
	return nil
}

func (d *Database) UpdateWatchProgress(progress WatchProgress) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Find and update existing progress or add new
	progressList := d.WatchProgress[progress.UserEmail]
	found := false
	for i, p := range progressList {
		if p.ContentID == progress.ContentID {
			progressList[i] = progress
			found = true
			break
		}
	}

	if !found {
		progressList = append(progressList, progress)
	}

	d.WatchProgress[progress.UserEmail] = progressList
	return nil
}

// HTTP Handlers
func getCatalog(c *fiber.Ctx) error {
	category := c.Query("category")
	genre := c.Query("genre")

	var filteredContent []Content

	db.mu.RLock()
	for _, content := range db.Content {
		if (category == "" || content.Type == category) &&
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
		return fiber.NewError(fiber.StatusBadRequest, "Email is required")
	}

	// Verify user exists
	if _, err := db.GetUser(email); err != nil {
		return err
	}

	watchlist := db.GetWatchlist(email)

	// Enhance watchlist with content details
	var enhancedWatchlist []map[string]interface{}
	for _, item := range watchlist {
		content, err := db.GetContent(item.ContentID)
		if err != nil {
			continue
		}

		enhanced := map[string]interface{}{
			"content":  content,
			"added_at": item.AddedAt,
		}
		enhancedWatchlist = append(enhancedWatchlist, enhanced)
	}

	return c.JSON(enhancedWatchlist)
}

func addToWatchlist(c *fiber.Ctx) error {
	var item WatchlistItem
	if err := c.BodyParser(&item); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	// Verify user exists
	if _, err := db.GetUser(item.UserEmail); err != nil {
		return err
	}

	// Verify content exists
	if _, err := db.GetContent(item.ContentID); err != nil {
		return err
	}

	item.AddedAt = time.Now()
	if err := db.AddToWatchlist(item); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to add to watchlist")
	}

	return c.SendStatus(fiber.StatusCreated)
}

func getContinueWatching(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Email is required")
	}

	// Verify user exists
	if _, err := db.GetUser(email); err != nil {
		return err
	}

	db.mu.RLock()
	progressList := db.WatchProgress[email]
	db.mu.RUnlock()

	var enhancedProgress []map[string]interface{}
	for _, progress := range progressList {
		content, err := db.GetContent(progress.ContentID)
		if err != nil {
			continue
		}

		// Only include if not finished (less than 90% watched)
		if float64(progress.Position)/float64(progress.Duration) < 0.9 {
			enhanced := map[string]interface{}{
				"content":      content,
				"position":     progress.Position,
				"duration":     progress.Duration,
				"last_watched": progress.LastWatched,
			}
			enhancedProgress = append(enhancedProgress, enhanced)
		}
	}

	return c.JSON(enhancedProgress)
}

func updateWatchProgress(c *fiber.Ctx) error {
	contentId := c.Params("contentId")
	var progress WatchProgress
	if err := c.BodyParser(&progress); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	// Verify user exists
	if _, err := db.GetUser(progress.UserEmail); err != nil {
		return err
	}

	// Verify content exists
	content, err := db.GetContent(contentId)
	if err != nil {
		return err
	}

	progress.ContentID = contentId
	progress.LastWatched = time.Now()
	progress.Duration = content.Duration

	if err := db.UpdateWatchProgress(progress); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to update watch progress")
	}

	return c.SendStatus(fiber.StatusOK)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:         make(map[string]User),
		Content:       make(map[string]Content),
		Watchlist:     make(map[string][]WatchlistItem),
		WatchProgress: make(map[string][]WatchProgress),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	api.Get("/catalog", getCatalog)
	api.Get("/watchlist", getWatchlist)
	api.Post("/watchlist", addToWatchlist)
	api.Get("/continue-watching", getContinueWatching)
	api.Post("/watch/:contentId", updateWatchProgress)
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

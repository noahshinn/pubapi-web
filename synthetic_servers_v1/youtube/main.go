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
type User struct {
	Email         string     `json:"email"`
	Name          string     `json:"name"`
	Avatar        string     `json:"avatar"`
	Subscriptions []Channel  `json:"subscriptions"`
	Playlists     []Playlist `json:"playlists"`
	WatchHistory  []Video    `json:"watch_history"`
	JoinedAt      time.Time  `json:"joined_at"`
}

type Channel struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Subscribers int       `json:"subscribers"`
	AvatarURL   string    `json:"avatar_url"`
	Verified    bool      `json:"verified"`
	CreatedAt   time.Time `json:"created_at"`
}

type Video struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	ThumbnailURL string    `json:"thumbnail_url"`
	Channel      Channel   `json:"channel"`
	Views        int       `json:"views"`
	Likes        int       `json:"likes"`
	Duration     int       `json:"duration"`
	PublishedAt  time.Time `json:"published_at"`
}

type Playlist struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Visibility  string    `json:"visibility"`
	VideoCount  int       `json:"video_count"`
	Videos      []Video   `json:"videos"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Database represents our in-memory database
type Database struct {
	Users     map[string]User     `json:"users"`
	Videos    map[string]Video    `json:"videos"`
	Channels  map[string]Channel  `json:"channels"`
	Playlists map[string]Playlist `json:"playlists"`
	mu        sync.RWMutex
}

// Custom errors
var (
	ErrUserNotFound     = errors.New("user not found")
	ErrVideoNotFound    = errors.New("video not found")
	ErrChannelNotFound  = errors.New("channel not found")
	ErrPlaylistNotFound = errors.New("playlist not found")
	ErrInvalidInput     = errors.New("invalid input")
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

func (d *Database) GetVideo(id string) (Video, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	video, exists := d.Videos[id]
	if !exists {
		return Video{}, ErrVideoNotFound
	}
	return video, nil
}

func (d *Database) CreatePlaylist(playlist Playlist) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Playlists[playlist.ID] = playlist
	return nil
}

// HTTP Handlers
func getRecommendedVideos(c *fiber.Ctx) error {
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)

	var videos []Video
	db.mu.RLock()
	for _, video := range db.Videos {
		videos = append(videos, video)
	}
	db.mu.RUnlock()

	// Simple pagination
	start := (page - 1) * limit
	end := start + limit
	if start >= len(videos) {
		return c.JSON([]Video{})
	}
	if end > len(videos) {
		end = len(videos)
	}

	return c.JSON(videos[start:end])
}

func getVideoDetails(c *fiber.Ctx) error {
	videoId := c.Params("videoId")

	video, err := db.GetVideo(videoId)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Increment views
	db.mu.Lock()
	video.Views++
	db.Videos[videoId] = video
	db.mu.Unlock()

	return c.JSON(video)
}

func getUserPlaylists(c *fiber.Ctx) error {
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

	return c.JSON(user.Playlists)
}

type CreatePlaylistRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Visibility  string `json:"visibility"`
	UserEmail   string `json:"user_email"`
}

func createPlaylist(c *fiber.Ctx) error {
	var req CreatePlaylistRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.Title == "" || req.UserEmail == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Title and user_email are required",
		})
	}

	user, err := db.GetUser(req.UserEmail)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	playlist := Playlist{
		ID:          uuid.New().String(),
		Title:       req.Title,
		Description: req.Description,
		Visibility:  req.Visibility,
		Videos:      []Video{},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := db.CreatePlaylist(playlist); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create playlist",
		})
	}

	// Update user's playlists
	db.mu.Lock()
	user.Playlists = append(user.Playlists, playlist)
	db.Users[req.UserEmail] = user
	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(playlist)
}

func getUserSubscriptions(c *fiber.Ctx) error {
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

	return c.JSON(user.Subscriptions)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:     make(map[string]User),
		Videos:    make(map[string]Video),
		Channels:  make(map[string]Channel),
		Playlists: make(map[string]Playlist),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Video routes
	api.Get("/videos", getRecommendedVideos)
	api.Get("/videos/:videoId", getVideoDetails)

	// Playlist routes
	api.Get("/playlists", getUserPlaylists)
	api.Post("/playlists", createPlaylist)

	// Subscription routes
	api.Get("/subscriptions", getUserSubscriptions)
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

	// Setup routes
	setupRoutes(app)

	// Start server
	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

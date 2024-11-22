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
	Movie    ContentType = "movie"
	TVSeries ContentType = "tv_series"
)

type Content struct {
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	Type        ContentType `json:"type"`
	Genre       string      `json:"genre"`
	Description string      `json:"description"`
	ReleaseYear int         `json:"release_year"`
	Rating      string      `json:"rating"`
	Duration    string      `json:"duration"`
	Thumbnail   string      `json:"thumbnail"`
	StreamURL   string      `json:"stream_url"`
	Episodes    []Episode   `json:"episodes,omitempty"`
}

type Episode struct {
	ID          string `json:"id"`
	SeasonNum   int    `json:"season_num"`
	EpisodeNum  int    `json:"episode_num"`
	Title       string `json:"title"`
	Duration    string `json:"duration"`
	Thumbnail   string `json:"thumbnail"`
	StreamURL   string `json:"stream_url"`
	Description string `json:"description"`
}

type WatchHistory struct {
	Content   Content   `json:"content"`
	WatchedAt time.Time `json:"watched_at"`
	Progress  int       `json:"progress"`
	Completed bool      `json:"completed"`
	EpisodeID string    `json:"episode_id,omitempty"`
}

type User struct {
	Email        string         `json:"email"`
	Name         string         `json:"name"`
	MyList       []string       `json:"my_list"`
	WatchHistory []WatchHistory `json:"watch_history"`
	Preferences  []string       `json:"preferences"`
	Subscription string         `json:"subscription"`
	ProfileImage string         `json:"profile_image"`
}

type Database struct {
	Users   map[string]User    `json:"users"`
	Content map[string]Content `json:"content"`
	mu      sync.RWMutex
}

var (
	ErrUserNotFound    = errors.New("user not found")
	ErrContentNotFound = errors.New("content not found")
	ErrInvalidInput    = errors.New("invalid input")
)

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

func (d *Database) AddToMyList(email, contentID string) error {
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

	// Check if already in list
	for _, id := range user.MyList {
		if id == contentID {
			return nil
		}
	}

	user.MyList = append(user.MyList, contentID)
	d.Users[email] = user
	return nil
}

func (d *Database) RemoveFromMyList(email, contentID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	user, exists := d.Users[email]
	if !exists {
		return ErrUserNotFound
	}

	// Remove content from list
	var newList []string
	for _, id := range user.MyList {
		if id != contentID {
			newList = append(newList, id)
		}
	}

	user.MyList = newList
	d.Users[email] = user
	return nil
}

func (d *Database) AddWatchHistory(email string, history WatchHistory) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	user, exists := d.Users[email]
	if !exists {
		return ErrUserNotFound
	}

	user.WatchHistory = append(user.WatchHistory, history)
	d.Users[email] = user
	return nil
}

// HTTP Handlers
func getBrowseContent(c *fiber.Ctx) error {
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

	// Get continue watching from watch history
	var continueWatching []Content
	for _, wh := range user.WatchHistory {
		if !wh.Completed {
			continueWatching = append(continueWatching, wh.Content)
		}
	}

	// Get my list content
	var myList []Content
	for _, contentID := range user.MyList {
		content, err := db.GetContent(contentID)
		if err == nil {
			myList = append(myList, content)
		}
	}

	// Get trending content (simplified)
	var trending []Content
	db.mu.RLock()
	for _, content := range db.Content {
		trending = append(trending, content)
		if len(trending) >= 10 {
			break
		}
	}
	db.mu.RUnlock()

	return c.JSON(fiber.Map{
		"continue_watching": continueWatching,
		"my_list":           myList,
		"trending":          trending,
	})
}

func getMyList(c *fiber.Ctx) error {
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

	var myList []Content
	for _, contentID := range user.MyList {
		content, err := db.GetContent(contentID)
		if err == nil {
			myList = append(myList, content)
		}
	}

	return c.JSON(myList)
}

func addToMyList(c *fiber.Ctx) error {
	var req struct {
		Email     string `json:"email"`
		ContentID string `json:"content_id"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if err := db.AddToMyList(req.Email, req.ContentID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.SendStatus(fiber.StatusCreated)
}

func removeFromMyList(c *fiber.Ctx) error {
	email := c.Query("email")
	contentID := c.Query("content_id")

	if email == "" || contentID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email and content_id parameters are required",
		})
	}

	if err := db.RemoveFromMyList(email, contentID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.SendStatus(fiber.StatusOK)
}

func getWatchHistory(c *fiber.Ctx) error {
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

	return c.JSON(user.WatchHistory)
}

func getContentDetails(c *fiber.Ctx) error {
	contentID := c.Params("contentId")
	if contentID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "content ID is required",
		})
	}

	content, err := db.GetContent(contentID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(content)
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

	// Browse routes
	api.Get("/browse", getBrowseContent)

	// My List routes
	api.Get("/my-list", getMyList)
	api.Post("/my-list", addToMyList)
	api.Delete("/my-list", removeFromMyList)

	// Watch History routes
	api.Get("/watch-history", getWatchHistory)

	// Content routes
	api.Get("/content/:contentId", getContentDetails)
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

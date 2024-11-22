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
	Email           string    `json:"email"`
	Name            string    `json:"name"`
	ProfileImageURL string    `json:"profile_image_url"`
	CreatedAt       time.Time `json:"created_at"`
	IsBroadcaster   bool      `json:"is_broadcaster"`
}

type Channel struct {
	ID              string    `json:"id"`
	OwnerEmail      string    `json:"owner_email"`
	DisplayName     string    `json:"display_name"`
	Description     string    `json:"description"`
	ProfileImageURL string    `json:"profile_image_url"`
	FollowersCount  int       `json:"followers_count"`
	IsLive          bool      `json:"is_live"`
	CreatedAt       time.Time `json:"created_at"`
}

type Stream struct {
	ID           string    `json:"id"`
	ChannelID    string    `json:"channel_id"`
	Title        string    `json:"title"`
	Game         string    `json:"game"`
	ViewerCount  int       `json:"viewer_count"`
	ThumbnailURL string    `json:"thumbnail_url"`
	StartedAt    time.Time `json:"started_at"`
	Tags         []string  `json:"tags"`
}

type ChatMessage struct {
	ID        string    `json:"id"`
	ChannelID string    `json:"channel_id"`
	UserEmail string    `json:"user_email"`
	Message   string    `json:"message"`
	Emotes    []string  `json:"emotes"`
	CreatedAt time.Time `json:"created_at"`
}

type Follow struct {
	UserEmail string    `json:"user_email"`
	ChannelID string    `json:"channel_id"`
	CreatedAt time.Time `json:"created_at"`
}

// Database represents our in-memory database
type Database struct {
	Users        map[string]User        `json:"users"`
	Channels     map[string]Channel     `json:"channels"`
	Streams      map[string]Stream      `json:"streams"`
	ChatMessages map[string]ChatMessage `json:"chat_messages"`
	Follows      map[string][]Follow    `json:"follows"`
	mu           sync.RWMutex
}

// Custom errors
var (
	ErrUserNotFound     = errors.New("user not found")
	ErrChannelNotFound  = errors.New("channel not found")
	ErrStreamNotFound   = errors.New("stream not found")
	ErrAlreadyFollowing = errors.New("already following this channel")
	ErrNotFollowing     = errors.New("not following this channel")
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

func (d *Database) GetChannel(id string) (Channel, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	channel, exists := d.Channels[id]
	if !exists {
		return Channel{}, ErrChannelNotFound
	}
	return channel, nil
}

func (d *Database) GetStream(id string) (Stream, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	stream, exists := d.Streams[id]
	if !exists {
		return Stream{}, ErrStreamNotFound
	}
	return stream, nil
}

func (d *Database) AddFollow(userEmail, channelID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Check if already following
	follows := d.Follows[userEmail]
	for _, f := range follows {
		if f.ChannelID == channelID {
			return ErrAlreadyFollowing
		}
	}

	// Add new follow
	follow := Follow{
		UserEmail: userEmail,
		ChannelID: channelID,
		CreatedAt: time.Now(),
	}
	d.Follows[userEmail] = append(d.Follows[userEmail], follow)

	// Update channel followers count
	channel := d.Channels[channelID]
	channel.FollowersCount++
	d.Channels[channelID] = channel

	return nil
}

func (d *Database) RemoveFollow(userEmail, channelID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	follows := d.Follows[userEmail]
	found := false
	newFollows := make([]Follow, 0)

	for _, f := range follows {
		if f.ChannelID == channelID {
			found = true
			continue
		}
		newFollows = append(newFollows, f)
	}

	if !found {
		return ErrNotFollowing
	}

	d.Follows[userEmail] = newFollows

	// Update channel followers count
	channel := d.Channels[channelID]
	channel.FollowersCount--
	d.Channels[channelID] = channel

	return nil
}

// HTTP Handlers
func getStreams(c *fiber.Ctx) error {
	category := c.Query("category")
	limit := c.QueryInt("limit", 20)

	var streams []Stream
	db.mu.RLock()
	for _, stream := range db.Streams {
		if category != "" && stream.Game != category {
			continue
		}
		streams = append(streams, stream)
		if len(streams) >= limit {
			break
		}
	}
	db.mu.RUnlock()

	return c.JSON(streams)
}

func followChannel(c *fiber.Ctx) error {
	channelID := c.Params("channelId")

	var req struct {
		UserEmail string `json:"user_email"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Verify user and channel exist
	if _, err := db.GetUser(req.UserEmail); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if _, err := db.GetChannel(channelID); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if err := db.AddFollow(req.UserEmail, channelID); err != nil {
		if err == ErrAlreadyFollowing {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to follow channel",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Successfully followed channel",
	})
}

func unfollowChannel(c *fiber.Ctx) error {
	channelID := c.Params("channelId")
	userEmail := c.Query("user_email")

	if err := db.RemoveFollow(userEmail, channelID); err != nil {
		if err == ErrNotFollowing {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to unfollow channel",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Successfully unfollowed channel",
	})
}

func getUserFollowing(c *fiber.Ctx) error {
	email := c.Params("email")

	// Verify user exists
	if _, err := db.GetUser(email); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	db.mu.RLock()
	follows := db.Follows[email]
	var channels []Channel
	for _, follow := range follows {
		if channel, exists := db.Channels[follow.ChannelID]; exists {
			channels = append(channels, channel)
		}
	}
	db.mu.RUnlock()

	return c.JSON(channels)
}

func sendChatMessage(c *fiber.Ctx) error {
	channelID := c.Params("channelId")

	var msg ChatMessage
	if err := c.BodyParser(&msg); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Verify user and channel exist
	if _, err := db.GetUser(msg.UserEmail); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if _, err := db.GetChannel(channelID); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	msg.ID = uuid.New().String()
	msg.ChannelID = channelID
	msg.CreatedAt = time.Now()

	db.mu.Lock()
	db.ChatMessages[msg.ID] = msg
	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(msg)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:        make(map[string]User),
		Channels:     make(map[string]Channel),
		Streams:      make(map[string]Stream),
		ChatMessages: make(map[string]ChatMessage),
		Follows:      make(map[string][]Follow),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Stream routes
	api.Get("/streams", getStreams)

	// Channel routes
	api.Post("/channels/:channelId/follow", followChannel)
	api.Delete("/channels/:channelId/follow", unfollowChannel)
	api.Post("/channels/:channelId/chat", sendChatMessage)

	// User routes
	api.Get("/users/:email/following", getUserFollowing)
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

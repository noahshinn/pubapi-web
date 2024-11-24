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
	ID            string    `json:"id"`
	Email         string    `json:"email"`
	Username      string    `json:"username"`
	Discriminator string    `json:"discriminator"`
	Avatar        string    `json:"avatar"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}

type Server struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Icon        string    `json:"icon"`
	OwnerID     string    `json:"owner_id"`
	MemberCount int       `json:"member_count"`
	Channels    []Channel `json:"channels"`
	CreatedAt   time.Time `json:"created_at"`
}

type ChannelType string

const (
	ChannelTypeText     ChannelType = "text"
	ChannelTypeVoice    ChannelType = "voice"
	ChannelTypeCategory ChannelType = "category"
)

type Channel struct {
	ID        string      `json:"id"`
	Type      ChannelType `json:"type"`
	Name      string      `json:"name"`
	Topic     string      `json:"topic"`
	ServerID  string      `json:"server_id"`
	Position  int         `json:"position"`
	CreatedAt time.Time   `json:"created_at"`
}

type Attachment struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	Size        int64  `json:"size"`
	URL         string `json:"url"`
	ProxyURL    string `json:"proxy_url"`
	ContentType string `json:"content_type"`
}

type Message struct {
	ID          string       `json:"id"`
	ChannelID   string       `json:"channel_id"`
	Author      User         `json:"author"`
	Content     string       `json:"content"`
	Attachments []Attachment `json:"attachments"`
	CreatedAt   time.Time    `json:"created_at"`
	EditedAt    *time.Time   `json:"edited_at,omitempty"`
}

// Database represents our in-memory database
type Database struct {
	Users    map[string]User    `json:"users"`
	Servers  map[string]Server  `json:"servers"`
	Channels map[string]Channel `json:"channels"`
	Messages map[string]Message `json:"messages"`
	mu       sync.RWMutex
}

// Custom errors
var (
	ErrUserNotFound    = errors.New("user not found")
	ErrServerNotFound  = errors.New("server not found")
	ErrChannelNotFound = errors.New("channel not found")
	ErrMessageNotFound = errors.New("message not found")
)

// Global database instance
var db *Database

// Database operations
func (d *Database) GetUser(email string) (User, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	for _, user := range d.Users {
		if user.Email == email {
			return user, nil
		}
	}
	return User{}, ErrUserNotFound
}

func (d *Database) GetUserServers(userId string) []Server {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var servers []Server
	for _, server := range d.Servers {
		// In a real implementation, we would check server membership
		servers = append(servers, server)
	}
	return servers
}

func (d *Database) GetServerChannels(serverId string) ([]Channel, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	server, exists := d.Servers[serverId]
	if !exists {
		return nil, ErrServerNotFound
	}

	return server.Channels, nil
}

func (d *Database) GetChannelMessages(channelId string, limit int, before string) ([]Message, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var messages []Message
	for _, msg := range d.Messages {
		if msg.ChannelID == channelId {
			messages = append(messages, msg)
		}
	}

	// In a real implementation, we would:
	// 1. Sort messages by timestamp
	// 2. Apply the 'before' cursor
	// 3. Limit the results

	return messages, nil
}

func (d *Database) CreateMessage(msg Message) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Messages[msg.ID] = msg
	return nil
}

// HTTP Handlers
func getCurrentUser(c *fiber.Ctx) error {
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

	return c.JSON(user)
}

func getUserServers(c *fiber.Ctx) error {
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

	servers := db.GetUserServers(user.ID)
	return c.JSON(servers)
}

func getServerChannels(c *fiber.Ctx) error {
	serverId := c.Params("serverId")
	if serverId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "server ID is required",
		})
	}

	channels, err := db.GetServerChannels(serverId)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(channels)
}

func getChannelMessages(c *fiber.Ctx) error {
	channelId := c.Params("channelId")
	if channelId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "channel ID is required",
		})
	}

	limit := c.QueryInt("limit", 50)
	before := c.Query("before")

	messages, err := db.GetChannelMessages(channelId, limit, before)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(messages)
}

type NewMessageRequest struct {
	Content     string   `json:"content"`
	Attachments []string `json:"attachments"`
}

func createMessage(c *fiber.Ctx) error {
	channelId := c.Params("channelId")
	if channelId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "channel ID is required",
		})
	}

	var req NewMessageRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// In a real implementation, we would get the user from the authentication token
	user, err := db.GetUser("casey.wringer@email.com")
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Unauthorized",
		})
	}

	msg := Message{
		ID:        uuid.New().String(),
		ChannelID: channelId,
		Author:    user,
		Content:   req.Content,
		CreatedAt: time.Now(),
	}

	if err := db.CreateMessage(msg); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create message",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(msg)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:    make(map[string]User),
		Servers:  make(map[string]Server),
		Channels: make(map[string]Channel),
		Messages: make(map[string]Message),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// User routes
	api.Get("/users/me", getCurrentUser)

	// Server routes
	api.Get("/servers", getUserServers)
	api.Get("/servers/:serverId/channels", getServerChannels)

	// Channel routes
	api.Get("/channels/:channelId/messages", getChannelMessages)
	api.Post("/channels/:channelId/messages", createMessage)
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

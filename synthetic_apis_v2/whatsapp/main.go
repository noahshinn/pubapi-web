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
	"github.com/google/uuid"
)

// Models
type MessageType string
type ChatType string
type StatusType string

const (
	MessageTypeText     MessageType = "text"
	MessageTypeImage    MessageType = "image"
	MessageTypeVideo    MessageType = "video"
	MessageTypeAudio    MessageType = "audio"
	MessageTypeDocument MessageType = "document"

	ChatTypeIndividual ChatType = "individual"
	ChatTypeGroup      ChatType = "group"

	StatusTypeText  StatusType = "text"
	StatusTypeImage StatusType = "image"
	StatusTypeVideo StatusType = "video"
)

type Message struct {
	ID        string      `json:"id"`
	ChatID    string      `json:"chat_id"`
	SenderID  string      `json:"sender_id"`
	Content   string      `json:"content"`
	Type      MessageType `json:"type"`
	MediaURL  string      `json:"media_url,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
	Read      bool        `json:"read"`
	Delivered bool        `json:"delivered"`
}

type User struct {
	ID             string    `json:"id"`
	Phone          string    `json:"phone"`
	Name           string    `json:"name"`
	Status         string    `json:"status"`
	ProfilePicture string    `json:"profile_picture"`
	LastSeen       time.Time `json:"last_seen"`
}

type Chat struct {
	ID           string    `json:"id"`
	Type         ChatType  `json:"type"`
	Name         string    `json:"name"`
	Participants []User    `json:"participants"`
	LastMessage  *Message  `json:"last_message"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type StatusView struct {
	UserID   string    `json:"user_id"`
	ViewedAt time.Time `json:"viewed_at"`
}

type Status struct {
	ID        string       `json:"id"`
	UserID    string       `json:"user_id"`
	Content   string       `json:"content"`
	Type      StatusType   `json:"type"`
	MediaURL  string       `json:"media_url,omitempty"`
	CreatedAt time.Time    `json:"created_at"`
	ExpiresAt time.Time    `json:"expires_at"`
	Views     []StatusView `json:"views"`
}

// Database
type Database struct {
	Users    map[string]User    `json:"users"`
	Chats    map[string]Chat    `json:"chats"`
	Messages map[string]Message `json:"messages"`
	Statuses map[string]Status  `json:"statuses"`
	mu       sync.RWMutex
}

var db *Database

// Database operations
func (d *Database) GetUser(id string) (User, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	user, exists := d.Users[id]
	return user, exists
}

func (d *Database) GetChat(id string) (Chat, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	chat, exists := d.Chats[id]
	return chat, exists
}

func (d *Database) GetUserChats(userID string) []Chat {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var userChats []Chat
	for _, chat := range d.Chats {
		for _, participant := range chat.Participants {
			if participant.ID == userID {
				userChats = append(userChats, chat)
				break
			}
		}
	}
	return userChats
}

func (d *Database) GetChatMessages(chatID string, limit int, beforeID string) []Message {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var messages []Message
	for _, msg := range d.Messages {
		if msg.ChatID == chatID {
			if beforeID != "" {
				if msg.ID == beforeID {
					break
				}
			}
			messages = append(messages, msg)
		}
	}

	// Sort messages by timestamp (newest first)
	// Note: In a real implementation, you'd want to use a proper database with sorting

	if len(messages) > limit {
		messages = messages[:limit]
	}
	return messages
}

func (d *Database) SaveMessage(msg Message) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Messages[msg.ID] = msg

	// Update last message in chat
	if chat, exists := d.Chats[msg.ChatID]; exists {
		chat.LastMessage = &msg
		chat.UpdatedAt = time.Now()
		d.Chats[msg.ChatID] = chat
	}

	return nil
}

func (d *Database) GetUserStatuses(userID string) []Status {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var statuses []Status
	now := time.Now()

	for _, status := range d.Statuses {
		// Only return statuses from the user's contacts that haven't expired
		if status.ExpiresAt.After(now) {
			// In a real implementation, you'd check if the status owner is in the user's contacts
			statuses = append(statuses, status)
		}
	}

	return statuses
}

func (d *Database) SaveStatus(status Status) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Statuses[status.ID] = status
	return nil
}

// Handlers
func getMessages(c *fiber.Ctx) error {
	chatID := c.Query("chat_id")
	if chatID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "chat_id is required",
		})
	}

	limit := c.QueryInt("limit", 50)
	beforeID := c.Query("before_id")

	messages := db.GetChatMessages(chatID, limit, beforeID)
	return c.JSON(messages)
}

func sendMessage(c *fiber.Ctx) error {
	var newMsg struct {
		ChatID   string      `json:"chat_id"`
		SenderID string      `json:"sender_id"`
		Content  string      `json:"content"`
		Type     MessageType `json:"type"`
		MediaURL string      `json:"media_url"`
	}

	if err := c.BodyParser(&newMsg); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate chat exists
	if _, exists := db.GetChat(newMsg.ChatID); !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Chat not found",
		})
	}

	// Validate sender exists
	if _, exists := db.GetUser(newMsg.SenderID); !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Sender not found",
		})
	}

	msg := Message{
		ID:        uuid.New().String(),
		ChatID:    newMsg.ChatID,
		SenderID:  newMsg.SenderID,
		Content:   newMsg.Content,
		Type:      newMsg.Type,
		MediaURL:  newMsg.MediaURL,
		Timestamp: time.Now(),
		Delivered: false,
		Read:      false,
	}

	if err := db.SaveMessage(msg); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to save message",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(msg)
}

func getChats(c *fiber.Ctx) error {
	userID := c.Query("user_id")
	if userID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "user_id is required",
		})
	}

	if _, exists := db.GetUser(userID); !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	chats := db.GetUserChats(userID)
	return c.JSON(chats)
}

func getStatuses(c *fiber.Ctx) error {
	userID := c.Query("user_id")
	if userID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "user_id is required",
		})
	}

	if _, exists := db.GetUser(userID); !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	statuses := db.GetUserStatuses(userID)
	return c.JSON(statuses)
}

func postStatus(c *fiber.Ctx) error {
	var newStatus struct {
		UserID   string     `json:"user_id"`
		Content  string     `json:"content"`
		Type     StatusType `json:"type"`
		MediaURL string     `json:"media_url"`
	}

	if err := c.BodyParser(&newStatus); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if _, exists := db.GetUser(newStatus.UserID); !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	status := Status{
		ID:        uuid.New().String(),
		UserID:    newStatus.UserID,
		Content:   newStatus.Content,
		Type:      newStatus.Type,
		MediaURL:  newStatus.MediaURL,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour), // Status expires after 24 hours
		Views:     []StatusView{},
	}

	if err := db.SaveStatus(status); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to save status",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(status)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:    make(map[string]User),
		Chats:    make(map[string]Chat),
		Messages: make(map[string]Message),
		Statuses: make(map[string]Status),
	}

	return json.Unmarshal(data, db)
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

	// Message routes
	api.Get("/messages", getMessages)
	api.Post("/messages", sendMessage)

	// Chat routes
	api.Get("/chats", getChats)

	// Status routes
	api.Get("/status", getStatuses)
	api.Post("/status", postStatus)
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

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

// Domain Models
type Contact struct {
	Email          string `json:"email"`
	Name           string `json:"name"`
	Phone          string `json:"phone"`
	Status         string `json:"status"`
	ProfilePicture string `json:"profile_picture"`
	LastSeen       string `json:"last_seen"`
	Online         bool   `json:"online"`
}

type Message struct {
	ID        string    `json:"id"`
	ChatID    string    `json:"chat_id"`
	Sender    Contact   `json:"sender"`
	Content   string    `json:"content"`
	Type      string    `json:"type"` // text, image, video, audio
	Timestamp time.Time `json:"timestamp"`
	Read      bool      `json:"read"`
}

type Chat struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Type         string    `json:"type"` // individual, group
	Participants []Contact `json:"participants"`
	Messages     []Message `json:"messages"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Database represents our in-memory database
type Database struct {
	Contacts  map[string]Contact   `json:"contacts"`
	Chats     map[string]Chat      `json:"chats"`
	Messages  map[string][]Message `json:"messages"`
	UserChats map[string][]string  `json:"user_chats"` // email -> chat IDs
	mu        sync.RWMutex
}

var db *Database

// Database operations
func (d *Database) GetContact(email string) (Contact, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	contact, exists := d.Contacts[email]
	return contact, exists
}

func (d *Database) GetChat(chatID string) (Chat, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	chat, exists := d.Chats[chatID]
	return chat, exists
}

func (d *Database) GetUserChats(email string) []Chat {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var chats []Chat
	chatIDs := d.UserChats[email]
	for _, id := range chatIDs {
		if chat, exists := d.Chats[id]; exists {
			chats = append(chats, chat)
		}
	}
	return chats
}

func (d *Database) AddMessage(msg Message) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.Messages[msg.ChatID]; !exists {
		d.Messages[msg.ChatID] = []Message{}
	}
	d.Messages[msg.ChatID] = append(d.Messages[msg.ChatID], msg)

	// Update chat's last message
	if chat, exists := d.Chats[msg.ChatID]; exists {
		chat.UpdatedAt = time.Now()
		d.Chats[msg.ChatID] = chat
	}

	return nil
}

// HTTP Handlers
func getUserChats(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	if _, exists := db.GetContact(email); !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "user not found",
		})
	}

	chats := db.GetUserChats(email)
	return c.JSON(chats)
}

func getChatMessages(c *fiber.Ctx) error {
	chatID := c.Query("chat_id")
	if chatID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "chat_id parameter is required",
		})
	}

	db.mu.RLock()
	messages, exists := db.Messages[chatID]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "chat not found",
		})
	}

	return c.JSON(messages)
}

type NewMessageRequest struct {
	ChatID      string `json:"chat_id"`
	SenderEmail string `json:"sender_email"`
	Content     string `json:"content"`
	Type        string `json:"type"`
}

func sendMessage(c *fiber.Ctx) error {
	var req NewMessageRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	sender, exists := db.GetContact(req.SenderEmail)
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "sender not found",
		})
	}

	chat, exists := db.GetChat(req.ChatID)
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "chat not found",
		})
	}

	// Verify sender is participant in chat
	senderIsParticipant := false
	for _, p := range chat.Participants {
		if p.Email == req.SenderEmail {
			senderIsParticipant = true
			break
		}
	}
	if !senderIsParticipant {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "sender is not a participant in this chat",
		})
	}

	msg := Message{
		ID:        uuid.New().String(),
		ChatID:    req.ChatID,
		Sender:    sender,
		Content:   req.Content,
		Type:      req.Type,
		Timestamp: time.Now(),
		Read:      false,
	}

	if err := db.AddMessage(msg); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to send message",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(msg)
}

func getUserContacts(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	if _, exists := db.GetContact(email); !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "user not found",
		})
	}

	db.mu.RLock()
	var contacts []Contact
	for _, contact := range db.Contacts {
		if contact.Email != email {
			contacts = append(contacts, contact)
		}
	}
	db.mu.RUnlock()

	return c.JSON(contacts)
}

func getUserStatus(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	contact, exists := db.GetContact(email)
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "user not found",
		})
	}

	return c.JSON(fiber.Map{
		"online":    contact.Online,
		"last_seen": contact.LastSeen,
		"status":    contact.Status,
	})
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Contacts:  make(map[string]Contact),
		Chats:     make(map[string]Chat),
		Messages:  make(map[string][]Message),
		UserChats: make(map[string][]string),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Chat routes
	api.Get("/chats", getUserChats)
	api.Get("/messages", getChatMessages)
	api.Post("/messages", sendMessage)

	// Contact routes
	api.Get("/contacts", getUserContacts)

	// Status routes
	api.Get("/status", getUserStatus)
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
	app.Use(cors.New())

	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

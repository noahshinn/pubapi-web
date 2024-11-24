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
type User struct {
	Email    string    `json:"email"`
	Name     string    `json:"name"`
	Avatar   string    `json:"avatar"`
	Status   string    `json:"status"`
	Presence string    `json:"presence"`
	LastSeen time.Time `json:"last_seen"`
}

type Message struct {
	ID      string    `json:"id"`
	ChatID  string    `json:"chat_id"`
	Sender  User      `json:"sender"`
	Content string    `json:"content"`
	Type    string    `json:"type"`
	SentAt  time.Time `json:"sent_at"`
}

type Chat struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"` // "direct" or "group"
	Participants []User    `json:"participants"`
	LastMessage  *Message  `json:"last_message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type Channel struct {
	ID          string    `json:"id"`
	TeamID      string    `json:"team_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Type        string    `json:"type"` // "standard" or "private"
	CreatedAt   time.Time `json:"created_at"`
}

type Team struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Members     []User    `json:"members"`
	Channels    []Channel `json:"channels"`
	CreatedAt   time.Time `json:"created_at"`
}

type Meeting struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	Organizer    User      `json:"organizer"`
	Participants []User    `json:"participants"`
	JoinURL      string    `json:"join_url"`
	Status       string    `json:"status"` // "scheduled", "ongoing", "completed", "cancelled"
}

// Database represents our in-memory database
type Database struct {
	Users    map[string]User    `json:"users"`
	Chats    map[string]Chat    `json:"chats"`
	Messages map[string]Message `json:"messages"`
	Teams    map[string]Team    `json:"teams"`
	Meetings map[string]Meeting `json:"meetings"`
	mu       sync.RWMutex
}

var db *Database

// Handlers
func getUserChats(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	var userChats []Chat
	for _, chat := range db.Chats {
		for _, participant := range chat.Participants {
			if participant.Email == email {
				userChats = append(userChats, chat)
				break
			}
		}
	}

	return c.JSON(userChats)
}

func createChat(c *fiber.Ctx) error {
	var req struct {
		Participants []string `json:"participants"`
		Message      string   `json:"message"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if len(req.Participants) < 2 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "At least 2 participants are required",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	// Validate participants
	var participants []User
	for _, email := range req.Participants {
		user, exists := db.Users[email]
		if !exists {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "User not found: " + email,
			})
		}
		participants = append(participants, user)
	}

	chatID := uuid.New().String()
	messageID := uuid.New().String()
	now := time.Now()

	// Create initial message
	message := Message{
		ID:      messageID,
		ChatID:  chatID,
		Sender:  participants[0],
		Content: req.Message,
		Type:    "text",
		SentAt:  now,
	}

	// Create chat
	chat := Chat{
		ID:           chatID,
		Type:         "direct",
		Participants: participants,
		LastMessage:  &message,
		CreatedAt:    now,
	}

	db.Messages[messageID] = message
	db.Chats[chatID] = chat

	return c.Status(fiber.StatusCreated).JSON(chat)
}

func getUserTeams(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	var userTeams []Team
	for _, team := range db.Teams {
		for _, member := range team.Members {
			if member.Email == email {
				userTeams = append(userTeams, team)
				break
			}
		}
	}

	return c.JSON(userTeams)
}

func getUserMeetings(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	var userMeetings []Meeting
	for _, meeting := range db.Meetings {
		if meeting.Organizer.Email == email {
			userMeetings = append(userMeetings, meeting)
			continue
		}
		for _, participant := range meeting.Participants {
			if participant.Email == email {
				userMeetings = append(userMeetings, meeting)
				break
			}
		}
	}

	return c.JSON(userMeetings)
}

func createMeeting(c *fiber.Ctx) error {
	var req struct {
		Title          string    `json:"title"`
		Description    string    `json:"description"`
		StartTime      time.Time `json:"start_time"`
		EndTime        time.Time `json:"end_time"`
		Participants   []string  `json:"participants"`
		OrganizerEmail string    `json:"organizer_email"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	// Validate organizer
	organizer, exists := db.Users[req.OrganizerEmail]
	if !exists {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Organizer not found",
		})
	}

	// Validate participants
	var participants []User
	for _, email := range req.Participants {
		user, exists := db.Users[email]
		if !exists {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "User not found: " + email,
			})
		}
		participants = append(participants, user)
	}

	meeting := Meeting{
		ID:           uuid.New().String(),
		Title:        req.Title,
		Description:  req.Description,
		StartTime:    req.StartTime,
		EndTime:      req.EndTime,
		Organizer:    organizer,
		Participants: participants,
		JoinURL:      "https://teams.microsoft.com/meet/" + uuid.New().String(),
		Status:       "scheduled",
	}

	db.Meetings[meeting.ID] = meeting

	return c.Status(fiber.StatusCreated).JSON(meeting)
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
		Teams:    make(map[string]Team),
		Meetings: make(map[string]Meeting),
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

	// Chat routes
	api.Get("/chats", getUserChats)
	api.Post("/chats", createChat)

	// Team routes
	api.Get("/teams", getUserTeams)

	// Meeting routes
	api.Get("/meetings", getUserMeetings)
	api.Post("/meetings", createMeeting)
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

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
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
	Email     string `json:"email"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	Status    string `json:"status"`
}

type Message struct {
	ID      string    `json:"id"`
	Sender  User      `json:"sender"`
	Content string    `json:"content"`
	SentAt  time.Time `json:"sent_at"`
	ReadBy  []string  `json:"read_by"`
}

type Chat struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"` // "direct" or "group"
	Participants []User    `json:"participants"`
	Messages     []Message `json:"messages"`
	LastMessage  *Message  `json:"last_message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type Channel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsPrivate   bool   `json:"is_private"`
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
	Organizer    User      `json:"organizer"`
	Participants []User    `json:"participants"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	JoinURL      string    `json:"join_url"`
	Status       string    `json:"status"` // scheduled, ongoing, completed, cancelled
}

// Database represents our in-memory database
type Database struct {
	Users    map[string]User    `json:"users"`
	Chats    map[string]Chat    `json:"chats"`
	Teams    map[string]Team    `json:"teams"`
	Meetings map[string]Meeting `json:"meetings"`
	mu       sync.RWMutex
}

// Custom errors
var (
	ErrUserNotFound    = errors.New("user not found")
	ErrChatNotFound    = errors.New("chat not found")
	ErrTeamNotFound    = errors.New("team not found")
	ErrMeetingNotFound = errors.New("meeting not found")
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

func (d *Database) GetUserChats(email string) []Chat {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var userChats []Chat
	for _, chat := range d.Chats {
		for _, participant := range chat.Participants {
			if participant.Email == email {
				userChats = append(userChats, chat)
				break
			}
		}
	}
	return userChats
}

func (d *Database) GetUserTeams(email string) []Team {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var userTeams []Team
	for _, team := range d.Teams {
		for _, member := range team.Members {
			if member.Email == email {
				userTeams = append(userTeams, team)
				break
			}
		}
	}
	return userTeams
}

func (d *Database) GetUserMeetings(email string) []Meeting {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var userMeetings []Meeting
	for _, meeting := range d.Meetings {
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
	return userMeetings
}

// HTTP Handlers
func getUserChats(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	_, err := db.GetUser(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	chats := db.GetUserChats(email)
	return c.JSON(chats)
}

type NewChatRequest struct {
	Participants []string `json:"participants"`
	Message      string   `json:"message"`
}

func createChat(c *fiber.Ctx) error {
	var req NewChatRequest
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

	var participants []User
	for _, email := range req.Participants {
		user, err := db.GetUser(email)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": fmt.Sprintf("User not found: %s", email),
			})
		}
		participants = append(participants, user)
	}

	chatType := "direct"
	if len(participants) > 2 {
		chatType = "group"
	}

	message := Message{
		ID:      uuid.New().String(),
		Sender:  participants[0],
		Content: req.Message,
		SentAt:  time.Now(),
		ReadBy:  []string{participants[0].Email},
	}

	chat := Chat{
		ID:           uuid.New().String(),
		Type:         chatType,
		Participants: participants,
		Messages:     []Message{message},
		LastMessage:  &message,
		CreatedAt:    time.Now(),
	}

	db.mu.Lock()
	db.Chats[chat.ID] = chat
	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(chat)
}

func getUserTeams(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	_, err := db.GetUser(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	teams := db.GetUserTeams(email)
	return c.JSON(teams)
}

func getUserMeetings(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	_, err := db.GetUser(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	meetings := db.GetUserMeetings(email)
	return c.JSON(meetings)
}

type NewMeetingRequest struct {
	Title        string    `json:"title"`
	Participants []string  `json:"participants"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	Description  string    `json:"description"`
}

func createMeeting(c *fiber.Ctx) error {
	var req NewMeetingRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	organizerEmail := c.Query("email")
	if organizerEmail == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "organizer email is required",
		})
	}

	organizer, err := db.GetUser(organizerEmail)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	var participants []User
	for _, email := range req.Participants {
		user, err := db.GetUser(email)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": fmt.Sprintf("User not found: %s", email),
			})
		}
		participants = append(participants, user)
	}

	meeting := Meeting{
		ID:           uuid.New().String(),
		Title:        req.Title,
		Organizer:    organizer,
		Participants: participants,
		StartTime:    req.StartTime,
		EndTime:      req.EndTime,
		JoinURL:      fmt.Sprintf("https://teams.microsoft.com/meet/%s", uuid.New().String()),
		Status:       "scheduled",
	}

	db.mu.Lock()
	db.Meetings[meeting.ID] = meeting
	db.mu.Unlock()

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
		Teams:    make(map[string]Team),
		Meetings: make(map[string]Meeting),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Chat routes
	api.Get("/chats", getUserChats)
	api.Post("/chats", createChat)

	// Team routes
	api.Get("/teams", getUserTeams)

	// Meeting routes
	api.Get("/meetings", getUserMeetings)
	api.Post("/meetings", createMeeting)

	// User routes
	api.Get("/users/:email", func(c *fiber.Ctx) error {
		email := c.Params("email")
		user, err := db.GetUser(email)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.JSON(user)
	})
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
	app.Use(cors.New())

	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

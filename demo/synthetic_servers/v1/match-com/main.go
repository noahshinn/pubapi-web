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
	"github.com/google/uuid"
)

// Domain Models
type Location struct {
	City    string  `json:"city"`
	State   string  `json:"state"`
	Country string  `json:"country"`
	Lat     float64 `json:"latitude"`
	Lon     float64 `json:"longitude"`
}

type Preferences struct {
	AgeRange struct {
		Min int `json:"min"`
		Max int `json:"max"`
	} `json:"age_range"`
	Distance      int    `json:"distance"`
	SeekingGender string `json:"seeking_gender"`
}

type Profile struct {
	ID          string      `json:"id"`
	Email       string      `json:"email"`
	Name        string      `json:"name"`
	Age         int         `json:"age"`
	Gender      string      `json:"gender"`
	Seeking     string      `json:"seeking"`
	Location    Location    `json:"location"`
	Photos      []string    `json:"photos"`
	Bio         string      `json:"bio"`
	Interests   []string    `json:"interests"`
	Preferences Preferences `json:"preferences"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

type Like struct {
	ID        string    `json:"id"`
	FromEmail string    `json:"from_email"`
	ToID      string    `json:"to_profile_id"`
	Action    string    `json:"action"` // "like" or "pass"
	CreatedAt time.Time `json:"created_at"`
}

type Message struct {
	ID        string    `json:"id"`
	SenderID  string    `json:"sender_id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type Conversation struct {
	ID           string    `json:"id"`
	Participants []string  `json:"participants"`
	Messages     []Message `json:"messages"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Database represents our in-memory database
type Database struct {
	Profiles      map[string]Profile      `json:"profiles"`
	Likes         map[string]Like         `json:"likes"`
	Conversations map[string]Conversation `json:"conversations"`
	mu            sync.RWMutex
}

var (
	ErrProfileNotFound = errors.New("profile not found")
	ErrUnauthorized    = errors.New("unauthorized")
	ErrInvalidInput    = errors.New("invalid input")
)

var db *Database

// Database operations
func (d *Database) GetProfile(email string) (Profile, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	for _, profile := range d.Profiles {
		if profile.Email == email {
			return profile, nil
		}
	}
	return Profile{}, ErrProfileNotFound
}

func (d *Database) UpdateProfile(email string, updates map[string]interface{}) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	profile, err := d.GetProfile(email)
	if err != nil {
		return err
	}

	// Apply updates
	if bio, ok := updates["bio"].(string); ok {
		profile.Bio = bio
	}
	if interests, ok := updates["interests"].([]string); ok {
		profile.Interests = interests
	}
	if prefs, ok := updates["preferences"].(Preferences); ok {
		profile.Preferences = prefs
	}

	profile.UpdatedAt = time.Now()
	d.Profiles[profile.ID] = profile
	return nil
}

func (d *Database) GetMatches(email string, page, limit int) ([]Profile, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	profile, err := d.GetProfile(email)
	if err != nil {
		return nil, err
	}

	var matches []Profile
	for _, p := range d.Profiles {
		if isMatch(profile, p) {
			matches = append(matches, p)
		}
	}

	// Apply pagination
	start := (page - 1) * limit
	end := start + limit
	if start >= len(matches) {
		return []Profile{}, nil
	}
	if end > len(matches) {
		end = len(matches)
	}

	return matches[start:end], nil
}

func (d *Database) RecordLike(like Like) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	like.ID = uuid.New().String()
	like.CreatedAt = time.Now()
	d.Likes[like.ID] = like

	// Check for mutual like and create conversation if needed
	for _, existingLike := range d.Likes {
		if existingLike.FromEmail == like.ToID &&
			existingLike.ToID == like.FromEmail &&
			existingLike.Action == "like" {
			// Create new conversation
			conv := Conversation{
				ID:           uuid.New().String(),
				Participants: []string{like.FromEmail, like.ToID},
				Messages:     []Message{},
				CreatedAt:    time.Now(),
				UpdatedAt:    time.Now(),
			}
			d.Conversations[conv.ID] = conv
			break
		}
	}

	return nil
}

func (d *Database) GetLikes(email string) ([]Like, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var likes []Like
	for _, like := range d.Likes {
		if like.ToID == email && like.Action == "like" {
			likes = append(likes, like)
		}
	}
	return likes, nil
}

func (d *Database) GetConversations(email string) ([]Conversation, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var conversations []Conversation
	for _, conv := range d.Conversations {
		for _, participant := range conv.Participants {
			if participant == email {
				conversations = append(conversations, conv)
				break
			}
		}
	}
	return conversations, nil
}

// Helper functions
func isMatch(profile1, profile2 Profile) bool {
	// Don't match with self
	if profile1.Email == profile2.Email {
		return false
	}

	// Check gender preferences
	if profile1.Preferences.SeekingGender != profile2.Gender {
		return false
	}

	// Check age preferences
	if profile2.Age < profile1.Preferences.AgeRange.Min ||
		profile2.Age > profile1.Preferences.AgeRange.Max {
		return false
	}

	// Check distance
	distance := calculateDistance(
		profile1.Location.Lat,
		profile1.Location.Lon,
		profile2.Location.Lat,
		profile2.Location.Lon,
	)

	return distance <= float64(profile1.Preferences.Distance)
}

func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	// Simplified distance calculation
	return ((lat2 - lat1) * (lat2 - lat1)) + ((lon2 - lon1) * (lon2 - lon1))
}

// HTTP Handlers
func getProfile(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	profile, err := db.GetProfile(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(profile)
}

func updateProfile(c *fiber.Ctx) error {
	var updates map[string]interface{}
	if err := c.BodyParser(&updates); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	if err := db.UpdateProfile(email, updates); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	profile, _ := db.GetProfile(email)
	return c.JSON(profile)
}

func getMatches(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)

	matches, err := db.GetMatches(email, page, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(matches)
}

func recordLike(c *fiber.Ctx) error {
	var like Like
	if err := c.BodyParser(&like); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if err := db.RecordLike(like); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(like)
}

func getLikes(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	likes, err := db.GetLikes(email)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(likes)
}

func getConversations(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	conversations, err := db.GetConversations(email)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(conversations)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Profiles:      make(map[string]Profile),
		Likes:         make(map[string]Like),
		Conversations: make(map[string]Conversation),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Profile routes
	api.Get("/profile", getProfile)
	api.Put("/profile", updateProfile)

	// Matching routes
	api.Get("/matches", getMatches)
	api.Post("/likes", recordLike)
	api.Get("/likes", getLikes)

	// Conversation routes
	api.Get("/conversations", getConversations)
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

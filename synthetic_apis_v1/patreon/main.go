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
type Tier struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Price    float64  `json:"price"`
	Benefits []string `json:"benefits"`
}

type Creator struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Description     string  `json:"description"`
	Category        string  `json:"category"`
	SubscriberCount int     `json:"subscribers_count"`
	Tiers           []Tier  `json:"tiers"`
	MonthlyIncome   float64 `json:"monthly_income"`
}

type Post struct {
	ID         string    `json:"id"`
	CreatorID  string    `json:"creator_id"`
	Title      string    `json:"title"`
	Content    string    `json:"content"`
	TierAccess string    `json:"tier_access"`
	CreatedAt  time.Time `json:"created_at"`
}

type SubscriptionStatus string

const (
	SubscriptionStatusActive   SubscriptionStatus = "active"
	SubscriptionStatusCanceled SubscriptionStatus = "canceled"
	SubscriptionStatusExpired  SubscriptionStatus = "expired"
)

type Subscription struct {
	ID        string             `json:"id"`
	UserEmail string             `json:"user_email"`
	Creator   Creator            `json:"creator"`
	Tier      Tier               `json:"tier"`
	Status    SubscriptionStatus `json:"status"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
}

// Database represents our in-memory database
type Database struct {
	Creators      map[string]Creator      `json:"creators"`
	Posts         map[string]Post         `json:"posts"`
	Subscriptions map[string]Subscription `json:"subscriptions"`
	mu            sync.RWMutex
}

// Custom errors
var (
	ErrCreatorNotFound      = errors.New("creator not found")
	ErrTierNotFound         = errors.New("tier not found")
	ErrSubscriptionExists   = errors.New("subscription already exists")
	ErrSubscriptionNotFound = errors.New("subscription not found")
)

var db *Database

// Database operations
func (d *Database) GetCreator(id string) (Creator, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	creator, exists := d.Creators[id]
	if !exists {
		return Creator{}, ErrCreatorNotFound
	}
	return creator, nil
}

func (d *Database) GetCreatorPosts(creatorId string) []Post {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var posts []Post
	for _, post := range d.Posts {
		if post.CreatorID == creatorId {
			posts = append(posts, post)
		}
	}
	return posts
}

func (d *Database) GetUserSubscriptions(email string) []Subscription {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var subs []Subscription
	for _, sub := range d.Subscriptions {
		if sub.UserEmail == email && sub.Status == SubscriptionStatusActive {
			subs = append(subs, sub)
		}
	}
	return subs
}

func (d *Database) CreateSubscription(sub Subscription) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Check if active subscription already exists
	for _, existing := range d.Subscriptions {
		if existing.UserEmail == sub.UserEmail &&
			existing.Creator.ID == sub.Creator.ID &&
			existing.Status == SubscriptionStatusActive {
			return ErrSubscriptionExists
		}
	}

	d.Subscriptions[sub.ID] = sub
	return nil
}

// HTTP Handlers
func getCreators(c *fiber.Ctx) error {
	category := c.Query("category")
	search := c.Query("search")

	var creators []Creator
	db.mu.RLock()
	for _, creator := range db.Creators {
		if (category == "" || creator.Category == category) &&
			(search == "" || contains(creator.Name, search) || contains(creator.Description, search)) {
			creators = append(creators, creator)
		}
	}
	db.mu.RUnlock()

	return c.JSON(creators)
}

func getCreator(c *fiber.Ctx) error {
	creatorId := c.Params("creatorId")
	creator, err := db.GetCreator(creatorId)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	return c.JSON(creator)
}

func getCreatorPosts(c *fiber.Ctx) error {
	creatorId := c.Query("creatorId")
	if creatorId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "creator_id is required",
		})
	}

	posts := db.GetCreatorPosts(creatorId)
	return c.JSON(posts)
}

func getUserSubscriptions(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	subs := db.GetUserSubscriptions(email)
	return c.JSON(subs)
}

type NewSubscriptionRequest struct {
	CreatorID       string `json:"creator_id"`
	TierID          string `json:"tier_id"`
	UserEmail       string `json:"user_email"`
	PaymentMethodID string `json:"payment_method_id"`
}

func createSubscription(c *fiber.Ctx) error {
	var req NewSubscriptionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate creator and tier
	creator, err := db.GetCreator(req.CreatorID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	var selectedTier Tier
	tierFound := false
	for _, tier := range creator.Tiers {
		if tier.ID == req.TierID {
			selectedTier = tier
			tierFound = true
			break
		}
	}
	if !tierFound {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": ErrTierNotFound.Error(),
		})
	}

	subscription := Subscription{
		ID:        uuid.New().String(),
		UserEmail: req.UserEmail,
		Creator:   creator,
		Tier:      selectedTier,
		Status:    SubscriptionStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.CreateSubscription(subscription); err != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(subscription)
}

func contains(s, substr string) bool {
	return s != "" && substr != "" && s != substr
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Creators:      make(map[string]Creator),
		Posts:         make(map[string]Post),
		Subscriptions: make(map[string]Subscription),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Creator routes
	api.Get("/creators", getCreators)
	api.Get("/creators/:creatorId", getCreator)

	// Post routes
	api.Get("/posts", getCreatorPosts)

	// Subscription routes
	api.Get("/subscriptions", getUserSubscriptions)
	api.Post("/subscriptions", createSubscription)
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

	app.Use(logger.New())
	app.Use(cors.New())

	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

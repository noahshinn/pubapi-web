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
type Author struct {
	Email           string `json:"email"`
	Name            string `json:"name"`
	Bio             string `json:"bio"`
	PublicationName string `json:"publication_name"`
}

type Post struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	Author      Author    `json:"author"`
	IsPremium   bool      `json:"is_premium"`
	PublishedAt time.Time `json:"published_at"`
	Comments    []Comment `json:"comments"`
}

type Comment struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	Author    Author    `json:"author"`
	CreatedAt time.Time `json:"created_at"`
}

type SubscriptionTier string

const (
	TierFree    SubscriptionTier = "free"
	TierPremium SubscriptionTier = "premium"
	TierFounder SubscriptionTier = "founder"
)

type SubscriptionStatus string

const (
	StatusActive   SubscriptionStatus = "active"
	StatusCanceled SubscriptionStatus = "canceled"
	StatusExpired  SubscriptionStatus = "expired"
)

type Subscription struct {
	ID              string             `json:"id"`
	Publication     string             `json:"publication"`
	SubscriberEmail string             `json:"subscriber_email"`
	Tier            SubscriptionTier   `json:"tier"`
	Status          SubscriptionStatus `json:"status"`
	CreatedAt       time.Time          `json:"created_at"`
}

// Database represents our in-memory database
type Database struct {
	Authors       map[string]Author       `json:"authors"`
	Posts         map[string]Post         `json:"posts"`
	Subscriptions map[string]Subscription `json:"subscriptions"`
	Comments      map[string]Comment      `json:"comments"`
	mu            sync.RWMutex
}

var db *Database

// Database operations
func (d *Database) GetAuthor(email string) (Author, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	author, exists := d.Authors[email]
	return author, exists
}

func (d *Database) GetPost(id string) (Post, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	post, exists := d.Posts[id]
	return post, exists
}

func (d *Database) CreatePost(post Post) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Posts[post.ID] = post
}

func (d *Database) AddComment(postID string, comment Comment) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	post, exists := d.Posts[postID]
	if !exists {
		return fiber.NewError(fiber.StatusNotFound, "Post not found")
	}

	post.Comments = append(post.Comments, comment)
	d.Posts[postID] = post
	d.Comments[comment.ID] = comment
	return nil
}

func (d *Database) CreateSubscription(sub Subscription) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Subscriptions[sub.ID] = sub
}

func (d *Database) GetUserSubscriptions(email string) []Subscription {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var subs []Subscription
	for _, sub := range d.Subscriptions {
		if sub.SubscriberEmail == email {
			subs = append(subs, sub)
		}
	}
	return subs
}

// HTTP Handlers
func getPosts(c *fiber.Ctx) error {
	authorEmail := c.Query("author_email")
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 10)

	var posts []Post
	db.mu.RLock()
	for _, post := range db.Posts {
		if authorEmail == "" || post.Author.Email == authorEmail {
			posts = append(posts, post)
		}
	}
	db.mu.RUnlock()

	// Simple pagination
	start := (page - 1) * limit
	end := start + limit
	if start >= len(posts) {
		return c.JSON([]Post{})
	}
	if end > len(posts) {
		end = len(posts)
	}

	return c.JSON(posts[start:end])
}

func createPost(c *fiber.Ctx) error {
	var req struct {
		Title       string `json:"title"`
		Content     string `json:"content"`
		AuthorEmail string `json:"author_email"`
		IsPremium   bool   `json:"is_premium"`
	}

	if err := c.BodyParser(&req); err != nil {
		return err
	}

	author, exists := db.GetAuthor(req.AuthorEmail)
	if !exists {
		return fiber.NewError(fiber.StatusNotFound, "Author not found")
	}

	post := Post{
		ID:          uuid.New().String(),
		Title:       req.Title,
		Content:     req.Content,
		Author:      author,
		IsPremium:   req.IsPremium,
		PublishedAt: time.Now(),
		Comments:    make([]Comment, 0),
	}

	db.CreatePost(post)
	return c.Status(fiber.StatusCreated).JSON(post)
}

func getPost(c *fiber.Ctx) error {
	postID := c.Params("postId")
	post, exists := db.GetPost(postID)
	if !exists {
		return fiber.NewError(fiber.StatusNotFound, "Post not found")
	}
	return c.JSON(post)
}

func createComment(c *fiber.Ctx) error {
	var req struct {
		PostID      string `json:"post_id"`
		Content     string `json:"content"`
		AuthorEmail string `json:"author_email"`
	}

	if err := c.BodyParser(&req); err != nil {
		return err
	}

	author, exists := db.GetAuthor(req.AuthorEmail)
	if !exists {
		return fiber.NewError(fiber.StatusNotFound, "Author not found")
	}

	comment := Comment{
		ID:        uuid.New().String(),
		Content:   req.Content,
		Author:    author,
		CreatedAt: time.Now(),
	}

	if err := db.AddComment(req.PostID, comment); err != nil {
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(comment)
}

func getSubscriptions(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Email is required")
	}

	subs := db.GetUserSubscriptions(email)
	return c.JSON(subs)
}

func createSubscription(c *fiber.Ctx) error {
	var req struct {
		Publication     string           `json:"publication"`
		SubscriberEmail string           `json:"subscriber_email"`
		Tier            SubscriptionTier `json:"tier"`
	}

	if err := c.BodyParser(&req); err != nil {
		return err
	}

	sub := Subscription{
		ID:              uuid.New().String(),
		Publication:     req.Publication,
		SubscriberEmail: req.SubscriberEmail,
		Tier:            req.Tier,
		Status:          StatusActive,
		CreatedAt:       time.Now(),
	}

	db.CreateSubscription(sub)
	return c.Status(fiber.StatusCreated).JSON(sub)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Authors:       make(map[string]Author),
		Posts:         make(map[string]Post),
		Subscriptions: make(map[string]Subscription),
		Comments:      make(map[string]Comment),
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

	// Post routes
	api.Get("/posts", getPosts)
	api.Post("/posts", createPost)
	api.Get("/posts/:postId", getPost)

	// Comment routes
	api.Post("/comments", createComment)

	// Subscription routes
	api.Get("/subscriptions", getSubscriptions)
	api.Post("/subscriptions", createSubscription)
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

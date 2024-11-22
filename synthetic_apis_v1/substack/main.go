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
type User struct {
	Email           string    `json:"email"`
	Name            string    `json:"name"`
	Bio             string    `json:"bio"`
	SubscribedSince time.Time `json:"subscribed_since"`
	IsWriter        bool      `json:"is_writer"`
}

type Publication struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	Author           string    `json:"author"`
	Subscribers      int       `json:"subscribers"`
	SubscriptionTier string    `json:"subscription_tier"`
	Price            float64   `json:"price"`
	CreatedAt        time.Time `json:"created_at"`
}

type Post struct {
	ID            string    `json:"id"`
	PublicationID string    `json:"publication_id"`
	Title         string    `json:"title"`
	Subtitle      string    `json:"subtitle"`
	Content       string    `json:"content"`
	Author        string    `json:"author"`
	IsPremium     bool      `json:"is_premium"`
	Likes         int       `json:"likes"`
	Comments      []Comment `json:"comments"`
	CreatedAt     time.Time `json:"created_at"`
}

type Comment struct {
	ID        string    `json:"id"`
	PostID    string    `json:"post_id"`
	Content   string    `json:"content"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"created_at"`
}

type Subscription struct {
	ID            string    `json:"id"`
	UserEmail     string    `json:"user_email"`
	PublicationID string    `json:"publication_id"`
	Plan          string    `json:"plan"`
	Active        bool      `json:"active"`
	CreatedAt     time.Time `json:"created_at"`
	RenewedAt     time.Time `json:"renewed_at"`
}

// Database represents our in-memory database
type Database struct {
	Users         map[string]User         `json:"users"`
	Publications  map[string]Publication  `json:"publications"`
	Posts         map[string]Post         `json:"posts"`
	Comments      map[string]Comment      `json:"comments"`
	Subscriptions map[string]Subscription `json:"subscriptions"`
	mu            sync.RWMutex
}

var (
	ErrUserNotFound        = errors.New("user not found")
	ErrPublicationNotFound = errors.New("publication not found")
	ErrPostNotFound        = errors.New("post not found")
	ErrUnauthorized        = errors.New("unauthorized")
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

func (d *Database) GetPublication(id string) (Publication, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	pub, exists := d.Publications[id]
	if !exists {
		return Publication{}, ErrPublicationNotFound
	}
	return pub, nil
}

func (d *Database) GetPost(id string) (Post, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	post, exists := d.Posts[id]
	if !exists {
		return Post{}, ErrPostNotFound
	}
	return post, nil
}

func (d *Database) CreateSubscription(sub Subscription) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Subscriptions[sub.ID] = sub
	return nil
}

func (d *Database) AddComment(comment Comment) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Comments[comment.ID] = comment

	// Add comment to post
	post := d.Posts[comment.PostID]
	post.Comments = append(post.Comments, comment)
	d.Posts[comment.PostID] = post

	return nil
}

// HTTP Handlers
func getUserPublications(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	// Verify user exists
	_, err := db.GetUser(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	var subscribed []Publication
	db.mu.RLock()
	for _, sub := range db.Subscriptions {
		if sub.UserEmail == email && sub.Active {
			if pub, exists := db.Publications[sub.PublicationID]; exists {
				subscribed = append(subscribed, pub)
			}
		}
	}
	db.mu.RUnlock()

	return c.JSON(subscribed)
}

func getPublicationPosts(c *fiber.Ctx) error {
	pubID := c.Params("publicationId")
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 10)

	// Verify publication exists
	_, err := db.GetPublication(pubID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	var posts []Post
	db.mu.RLock()
	for _, post := range db.Posts {
		if post.PublicationID == pubID {
			posts = append(posts, post)
		}
	}
	db.mu.RUnlock()

	// Simple pagination
	start := (page - 1) * limit
	end := start + limit
	if start >= len(posts) {
		posts = []Post{}
	} else if end > len(posts) {
		posts = posts[start:]
	} else {
		posts = posts[start:end]
	}

	return c.JSON(posts)
}

func getPost(c *fiber.Ctx) error {
	postID := c.Params("postId")

	post, err := db.GetPost(postID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(post)
}

type NewSubscriptionRequest struct {
	PublicationID   string `json:"publication_id"`
	UserEmail       string `json:"user_email"`
	Plan            string `json:"plan"`
	PaymentMethodID string `json:"payment_method_id"`
}

func createSubscription(c *fiber.Ctx) error {
	var req NewSubscriptionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Verify user exists
	user, err := db.GetUser(req.UserEmail)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Verify publication exists
	pub, err := db.GetPublication(req.PublicationID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Check if already subscribed
	db.mu.RLock()
	for _, sub := range db.Subscriptions {
		if sub.UserEmail == user.Email && sub.PublicationID == pub.ID && sub.Active {
			db.mu.RUnlock()
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Already subscribed to this publication",
			})
		}
	}
	db.mu.RUnlock()

	// Create new subscription
	subscription := Subscription{
		ID:            uuid.New().String(),
		UserEmail:     user.Email,
		PublicationID: pub.ID,
		Plan:          req.Plan,
		Active:        true,
		CreatedAt:     time.Now(),
		RenewedAt:     time.Now(),
	}

	if err := db.CreateSubscription(subscription); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create subscription",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(subscription)
}

type NewCommentRequest struct {
	PostID    string `json:"post_id"`
	UserEmail string `json:"user_email"`
	Content   string `json:"content"`
}

func createComment(c *fiber.Ctx) error {
	var req NewCommentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Verify user exists
	user, err := db.GetUser(req.UserEmail)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Verify post exists
	post, err := db.GetPost(req.PostID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Create new comment
	comment := Comment{
		ID:        uuid.New().String(),
		PostID:    post.ID,
		Content:   req.Content,
		Author:    user.Name,
		CreatedAt: time.Now(),
	}

	if err := db.AddComment(comment); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create comment",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(comment)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:         make(map[string]User),
		Publications:  make(map[string]Publication),
		Posts:         make(map[string]Post),
		Comments:      make(map[string]Comment),
		Subscriptions: make(map[string]Subscription),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Publication routes
	api.Get("/publications", getUserPublications)
	api.Get("/publications/:publicationId/posts", getPublicationPosts)

	// Post routes
	api.Get("/posts/:postId", getPost)

	// Subscription routes
	api.Post("/subscriptions", createSubscription)

	// Comment routes
	api.Post("/comments", createComment)
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

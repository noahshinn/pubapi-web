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
	"github.com/gofiber/fiber/v2/middleware/recover"
)

// Models
type Article struct {
	ID              string    `json:"id"`
	Title           string    `json:"title"`
	Subtitle        string    `json:"subtitle"`
	Content         string    `json:"content"`
	Author          string    `json:"author"`
	Category        string    `json:"category"`
	PublishDate     time.Time `json:"publishDate"`
	ImageURL        string    `json:"imageUrl"`
	ReadTimeMinutes int       `json:"readTimeMinutes"`
}

type NotificationPreferences struct {
	BreakingNews     bool `json:"breakingNews"`
	DailyDigest      bool `json:"dailyDigest"`
	WeeklyNewsletter bool `json:"weeklyNewsletter"`
}

type UserPreferences struct {
	Email                   string                  `json:"email"`
	PreferredCategories     []string                `json:"preferredCategories"`
	NewsletterSubscriptions []string                `json:"newsletterSubscriptions"`
	NotificationPreferences NotificationPreferences `json:"notificationPreferences"`
}

type User struct {
	Email              string          `json:"email"`
	Name               string          `json:"name"`
	SubscriptionType   string          `json:"subscriptionType"`
	Preferences        UserPreferences `json:"preferences"`
	BookmarkedArticles []string        `json:"bookmarkedArticles"`
}

// Database represents our in-memory database
type Database struct {
	Users    map[string]User    `json:"users"`
	Articles map[string]Article `json:"articles"`
	mu       sync.RWMutex
}

var db *Database

// Database operations
func (d *Database) GetUser(email string) (User, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if user, exists := d.Users[email]; exists {
		return user, nil
	}
	return User{}, fiber.NewError(fiber.StatusNotFound, "User not found")
}

func (d *Database) UpdateUserPreferences(email string, preferences UserPreferences) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if user, exists := d.Users[email]; exists {
		user.Preferences = preferences
		d.Users[email] = user
		return nil
	}
	return fiber.NewError(fiber.StatusNotFound, "User not found")
}

func (d *Database) AddBookmark(email string, articleId string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if user, exists := d.Users[email]; exists {
		// Check if article exists
		if _, exists := d.Articles[articleId]; !exists {
			return fiber.NewError(fiber.StatusNotFound, "Article not found")
		}

		// Check if already bookmarked
		for _, id := range user.BookmarkedArticles {
			if id == articleId {
				return nil
			}
		}

		user.BookmarkedArticles = append(user.BookmarkedArticles, articleId)
		d.Users[email] = user
		return nil
	}
	return fiber.NewError(fiber.StatusNotFound, "User not found")
}

func (d *Database) GetArticles(category string, page, limit int) []Article {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var articles []Article
	for _, article := range d.Articles {
		if category == "" || article.Category == category {
			articles = append(articles, article)
		}
	}

	// Simple pagination
	start := (page - 1) * limit
	end := start + limit
	if start >= len(articles) {
		return []Article{}
	}
	if end > len(articles) {
		end = len(articles)
	}

	return articles[start:end]
}

func (d *Database) GetArticle(id string) (Article, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if article, exists := d.Articles[id]; exists {
		return article, nil
	}
	return Article{}, fiber.NewError(fiber.StatusNotFound, "Article not found")
}

// Handlers
func getArticles(c *fiber.Ctx) error {
	category := c.Query("category")
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 10)

	articles := db.GetArticles(category, page, limit)
	return c.JSON(articles)
}

func getArticle(c *fiber.Ctx) error {
	articleId := c.Params("articleId")

	article, err := db.GetArticle(articleId)
	if err != nil {
		return err
	}

	return c.JSON(article)
}

func getUserPreferences(c *fiber.Ctx) error {
	email := c.Params("email")

	user, err := db.GetUser(email)
	if err != nil {
		return err
	}

	return c.JSON(user.Preferences)
}

func updateUserPreferences(c *fiber.Ctx) error {
	email := c.Params("email")

	var preferences UserPreferences
	if err := c.BodyParser(&preferences); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if err := db.UpdateUserPreferences(email, preferences); err != nil {
		return err
	}

	return c.SendStatus(fiber.StatusOK)
}

func getUserBookmarks(c *fiber.Ctx) error {
	email := c.Params("email")

	user, err := db.GetUser(email)
	if err != nil {
		return err
	}

	var bookmarkedArticles []Article
	for _, articleId := range user.BookmarkedArticles {
		if article, err := db.GetArticle(articleId); err == nil {
			bookmarkedArticles = append(bookmarkedArticles, article)
		}
	}

	return c.JSON(bookmarkedArticles)
}

func addBookmark(c *fiber.Ctx) error {
	email := c.Params("email")

	var req struct {
		ArticleId string `json:"articleId"`
	}

	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if err := db.AddBookmark(email, req.ArticleId); err != nil {
		return err
	}

	return c.SendStatus(fiber.StatusCreated)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:    make(map[string]User),
		Articles: make(map[string]Article),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Article routes
	api.Get("/articles", getArticles)
	api.Get("/articles/:articleId", getArticle)

	// User preference routes
	api.Get("/users/:email/preferences", getUserPreferences)
	api.Put("/users/:email/preferences", updateUserPreferences)

	// Bookmark routes
	api.Get("/users/:email/bookmarks", getUserBookmarks)
	api.Post("/users/:email/bookmarks", addBookmark)
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
	app.Use(cors.New())

	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

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

type Article struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Abstract    string    `json:"abstract"`
	Content     string    `json:"content"`
	Author      string    `json:"author"`
	Section     string    `json:"section"`
	PublishedAt time.Time `json:"publishedAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	URL         string    `json:"url"`
	ImageURL    string    `json:"imageUrl"`
}

type PaymentMethod struct {
	Type  string `json:"type"`
	Last4 string `json:"last4"`
}

type Subscription struct {
	Email         string        `json:"email"`
	Type          string        `json:"type"`
	Status        string        `json:"status"`
	StartDate     time.Time     `json:"startDate"`
	EndDate       time.Time     `json:"endDate"`
	AutoRenew     bool          `json:"autoRenew"`
	PaymentMethod PaymentMethod `json:"paymentMethod"`
}

type User struct {
	Email        string       `json:"email"`
	Name         string       `json:"name"`
	Subscription Subscription `json:"subscription"`
	Bookmarks    []string     `json:"bookmarks"`
}

type Database struct {
	Articles map[string]Article `json:"articles"`
	Users    map[string]User    `json:"users"`
	mu       sync.RWMutex
}

var db *Database

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Articles: make(map[string]Article),
		Users:    make(map[string]User),
	}

	return json.Unmarshal(data, db)
}

func getArticles(c *fiber.Ctx) error {
	section := c.Query("section")
	limit := c.QueryInt("limit", 10)

	db.mu.RLock()
	defer db.mu.RUnlock()

	var articles []Article
	for _, article := range db.Articles {
		if section != "" && article.Section != section {
			continue
		}
		articles = append(articles, article)
		if len(articles) >= limit {
			break
		}
	}

	return c.JSON(articles)
}

func getArticle(c *fiber.Ctx) error {
	articleID := c.Params("articleId")

	db.mu.RLock()
	article, exists := db.Articles[articleID]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Article not found",
		})
	}

	return c.JSON(article)
}

func getUserBookmarks(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email is required",
		})
	}

	db.mu.RLock()
	user, exists := db.Users[email]
	if !exists {
		db.mu.RUnlock()
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	var bookmarkedArticles []Article
	for _, bookmarkID := range user.Bookmarks {
		if article, exists := db.Articles[bookmarkID]; exists {
			bookmarkedArticles = append(bookmarkedArticles, article)
		}
	}
	db.mu.RUnlock()

	return c.JSON(bookmarkedArticles)
}

func addBookmark(c *fiber.Ctx) error {
	var req struct {
		Email     string `json:"email"`
		ArticleID string `json:"articleId"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	user, exists := db.Users[req.Email]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	if _, exists := db.Articles[req.ArticleID]; !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Article not found",
		})
	}

	// Check if article is already bookmarked
	for _, bookmark := range user.Bookmarks {
		if bookmark == req.ArticleID {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "Article already bookmarked",
			})
		}
	}

	user.Bookmarks = append(user.Bookmarks, req.ArticleID)
	db.Users[req.Email] = user

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Bookmark added successfully",
	})
}

func getSubscription(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email is required",
		})
	}

	db.mu.RLock()
	user, exists := db.Users[email]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	return c.JSON(user.Subscription)
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

	// Articles routes
	api.Get("/articles", getArticles)
	api.Get("/articles/:articleId", getArticle)

	// User routes
	api.Get("/user/bookmarks", getUserBookmarks)
	api.Post("/user/bookmarks", addBookmark)
	api.Get("/user/subscription", getSubscription)
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

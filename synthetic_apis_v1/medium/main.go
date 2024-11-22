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
	Email          string    `json:"email"`
	Name           string    `json:"name"`
	Bio            string    `json:"bio"`
	FollowingCount int       `json:"following_count"`
	FollowersCount int       `json:"followers_count"`
	JoinedAt       time.Time `json:"joined_at"`
}

type Article struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	Author      User      `json:"author"`
	Tags        []string  `json:"tags"`
	Claps       int       `json:"claps"`
	ReadingTime int       `json:"reading_time"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Comment struct {
	ID        string    `json:"id"`
	ArticleID string    `json:"article_id"`
	Content   string    `json:"content"`
	Author    User      `json:"author"`
	CreatedAt time.Time `json:"created_at"`
}

// Database represents our in-memory database
type Database struct {
	Users    map[string]User    `json:"users"`
	Articles map[string]Article `json:"articles"`
	Comments map[string]Comment `json:"comments"`
	mu       sync.RWMutex
}

// Custom errors
var (
	ErrUserNotFound    = errors.New("user not found")
	ErrArticleNotFound = errors.New("article not found")
	ErrInvalidInput    = errors.New("invalid input")
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

func (d *Database) GetArticle(id string) (Article, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	article, exists := d.Articles[id]
	if !exists {
		return Article{}, ErrArticleNotFound
	}
	return article, nil
}

func (d *Database) CreateArticle(article Article) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Articles[article.ID] = article
	return nil
}

func (d *Database) AddClap(articleId string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	article, exists := d.Articles[articleId]
	if !exists {
		return ErrArticleNotFound
	}

	article.Claps++
	d.Articles[articleId] = article
	return nil
}

func (d *Database) AddComment(comment Comment) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Comments[comment.ID] = comment
	return nil
}

// Handlers
func getArticles(c *fiber.Ctx) error {
	tag := c.Query("tag")

	var articles []Article
	db.mu.RLock()
	for _, article := range db.Articles {
		if tag == "" {
			articles = append(articles, article)
			continue
		}

		for _, articleTag := range article.Tags {
			if articleTag == tag {
				articles = append(articles, article)
				break
			}
		}
	}
	db.mu.RUnlock()

	return c.JSON(articles)
}

func getUserArticles(c *fiber.Ctx) error {
	email := c.Params("email")

	var articles []Article
	db.mu.RLock()
	for _, article := range db.Articles {
		if article.Author.Email == email {
			articles = append(articles, article)
		}
	}
	db.mu.RUnlock()

	return c.JSON(articles)
}

type NewArticleRequest struct {
	Title   string   `json:"title"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
	Email   string   `json:"email"`
}

func createArticle(c *fiber.Ctx) error {
	var req NewArticleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	user, err := db.GetUser(req.Email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// Calculate reading time (rough estimate: 200 words per minute)
	wordCount := len(req.Content) / 5
	readingTime := (wordCount + 199) / 200

	article := Article{
		ID:          uuid.New().String(),
		Title:       req.Title,
		Content:     req.Content,
		Author:      user,
		Tags:        req.Tags,
		Claps:       0,
		ReadingTime: readingTime,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := db.CreateArticle(article); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create article",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(article)
}

func clapArticle(c *fiber.Ctx) error {
	articleId := c.Params("articleId")

	if err := db.AddClap(articleId); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Article not found",
		})
	}

	article, _ := db.GetArticle(articleId)
	return c.JSON(fiber.Map{
		"claps": article.Claps,
	})
}

func getArticleComments(c *fiber.Ctx) error {
	articleId := c.Params("articleId")

	var comments []Comment
	db.mu.RLock()
	for _, comment := range db.Comments {
		if comment.ArticleID == articleId {
			comments = append(comments, comment)
		}
	}
	db.mu.RUnlock()

	return c.JSON(comments)
}

type NewCommentRequest struct {
	Content string `json:"content"`
	Email   string `json:"email"`
}

func createComment(c *fiber.Ctx) error {
	articleId := c.Params("articleId")

	var req NewCommentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	user, err := db.GetUser(req.Email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	comment := Comment{
		ID:        uuid.New().String(),
		ArticleID: articleId,
		Content:   req.Content,
		Author:    user,
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
		Users:    make(map[string]User),
		Articles: make(map[string]Article),
		Comments: make(map[string]Comment),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Article routes
	api.Get("/articles", getArticles)
	api.Post("/articles", createArticle)
	api.Get("/users/:email/articles", getUserArticles)
	api.Post("/articles/:articleId/claps", clapArticle)

	// Comment routes
	api.Get("/articles/:articleId/comments", getArticleComments)
	api.Post("/articles/:articleId/comments", createComment)

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
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE",
		AllowHeaders: "Origin, Content-Type, Accept",
	}))

	// Setup routes
	setupRoutes(app)

	// Start server
	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

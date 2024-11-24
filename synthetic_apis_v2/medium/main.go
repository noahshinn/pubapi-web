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

// Models
type User struct {
	ID             string    `json:"id"`
	Email          string    `json:"email"`
	Name           string    `json:"name"`
	Bio            string    `json:"bio"`
	FollowingCount int       `json:"following_count"`
	FollowersCount int       `json:"followers_count"`
	CreatedAt      time.Time `json:"created_at"`
}

type Article struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Subtitle    string    `json:"subtitle"`
	Content     string    `json:"content"`
	Author      User      `json:"author"`
	Tags        []string  `json:"tags"`
	Claps       int       `json:"claps"`
	ReadingTime int       `json:"reading_time"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Database struct {
	Users     map[string]User     `json:"users"`
	Articles  map[string]Article  `json:"articles"`
	Following map[string][]string `json:"following"` // userId -> []userId
	mu        sync.RWMutex
}

var db *Database

// Database operations
func (d *Database) GetUser(id string) (User, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	user, exists := d.Users[id]
	return user, exists
}

func (d *Database) GetArticle(id string) (Article, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	article, exists := d.Articles[id]
	return article, exists
}

func (d *Database) CreateArticle(article Article) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Articles[article.ID] = article
}

func (d *Database) UpdateArticleClaps(id string, claps int) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if article, exists := d.Articles[id]; exists {
		article.Claps += claps
		d.Articles[id] = article
		return true
	}
	return false
}

func (d *Database) GetFollowing(userId string) []User {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var following []User
	if followingIds, exists := d.Following[userId]; exists {
		for _, id := range followingIds {
			if user, exists := d.Users[id]; exists {
				following = append(following, user)
			}
		}
	}
	return following
}

// Handlers
func getArticles(c *fiber.Ctx) error {
	tag := c.Query("tag")
	author := c.Query("author")

	var articles []Article
	db.mu.RLock()
	for _, article := range db.Articles {
		if tag != "" {
			tagFound := false
			for _, t := range article.Tags {
				if t == tag {
					tagFound = true
					break
				}
			}
			if !tagFound {
				continue
			}
		}

		if author != "" && article.Author.ID != author {
			continue
		}

		articles = append(articles, article)
	}
	db.mu.RUnlock()

	return c.JSON(articles)
}

func getArticle(c *fiber.Ctx) error {
	articleId := c.Params("articleId")

	article, exists := db.GetArticle(articleId)
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Article not found",
		})
	}

	return c.JSON(article)
}

type NewArticleRequest struct {
	Title    string   `json:"title"`
	Subtitle string   `json:"subtitle"`
	Content  string   `json:"content"`
	Tags     []string `json:"tags"`
}

func createArticle(c *fiber.Ctx) error {
	var req NewArticleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate required fields
	if req.Title == "" || req.Content == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Title and content are required",
		})
	}

	// Get author (using the consistent user for demo)
	author, exists := db.GetUser("user_1")
	if !exists {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Author not found",
		})
	}

	// Calculate reading time (rough estimate: 200 words per minute)
	wordCount := len(req.Content) / 5      // rough word count
	readingTime := (wordCount + 199) / 200 // round up

	article := Article{
		ID:          uuid.New().String(),
		Title:       req.Title,
		Subtitle:    req.Subtitle,
		Content:     req.Content,
		Author:      author,
		Tags:        req.Tags,
		Claps:       0,
		ReadingTime: readingTime,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	db.CreateArticle(article)

	return c.Status(fiber.StatusCreated).JSON(article)
}

func clapArticle(c *fiber.Ctx) error {
	articleId := c.Params("articleId")

	var req struct {
		Count int `json:"count"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.Count < 1 || req.Count > 50 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Clap count must be between 1 and 50",
		})
	}

	if success := db.UpdateArticleClaps(articleId, req.Count); !success {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Article not found",
		})
	}

	return c.SendStatus(fiber.StatusOK)
}

func getFollowing(c *fiber.Ctx) error {
	userId := c.Params("userId")

	if _, exists := db.GetUser(userId); !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	following := db.GetFollowing(userId)
	return c.JSON(following)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:     make(map[string]User),
		Articles:  make(map[string]Article),
		Following: make(map[string][]string),
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

	// Article routes
	api.Get("/articles", getArticles)
	api.Post("/articles", createArticle)
	api.Get("/articles/:articleId", getArticle)
	api.Post("/articles/:articleId/claps", clapArticle)

	// User routes
	api.Get("/users/:userId/following", getFollowing)
}

func main() {
	port := flag.String("port", "3000", "Port to run the server on")
	flag.Parse()

	if err := loadDatabase(); err != nil {
		log.Fatal(err)
	}

	app := fiber.New()

	app.Use(logger.New())
	app.Use(cors.New())

	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

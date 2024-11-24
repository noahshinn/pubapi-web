package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

type Book struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	Author        string    `json:"author"`
	Description   string    `json:"description"`
	Genre         string    `json:"genre"`
	CoverURL      string    `json:"cover_url"`
	PageCount     int       `json:"page_count"`
	Rating        float64   `json:"rating"`
	PublishedDate time.Time `json:"published_date"`
}

type LibraryBook struct {
	Book            Book      `json:"book"`
	AddedDate       time.Time `json:"added_date"`
	LastReadDate    time.Time `json:"last_read_date"`
	ReadingProgress int       `json:"reading_progress"`
	IsFinished      bool      `json:"is_finished"`
}

type User struct {
	Email           string        `json:"email"`
	Name            string        `json:"name"`
	SubscriptionEnd time.Time     `json:"subscription_end"`
	Library         []LibraryBook `json:"library"`
}

type Database struct {
	Users map[string]User `json:"users"`
	Books []Book          `json:"books"`
	mu    sync.RWMutex
}

var db *Database

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users: make(map[string]User),
	}

	return json.Unmarshal(data, db)
}

func getBooks(c *fiber.Ctx) error {
	genre := c.Query("genre")
	search := c.Query("search")
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	var filteredBooks []Book
	for _, book := range db.Books {
		if genre != "" && !strings.EqualFold(book.Genre, genre) {
			continue
		}
		if search != "" && !strings.Contains(
			strings.ToLower(book.Title+" "+book.Author),
			strings.ToLower(search),
		) {
			continue
		}
		filteredBooks = append(filteredBooks, book)
	}

	// Sort books by rating
	sort.Slice(filteredBooks, func(i, j int) bool {
		return filteredBooks[i].Rating > filteredBooks[j].Rating
	})

	// Calculate pagination
	start := (page - 1) * limit
	end := start + limit
	if start >= len(filteredBooks) {
		return c.JSON(fiber.Map{
			"books": []Book{},
			"total": len(filteredBooks),
		})
	}
	if end > len(filteredBooks) {
		end = len(filteredBooks)
	}

	return c.JSON(fiber.Map{
		"books": filteredBooks[start:end],
		"total": len(filteredBooks),
	})
}

func getLibrary(c *fiber.Ctx) error {
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

	if time.Now().After(user.SubscriptionEnd) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Subscription has expired",
		})
	}

	return c.JSON(user.Library)
}

func addToLibrary(c *fiber.Ctx) error {
	bookId := c.Params("bookId")
	var req struct {
		Email string `json:"email"`
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

	if time.Now().After(user.SubscriptionEnd) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Subscription has expired",
		})
	}

	// Find book
	var book Book
	found := false
	for _, b := range db.Books {
		if b.ID == bookId {
			book = b
			found = true
			break
		}
	}

	if !found {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Book not found",
		})
	}

	// Check if book is already in library
	for _, lb := range user.Library {
		if lb.Book.ID == bookId {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "Book already in library",
			})
		}
	}

	// Add book to library
	libraryBook := LibraryBook{
		Book:            book,
		AddedDate:       time.Now(),
		LastReadDate:    time.Now(),
		ReadingProgress: 0,
		IsFinished:      false,
	}

	user.Library = append(user.Library, libraryBook)
	db.Users[req.Email] = user

	return c.Status(fiber.StatusCreated).JSON(libraryBook)
}

func removeFromLibrary(c *fiber.Ctx) error {
	bookId := c.Params("bookId")
	email := c.Query("email")

	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email is required",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	user, exists := db.Users[email]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// Remove book from library
	found := false
	var updatedLibrary []LibraryBook
	for _, book := range user.Library {
		if book.Book.ID != bookId {
			updatedLibrary = append(updatedLibrary, book)
		} else {
			found = true
		}
	}

	if !found {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Book not found in library",
		})
	}

	user.Library = updatedLibrary
	db.Users[email] = user

	return c.SendStatus(fiber.StatusOK)
}

func updateReadingProgress(c *fiber.Ctx) error {
	var req struct {
		Email      string `json:"email"`
		BookID     string `json:"book_id"`
		Progress   int    `json:"progress"`
		IsFinished bool   `json:"is_finished"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.Progress < 0 || req.Progress > 100 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Progress must be between 0 and 100",
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

	// Update book progress
	found := false
	for i, book := range user.Library {
		if book.Book.ID == req.BookID {
			user.Library[i].ReadingProgress = req.Progress
			user.Library[i].IsFinished = req.IsFinished
			user.Library[i].LastReadDate = time.Now()
			found = true
			break
		}
	}

	if !found {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Book not found in library",
		})
	}

	db.Users[req.Email] = user

	return c.SendStatus(fiber.StatusOK)
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

	// Books routes
	api.Get("/books", getBooks)

	// Library routes
	api.Get("/library", getLibrary)
	api.Post("/library/:bookId", addToLibrary)
	api.Delete("/library/:bookId", removeFromLibrary)

	// Reading progress route
	api.Post("/reading-progress", updateReadingProgress)
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

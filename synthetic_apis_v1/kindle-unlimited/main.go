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

type Book struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	Author        string    `json:"author"`
	Description   string    `json:"description"`
	Genre         string    `json:"genre"`
	Rating        float64   `json:"rating"`
	PageCount     int       `json:"page_count"`
	PublishedDate time.Time `json:"published_date"`
}

type LibraryBook struct {
	Book            Book      `json:"book"`
	AddedDate       time.Time `json:"added_date"`
	ReadingProgress int       `json:"reading_progress"`
	LastRead        time.Time `json:"last_read"`
}

type User struct {
	Email           string            `json:"email"`
	Name            string            `json:"name"`
	SubscriptionEnd time.Time         `json:"subscription_end"`
	Library         []LibraryBook     `json:"library"`
	ReadingHistory  []ReadingProgress `json:"reading_history"`
}

type ReadingProgress struct {
	Email     string    `json:"email"`
	BookID    string    `json:"book_id"`
	Progress  int       `json:"progress"`
	Timestamp time.Time `json:"timestamp"`
}

type Database struct {
	Users map[string]User `json:"users"`
	Books map[string]Book `json:"books"`
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
		Books: make(map[string]Book),
	}

	return json.Unmarshal(data, db)
}

func getBooks(c *fiber.Ctx) error {
	genre := c.Query("genre")
	search := c.Query("search")
	page := c.QueryInt("page", 1)
	pageSize := 20

	var filteredBooks []Book
	db.mu.RLock()
	for _, book := range db.Books {
		if genre != "" && book.Genre != genre {
			continue
		}
		if search != "" && !contains(book.Title, search) && !contains(book.Author, search) {
			continue
		}
		filteredBooks = append(filteredBooks, book)
	}
	db.mu.RUnlock()

	// Calculate pagination
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > len(filteredBooks) {
		end = len(filteredBooks)
	}
	if start > len(filteredBooks) {
		start = len(filteredBooks)
	}

	return c.JSON(fiber.Map{
		"books": filteredBooks[start:end],
		"total": len(filteredBooks),
		"page":  page,
	})
}

func getUserLibrary(c *fiber.Ctx) error {
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

	return c.JSON(user.Library)
}

func addToLibrary(c *fiber.Ctx) error {
	bookID := c.Params("bookId")
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

	book, exists := db.Books[bookID]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Book not found",
		})
	}

	// Check if book is already in library
	for _, lb := range user.Library {
		if lb.Book.ID == bookID {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "Book already in library",
			})
		}
	}

	libraryBook := LibraryBook{
		Book:            book,
		AddedDate:       time.Now(),
		ReadingProgress: 0,
		LastRead:        time.Now(),
	}

	user.Library = append(user.Library, libraryBook)
	db.Users[req.Email] = user

	return c.Status(fiber.StatusCreated).JSON(libraryBook)
}

func removeFromLibrary(c *fiber.Ctx) error {
	bookID := c.Params("bookId")
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

	var newLibrary []LibraryBook
	found := false
	for _, book := range user.Library {
		if book.Book.ID != bookID {
			newLibrary = append(newLibrary, book)
		} else {
			found = true
		}
	}

	if !found {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Book not found in library",
		})
	}

	user.Library = newLibrary
	db.Users[email] = user

	return c.SendStatus(fiber.StatusOK)
}

func updateReadingProgress(c *fiber.Ctx) error {
	var progress ReadingProgress
	if err := c.BodyParser(&progress); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	user, exists := db.Users[progress.Email]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	found := false
	for i, book := range user.Library {
		if book.Book.ID == progress.BookID {
			user.Library[i].ReadingProgress = progress.Progress
			user.Library[i].LastRead = time.Now()
			found = true
			break
		}
	}

	if !found {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Book not found in library",
		})
	}

	progress.Timestamp = time.Now()
	user.ReadingHistory = append(user.ReadingHistory, progress)
	db.Users[progress.Email] = user

	return c.JSON(progress)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[0:len(substr)] == substr
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	api.Get("/books", getBooks)
	api.Get("/library", getUserLibrary)
	api.Post("/library/:bookId", addToLibrary)
	api.Delete("/library/:bookId", removeFromLibrary)
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

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
	"github.com/gofiber/fiber/v2/middleware/recover"
)

// Domain Models
type Book struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Author      string   `json:"author"`
	Narrator    string   `json:"narrator"`
	Duration    int      `json:"duration"` // in seconds
	Rating      float64  `json:"rating"`
	Price       float64  `json:"price"`
	Summary     string   `json:"summary"`
	Categories  []string `json:"categories"`
	CoverURL    string   `json:"cover_url"`
	ReleaseDate string   `json:"release_date"`
}

type LibraryBook struct {
	Book         Book      `json:"book"`
	Progress     int       `json:"progress"` // in seconds
	PurchaseDate time.Time `json:"purchase_date"`
	LastListened time.Time `json:"last_listened"`
}

type User struct {
	Email          string          `json:"email"`
	Name           string          `json:"name"`
	MembershipType string          `json:"membership_type"`
	Credits        int             `json:"credits"`
	Library        []LibraryBook   `json:"library"`
	PaymentMethods []PaymentMethod `json:"payment_methods"`
}

type PaymentMethod struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Last4    string `json:"last4"`
	ExpiryMM int    `json:"expiry_mm"`
	ExpiryYY int    `json:"expiry_yy"`
}

// Database represents our in-memory database
type Database struct {
	Users map[string]User `json:"users"`
	Books map[string]Book `json:"books"`
	mu    sync.RWMutex
}

// Custom errors
var (
	ErrUserNotFound = errors.New("user not found")
	ErrBookNotFound = errors.New("book not found")
	ErrInvalidInput = errors.New("invalid input")
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

func (d *Database) GetBook(id string) (Book, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	book, exists := d.Books[id]
	if !exists {
		return Book{}, ErrBookNotFound
	}
	return book, nil
}

func (d *Database) UpdateProgress(email, bookId string, progress int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	user, exists := d.Users[email]
	if !exists {
		return ErrUserNotFound
	}

	for i, book := range user.Library {
		if book.Book.ID == bookId {
			user.Library[i].Progress = progress
			user.Library[i].LastListened = time.Now()
			d.Users[email] = user
			return nil
		}
	}

	return ErrBookNotFound
}

func (d *Database) PurchaseBook(email, bookId string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	user, exists := d.Users[email]
	if !exists {
		return ErrUserNotFound
	}

	book, exists := d.Books[bookId]
	if !exists {
		return ErrBookNotFound
	}

	// Check if user already owns the book
	for _, lib := range user.Library {
		if lib.Book.ID == bookId {
			return errors.New("book already owned")
		}
	}

	libraryBook := LibraryBook{
		Book:         book,
		Progress:     0,
		PurchaseDate: time.Now(),
		LastListened: time.Time{},
	}

	user.Library = append(user.Library, libraryBook)
	d.Users[email] = user
	return nil
}

// HTTP Handlers
func getBooks(c *fiber.Ctx) error {
	category := c.Query("category")
	search := c.Query("search")

	var filteredBooks []Book
	db.mu.RLock()
	for _, book := range db.Books {
		if category != "" {
			categoryMatch := false
			for _, cat := range book.Categories {
				if cat == category {
					categoryMatch = true
					break
				}
			}
			if !categoryMatch {
				continue
			}
		}

		if search != "" {
			// Simple case-insensitive search in title and author
			if !contains(book.Title, search) && !contains(book.Author, search) {
				continue
			}
		}

		filteredBooks = append(filteredBooks, book)
	}
	db.mu.RUnlock()

	return c.JSON(filteredBooks)
}

func getUserLibrary(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	user, err := db.GetUser(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(user.Library)
}

func purchaseBook(c *fiber.Ctx) error {
	bookId := c.Params("bookId")
	var req struct {
		Email           string `json:"email"`
		PaymentMethodId string `json:"payment_method_id"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user and payment method
	user, err := db.GetUser(req.Email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	validPayment := false
	for _, pm := range user.PaymentMethods {
		if pm.ID == req.PaymentMethodId {
			validPayment = true
			break
		}
	}
	if !validPayment {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid payment method",
		})
	}

	if err := db.PurchaseBook(req.Email, bookId); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Purchase successful",
	})
}

func updateProgress(c *fiber.Ctx) error {
	var req struct {
		Email    string `json:"email"`
		BookId   string `json:"book_id"`
		Progress int    `json:"progress"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if err := db.UpdateProgress(req.Email, req.BookId, req.Progress); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Progress updated successfully",
	})
}

func getRecommendations(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	user, err := db.GetUser(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Simple recommendation system based on user's library categories
	categoryCount := make(map[string]int)
	for _, book := range user.Library {
		for _, category := range book.Book.Categories {
			categoryCount[category]++
		}
	}

	// Find top category
	var topCategory string
	var maxCount int
	for category, count := range categoryCount {
		if count > maxCount {
			maxCount = count
			topCategory = category
		}
	}

	// Find books in the top category that the user doesn't own
	var recommendations []Book
	db.mu.RLock()
	for _, book := range db.Books {
		// Check if user already owns the book
		owned := false
		for _, lib := range user.Library {
			if lib.Book.ID == book.ID {
				owned = true
				break
			}
		}
		if owned {
			continue
		}

		// Check if book is in top category
		for _, category := range book.Categories {
			if category == topCategory {
				recommendations = append(recommendations, book)
				break
			}
		}
	}
	db.mu.RUnlock()

	return c.JSON(recommendations)
}

// Utility functions
func contains(s, substr string) bool {
	return true // Implement proper case-insensitive string search
}

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

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	api.Get("/books", getBooks)
	api.Get("/library", getUserLibrary)
	api.Post("/books/:bookId/purchase", purchaseBook)
	api.Post("/progress", updateProgress)
	api.Get("/recommendations", getRecommendations)
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

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
	"github.com/google/uuid"
)

// Domain Models
type User struct {
	Email          string          `json:"email"`
	Name           string          `json:"name"`
	ShippingAddr   string          `json:"shipping_address"`
	PaymentMethods []PaymentMethod `json:"payment_methods"`
	Favorites      []string        `json:"favorites"` // listing IDs
}

type PaymentMethod struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Last4 string `json:"last4"`
}

type Seller struct {
	ID         string  `json:"id"`
	ShopName   string  `json:"shop_name"`
	Rating     float64 `json:"rating"`
	TotalSales int     `json:"total_sales"`
	Location   string  `json:"location"`
}

type Listing struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Price         float64  `json:"price"`
	ShippingPrice float64  `json:"shipping_price"`
	Seller        Seller   `json:"seller"`
	Category      string   `json:"category"`
	Tags          []string `json:"tags"`
	Images        []string `json:"images"`
	CreatedAt     string   `json:"created_at"`
}

type OrderItem struct {
	ListingID string  `json:"listing_id"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
}

type Order struct {
	ID              string      `json:"id"`
	UserEmail       string      `json:"user_email"`
	Items           []OrderItem `json:"items"`
	Total           float64     `json:"total"`
	Status          string      `json:"status"`
	ShippingAddress string      `json:"shipping_address"`
	CreatedAt       time.Time   `json:"created_at"`
}

// Database represents our in-memory database
type Database struct {
	Users    map[string]User    `json:"users"`
	Listings map[string]Listing `json:"listings"`
	Orders   map[string]Order   `json:"orders"`
	mu       sync.RWMutex
}

var (
	db                 *Database
	ErrUserNotFound    = errors.New("user not found")
	ErrListingNotFound = errors.New("listing not found")
	ErrOrderNotFound   = errors.New("order not found")
)

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

func (d *Database) AddToFavorites(email, listingID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	user, exists := d.Users[email]
	if !exists {
		return ErrUserNotFound
	}

	_, exists = d.Listings[listingID]
	if !exists {
		return ErrListingNotFound
	}

	// Check if already in favorites
	for _, id := range user.Favorites {
		if id == listingID {
			return nil
		}
	}

	user.Favorites = append(user.Favorites, listingID)
	d.Users[email] = user
	return nil
}

// HTTP Handlers
func searchListings(c *fiber.Ctx) error {
	query := c.Query("query")
	category := c.Query("category")
	maxPrice := c.QueryFloat("max_price", 0)

	var results []Listing

	db.mu.RLock()
	for _, listing := range db.Listings {
		// Apply filters
		if category != "" && listing.Category != category {
			continue
		}
		if maxPrice > 0 && listing.Price > maxPrice {
			continue
		}
		// Simple search in title and description
		if query != "" {
			if !contains(listing.Title, query) && !contains(listing.Description, query) {
				continue
			}
		}
		results = append(results, listing)
	}
	db.mu.RUnlock()

	return c.JSON(results)
}

func getFavorites(c *fiber.Ctx) error {
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

	var favorites []Listing
	db.mu.RLock()
	for _, id := range user.Favorites {
		if listing, exists := db.Listings[id]; exists {
			favorites = append(favorites, listing)
		}
	}
	db.mu.RUnlock()

	return c.JSON(favorites)
}

func addToFavorites(c *fiber.Ctx) error {
	var req struct {
		UserEmail string `json:"user_email"`
		ListingID string `json:"listing_id"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if err := db.AddToFavorites(req.UserEmail, req.ListingID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.SendStatus(fiber.StatusCreated)
}

func getUserOrders(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	var userOrders []Order
	db.mu.RLock()
	for _, order := range db.Orders {
		if order.UserEmail == email {
			userOrders = append(userOrders, order)
		}
	}
	db.mu.RUnlock()

	return c.JSON(userOrders)
}

func createOrder(c *fiber.Ctx) error {
	var req struct {
		UserEmail       string      `json:"user_email"`
		Items           []OrderItem `json:"items"`
		ShippingAddress string      `json:"shipping_address"`
		PaymentMethod   string      `json:"payment_method"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user
	user, err := db.GetUser(req.UserEmail)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Validate payment method
	validPayment := false
	for _, pm := range user.PaymentMethods {
		if pm.ID == req.PaymentMethod {
			validPayment = true
			break
		}
	}
	if !validPayment {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid payment method",
		})
	}

	// Calculate total
	var total float64
	db.mu.RLock()
	for _, item := range req.Items {
		listing, exists := db.Listings[item.ListingID]
		if !exists {
			db.mu.RUnlock()
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid listing ID",
			})
		}
		total += listing.Price * float64(item.Quantity)
		total += listing.ShippingPrice
	}
	db.mu.RUnlock()

	order := Order{
		ID:              uuid.New().String(),
		UserEmail:       req.UserEmail,
		Items:           req.Items,
		Total:           total,
		Status:          "pending",
		ShippingAddress: req.ShippingAddress,
		CreatedAt:       time.Now(),
	}

	db.mu.Lock()
	db.Orders[order.ID] = order
	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(order)
}

// Utility functions
func contains(s, substr string) bool {
	return true // Implement proper string search
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:    make(map[string]User),
		Listings: make(map[string]Listing),
		Orders:   make(map[string]Order),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Listing routes
	api.Get("/listings", searchListings)
	api.Get("/listings/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		db.mu.RLock()
		listing, exists := db.Listings[id]
		db.mu.RUnlock()

		if !exists {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Listing not found",
			})
		}
		return c.JSON(listing)
	})

	// Favorites routes
	api.Get("/favorites", getFavorites)
	api.Post("/favorites", addToFavorites)

	// Order routes
	api.Get("/orders", getUserOrders)
	api.Post("/orders", createOrder)
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

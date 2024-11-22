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

// Domain Models
type Product struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Brand        string    `json:"brand"`
	Category     string    `json:"category"`
	Price        float64   `json:"price"`
	Size         string    `json:"size"`
	Description  string    `json:"description"`
	Ingredients  []string  `json:"ingredients"`
	Rating       float64   `json:"rating"`
	ReviewsCount int       `json:"reviews_count"`
	InStock      bool      `json:"in_stock"`
	CreatedAt    time.Time `json:"created_at"`
}

type BeautyProfile struct {
	SkinType          string            `json:"skin_type"`
	SkinConcerns      []string          `json:"skin_concerns"`
	HairType          string            `json:"hair_type"`
	HairConcerns      []string          `json:"hair_concerns"`
	MakeupPreferences map[string]string `json:"makeup_preferences"`
	Allergies         []string          `json:"allergies"`
	PreferredBrands   []string          `json:"preferred_brands"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

type OrderItem struct {
	ProductID string  `json:"product_id"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
}

type Order struct {
	ID        string      `json:"id"`
	UserEmail string      `json:"user_email"`
	Items     []OrderItem `json:"items"`
	Total     float64     `json:"total"`
	Status    string      `json:"status"`
	CreatedAt time.Time   `json:"created_at"`
}

type User struct {
	Email         string        `json:"email"`
	Name          string        `json:"name"`
	BeautyProfile BeautyProfile `json:"beauty_profile"`
	BeautyPoints  int           `json:"beauty_points"`
}

// Database represents our in-memory database
type Database struct {
	Users    map[string]User    `json:"users"`
	Products map[string]Product `json:"products"`
	Orders   map[string]Order   `json:"orders"`
	mu       sync.RWMutex
}

var db *Database

// Database operations
func (d *Database) GetUser(email string) (User, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	user, exists := d.Users[email]
	if !exists {
		return User{}, fiber.NewError(fiber.StatusNotFound, "User not found")
	}
	return user, nil
}

func (d *Database) UpdateBeautyProfile(email string, profile BeautyProfile) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	user, exists := d.Users[email]
	if !exists {
		return fiber.NewError(fiber.StatusNotFound, "User not found")
	}

	user.BeautyProfile = profile
	user.BeautyProfile.UpdatedAt = time.Now()
	d.Users[email] = user
	return nil
}

func (d *Database) GetProducts(category, brand string, priceRange string) []Product {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var filtered []Product
	for _, product := range d.Products {
		if (category == "" || product.Category == category) &&
			(brand == "" || product.Brand == brand) {
			// Add price range filtering logic here if needed
			filtered = append(filtered, product)
		}
	}
	return filtered
}

func (d *Database) GetUserOrders(email string) []Order {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var userOrders []Order
	for _, order := range d.Orders {
		if order.UserEmail == email {
			userOrders = append(userOrders, order)
		}
	}
	return userOrders
}

// Handlers
func getProducts(c *fiber.Ctx) error {
	category := c.Query("category")
	brand := c.Query("brand")
	priceRange := c.Query("price_range")

	products := db.GetProducts(category, brand, priceRange)
	return c.JSON(products)
}

func getBeautyProfile(c *fiber.Ctx) error {
	email := c.Params("email")

	user, err := db.GetUser(email)
	if err != nil {
		return err
	}

	return c.JSON(user.BeautyProfile)
}

func updateBeautyProfile(c *fiber.Ctx) error {
	email := c.Params("email")

	var profile BeautyProfile
	if err := c.BodyParser(&profile); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if err := db.UpdateBeautyProfile(email, profile); err != nil {
		return err
	}

	return c.JSON(profile)
}

func getUserOrders(c *fiber.Ctx) error {
	email := c.Params("email")

	if _, err := db.GetUser(email); err != nil {
		return err
	}

	orders := db.GetUserOrders(email)
	return c.JSON(orders)
}

func getRecommendations(c *fiber.Ctx) error {
	email := c.Params("email")

	user, err := db.GetUser(email)
	if err != nil {
		return err
	}

	// Simple recommendation logic based on preferred brands and skin type
	var recommendations []Product
	for _, product := range db.GetProducts("", "", "") {
		for _, brand := range user.BeautyProfile.PreferredBrands {
			if product.Brand == brand {
				recommendations = append(recommendations, product)
				break
			}
		}
	}

	return c.JSON(recommendations)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:    make(map[string]User),
		Products: make(map[string]Product),
		Orders:   make(map[string]Order),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Product routes
	api.Get("/products", getProducts)

	// User routes
	api.Get("/users/:email/beauty-profile", getBeautyProfile)
	api.Put("/users/:email/beauty-profile", updateBeautyProfile)
	api.Get("/users/:email/orders", getUserOrders)
	api.Get("/users/:email/recommendations", getRecommendations)
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

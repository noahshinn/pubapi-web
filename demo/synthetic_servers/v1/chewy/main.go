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
type Pet struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Breed     string    `json:"breed"`
	Age       float64   `json:"age"`
	Weight    float64   `json:"weight"`
	CreatedAt time.Time `json:"created_at"`
}

type Product struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Brand         string    `json:"brand"`
	Category      string    `json:"category"`
	Description   string    `json:"description"`
	Price         float64   `json:"price"`
	AutoshipPrice float64   `json:"autoship_price"`
	InStock       bool      `json:"in_stock"`
	Rating        float64   `json:"rating"`
	PetTypes      []string  `json:"pet_types"`
	ImageURL      string    `json:"image_url"`
	CreatedAt     time.Time `json:"created_at"`
}

type AutoshipSubscription struct {
	ID             string    `json:"id"`
	UserEmail      string    `json:"user_email"`
	Product        Product   `json:"product"`
	FrequencyWeeks int       `json:"frequency_weeks"`
	NextDelivery   time.Time `json:"next_delivery"`
	Quantity       int       `json:"quantity"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type User struct {
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Pets      []Pet     `json:"pets"`
	Address   Address   `json:"address"`
	CreatedAt time.Time `json:"created_at"`
}

type Address struct {
	Street  string `json:"street"`
	City    string `json:"city"`
	State   string `json:"state"`
	ZipCode string `json:"zip_code"`
}

// Database represents our in-memory database
type Database struct {
	Users    map[string]User                 `json:"users"`
	Products map[string]Product              `json:"products"`
	Autoship map[string]AutoshipSubscription `json:"autoship"`
	mu       sync.RWMutex
}

// Custom errors
var (
	ErrUserNotFound    = errors.New("user not found")
	ErrProductNotFound = errors.New("product not found")
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

func (d *Database) AddPet(email string, pet Pet) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	user, exists := d.Users[email]
	if !exists {
		return ErrUserNotFound
	}

	user.Pets = append(user.Pets, pet)
	d.Users[email] = user
	return nil
}

func (d *Database) GetProducts(category, petType, brand string) []Product {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var filtered []Product
	for _, product := range d.Products {
		if (category == "" || product.Category == category) &&
			(brand == "" || product.Brand == brand) &&
			(petType == "" || contains(product.PetTypes, petType)) {
			filtered = append(filtered, product)
		}
	}
	return filtered
}

func (d *Database) GetAutoship(email string) []AutoshipSubscription {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var subscriptions []AutoshipSubscription
	for _, sub := range d.Autoship {
		if sub.UserEmail == email {
			subscriptions = append(subscriptions, sub)
		}
	}
	return subscriptions
}

func (d *Database) CreateAutoship(sub AutoshipSubscription) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Autoship[sub.ID] = sub
	return nil
}

// Utility functions
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// HTTP Handlers
func getPets(c *fiber.Ctx) error {
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

	return c.JSON(user.Pets)
}

func addPet(c *fiber.Ctx) error {
	var pet Pet
	if err := c.BodyParser(&pet); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	pet.ID = uuid.New().String()
	pet.CreatedAt = time.Now()

	if err := db.AddPet(email, pet); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(pet)
}

func getProducts(c *fiber.Ctx) error {
	category := c.Query("category")
	petType := c.Query("pet_type")
	brand := c.Query("brand")

	products := db.GetProducts(category, petType, brand)
	return c.JSON(products)
}

func getAutoship(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	subscriptions := db.GetAutoship(email)
	return c.JSON(subscriptions)
}

func createAutoship(c *fiber.Ctx) error {
	var sub AutoshipSubscription
	if err := c.BodyParser(&sub); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	sub.ID = uuid.New().String()
	sub.CreatedAt = time.Now()
	sub.UpdatedAt = time.Now()
	sub.Status = "active"

	if err := db.CreateAutoship(sub); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(sub)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:    make(map[string]User),
		Products: make(map[string]Product),
		Autoship: make(map[string]AutoshipSubscription),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Pet routes
	api.Get("/pets", getPets)
	api.Post("/pets", addPet)

	// Product routes
	api.Get("/products", getProducts)

	// Autoship routes
	api.Get("/autoship", getAutoship)
	api.Post("/autoship", createAutoship)
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

	// Setup routes
	setupRoutes(app)

	// Start server
	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

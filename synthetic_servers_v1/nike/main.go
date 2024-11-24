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
type Product struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Category    string    `json:"category"`
	Gender      string    `json:"gender"`
	Price       float64   `json:"price"`
	Sizes       []string  `json:"sizes"`
	Colors      []string  `json:"colors"`
	Description string    `json:"description"`
	Images      []string  `json:"images"`
	CreatedAt   time.Time `json:"created_at"`
}

type OrderItem struct {
	ProductID string  `json:"product_id"`
	Size      string  `json:"size"`
	Color     string  `json:"color"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
}

type Order struct {
	ID              string      `json:"id"`
	UserEmail       string      `json:"user_email"`
	Items           []OrderItem `json:"items"`
	Status          string      `json:"status"`
	Total           float64     `json:"total"`
	ShippingAddress string      `json:"shipping_address"`
	TrackingNumber  string      `json:"tracking_number,omitempty"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

type Activity struct {
	ID        string    `json:"id"`
	UserEmail string    `json:"user_email"`
	Type      string    `json:"type"`
	Duration  int       `json:"duration"` // in minutes
	Distance  float64   `json:"distance"` // in kilometers
	Calories  int       `json:"calories"`
	Date      time.Time `json:"date"`
}

type User struct {
	Email           string    `json:"email"`
	Name            string    `json:"name"`
	ShippingAddress string    `json:"shipping_address"`
	ShoeSize        string    `json:"shoe_size"`
	ClothingSize    string    `json:"clothing_size"`
	JoinedAt        time.Time `json:"joined_at"`
}

// Database represents our in-memory database
type Database struct {
	Users      map[string]User     `json:"users"`
	Products   map[string]Product  `json:"products"`
	Orders     map[string]Order    `json:"orders"`
	Activities map[string]Activity `json:"activities"`
	mu         sync.RWMutex
}

var db *Database

// Database operations
func (d *Database) GetUser(email string) (User, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	user, exists := d.Users[email]
	if !exists {
		return User{}, errors.New("user not found")
	}
	return user, nil
}

func (d *Database) GetProducts(category, gender string) []Product {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var products []Product
	for _, p := range d.Products {
		if (category == "" || p.Category == category) &&
			(gender == "" || p.Gender == gender) {
			products = append(products, p)
		}
	}
	return products
}

func (d *Database) GetUserOrders(email string) []Order {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var orders []Order
	for _, o := range d.Orders {
		if o.UserEmail == email {
			orders = append(orders, o)
		}
	}
	return orders
}

func (d *Database) CreateOrder(order Order) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Orders[order.ID] = order
	return nil
}

func (d *Database) GetUserActivities(email string) []Activity {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var activities []Activity
	for _, a := range d.Activities {
		if a.UserEmail == email {
			activities = append(activities, a)
		}
	}
	return activities
}

func (d *Database) CreateActivity(activity Activity) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Activities[activity.ID] = activity
	return nil
}

// HTTP Handlers
func getProducts(c *fiber.Ctx) error {
	category := c.Query("category")
	gender := c.Query("gender")

	products := db.GetProducts(category, gender)
	return c.JSON(products)
}

func getUserOrders(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	orders := db.GetUserOrders(email)
	return c.JSON(orders)
}

func createOrder(c *fiber.Ctx) error {
	var order Order
	if err := c.BodyParser(&order); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user exists
	if _, err := db.GetUser(order.UserEmail); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	order.ID = uuid.New().String()
	order.Status = "pending"
	order.CreatedAt = time.Now()
	order.UpdatedAt = time.Now()

	if err := db.CreateOrder(order); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create order",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(order)
}

func getUserActivities(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	activities := db.GetUserActivities(email)
	return c.JSON(activities)
}

func createActivity(c *fiber.Ctx) error {
	var activity Activity
	if err := c.BodyParser(&activity); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user exists
	if _, err := db.GetUser(activity.UserEmail); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	activity.ID = uuid.New().String()
	activity.Date = time.Now()

	// Calculate calories based on activity type and duration
	switch activity.Type {
	case "running":
		activity.Calories = int(float64(activity.Duration) * 11.4)
	case "walking":
		activity.Calories = int(float64(activity.Duration) * 5.2)
	default:
		activity.Calories = int(float64(activity.Duration) * 7.0)
	}

	if err := db.CreateActivity(activity); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create activity",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(activity)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:      make(map[string]User),
		Products:   make(map[string]Product),
		Orders:     make(map[string]Order),
		Activities: make(map[string]Activity),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Product routes
	api.Get("/products", getProducts)

	// Order routes
	api.Get("/orders", getUserOrders)
	api.Post("/orders", createOrder)

	// Activity routes
	api.Get("/activities", getUserActivities)
	api.Post("/activities", createActivity)

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

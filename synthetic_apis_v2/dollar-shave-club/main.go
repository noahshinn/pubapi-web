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

type Product struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Category    string  `json:"category"`
	Price       float64 `json:"price"`
	ImageURL    string  `json:"image_url"`
	InStock     bool    `json:"in_stock"`
}

type Subscription struct {
	ID           string    `json:"id"`
	UserEmail    string    `json:"user_email"`
	Product      Product   `json:"product"`
	Frequency    string    `json:"frequency"`
	NextDelivery time.Time `json:"next_delivery"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
}

type Order struct {
	ID             string      `json:"id"`
	UserEmail      string      `json:"user_email"`
	Items          []OrderItem `json:"items"`
	Total          float64     `json:"total"`
	Status         string      `json:"status"`
	ShippingAddr   string      `json:"shipping_address"`
	TrackingNumber string      `json:"tracking_number"`
	CreatedAt      time.Time   `json:"created_at"`
}

type OrderItem struct {
	ProductID string  `json:"product_id"`
	Name      string  `json:"name"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
}

type Database struct {
	Products      map[string]Product      `json:"products"`
	Subscriptions map[string]Subscription `json:"subscriptions"`
	Orders        map[string]Order        `json:"orders"`
	Users         map[string]User         `json:"users"`
	mu            sync.RWMutex
}

type User struct {
	Email          string    `json:"email"`
	Name           string    `json:"name"`
	ShippingAddr   string    `json:"shipping_address"`
	PaymentMethod  string    `json:"payment_method"`
	SubscribedDate time.Time `json:"subscribed_date"`
}

var db *Database

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Products:      make(map[string]Product),
		Subscriptions: make(map[string]Subscription),
		Orders:        make(map[string]Order),
		Users:         make(map[string]User),
	}

	return json.Unmarshal(data, db)
}

func getProducts(c *fiber.Ctx) error {
	category := c.Query("category")

	db.mu.RLock()
	defer db.mu.RUnlock()

	var products []Product
	for _, product := range db.Products {
		if category == "" || product.Category == category {
			products = append(products, product)
		}
	}

	return c.JSON(products)
}

func getSubscriptions(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	var subs []Subscription
	for _, sub := range db.Subscriptions {
		if sub.UserEmail == email {
			subs = append(subs, sub)
		}
	}

	return c.JSON(subs)
}

type NewSubscriptionRequest struct {
	UserEmail string `json:"user_email"`
	ProductID string `json:"product_id"`
	Frequency string `json:"frequency"`
}

func createSubscription(c *fiber.Ctx) error {
	var req NewSubscriptionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	// Validate user exists
	if _, exists := db.Users[req.UserEmail]; !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// Validate product exists
	product, exists := db.Products[req.ProductID]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Product not found",
		})
	}

	subscription := Subscription{
		ID:           uuid.New().String(),
		UserEmail:    req.UserEmail,
		Product:      product,
		Frequency:    req.Frequency,
		NextDelivery: time.Now().AddDate(0, 0, 30), // Default to 30 days
		Status:       "active",
		CreatedAt:    time.Now(),
	}

	db.Subscriptions[subscription.ID] = subscription

	return c.Status(fiber.StatusCreated).JSON(subscription)
}

func getOrders(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	var orders []Order
	for _, order := range db.Orders {
		if order.UserEmail == email {
			orders = append(orders, order)
		}
	}

	return c.JSON(orders)
}

type UpdateSubscriptionRequest struct {
	Frequency string `json:"frequency"`
	Status    string `json:"status"`
}

func updateSubscription(c *fiber.Ctx) error {
	subID := c.Params("subscriptionId")

	var req UpdateSubscriptionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	sub, exists := db.Subscriptions[subID]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Subscription not found",
		})
	}

	if req.Frequency != "" {
		sub.Frequency = req.Frequency
	}
	if req.Status != "" {
		sub.Status = req.Status
	}

	db.Subscriptions[subID] = sub

	return c.JSON(sub)
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

	// Products
	api.Get("/products", getProducts)

	// Subscriptions
	api.Get("/subscriptions", getSubscriptions)
	api.Post("/subscriptions", createSubscription)
	api.Put("/subscriptions/:subscriptionId", updateSubscription)

	// Orders
	api.Get("/orders", getOrders)
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
	log.Fatal(app.Listen(":" + *port))
}

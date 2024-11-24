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
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	Description       string  `json:"description"`
	Category          string  `json:"category"`
	Price             float64 `json:"price"`
	SubscriptionPrice float64 `json:"subscription_price"`
	ImageURL          string  `json:"image_url"`
	InStock           bool    `json:"in_stock"`
}

type SubscriptionFrequency string

const (
	FrequencyMonthly   SubscriptionFrequency = "monthly"
	FrequencyBiMonthly SubscriptionFrequency = "bimonthly"
	FrequencyQuarterly SubscriptionFrequency = "quarterly"
)

type SubscriptionStatus string

const (
	StatusActive    SubscriptionStatus = "active"
	StatusPaused    SubscriptionStatus = "paused"
	StatusCancelled SubscriptionStatus = "cancelled"
)

type Subscription struct {
	ID           string                `json:"id"`
	UserEmail    string                `json:"user_email"`
	Product      Product               `json:"product"`
	Frequency    SubscriptionFrequency `json:"frequency"`
	NextDelivery time.Time             `json:"next_delivery"`
	Status       SubscriptionStatus    `json:"status"`
	CreatedAt    time.Time             `json:"created_at"`
}

type OrderStatus string

const (
	OrderStatusProcessing OrderStatus = "processing"
	OrderStatusShipped    OrderStatus = "shipped"
	OrderStatusDelivered  OrderStatus = "delivered"
)

type Order struct {
	ID              string      `json:"id"`
	SubscriptionID  string      `json:"subscription_id"`
	Items           []OrderItem `json:"items"`
	Status          OrderStatus `json:"status"`
	Total           float64     `json:"total"`
	ShippingAddress string      `json:"shipping_address"`
	TrackingNumber  string      `json:"tracking_number"`
	CreatedAt       time.Time   `json:"created_at"`
}

type OrderItem struct {
	ProductID string  `json:"product_id"`
	Name      string  `json:"name"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
}

type User struct {
	Email           string    `json:"email"`
	Name            string    `json:"name"`
	ShippingAddress string    `json:"shipping_address"`
	PaymentMethod   string    `json:"payment_method"`
	CreatedAt       time.Time `json:"created_at"`
}

// Database represents our in-memory database
type Database struct {
	Users         map[string]User         `json:"users"`
	Products      map[string]Product      `json:"products"`
	Subscriptions map[string]Subscription `json:"subscriptions"`
	Orders        map[string]Order        `json:"orders"`
	mu            sync.RWMutex
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

func (d *Database) GetProduct(id string) (Product, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	product, exists := d.Products[id]
	if !exists {
		return Product{}, errors.New("product not found")
	}
	return product, nil
}

func (d *Database) GetSubscription(id string) (Subscription, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	sub, exists := d.Subscriptions[id]
	if !exists {
		return Subscription{}, errors.New("subscription not found")
	}
	return sub, nil
}

func (d *Database) CreateSubscription(sub Subscription) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Subscriptions[sub.ID] = sub
	return nil
}

func (d *Database) UpdateSubscription(sub Subscription) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Subscriptions[sub.ID] = sub
	return nil
}

// HTTP Handlers
func getProducts(c *fiber.Ctx) error {
	category := c.Query("category")

	var products []Product
	db.mu.RLock()
	for _, product := range db.Products {
		if category == "" || product.Category == category {
			products = append(products, product)
		}
	}
	db.mu.RUnlock()

	return c.JSON(products)
}

func getUserSubscriptions(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	var subs []Subscription
	db.mu.RLock()
	for _, sub := range db.Subscriptions {
		if sub.UserEmail == email {
			subs = append(subs, sub)
		}
	}
	db.mu.RUnlock()

	return c.JSON(subs)
}

type NewSubscriptionRequest struct {
	UserEmail     string                `json:"user_email"`
	ProductID     string                `json:"product_id"`
	Frequency     SubscriptionFrequency `json:"frequency"`
	PaymentMethod string                `json:"payment_method"`
}

func createSubscription(c *fiber.Ctx) error {
	var req NewSubscriptionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user
	user, err := db.GetUser(req.UserEmail)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// Validate product
	product, err := db.GetProduct(req.ProductID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Product not found",
		})
	}

	// Calculate next delivery date based on frequency
	var nextDelivery time.Time
	switch req.Frequency {
	case FrequencyMonthly:
		nextDelivery = time.Now().AddDate(0, 1, 0)
	case FrequencyBiMonthly:
		nextDelivery = time.Now().AddDate(0, 2, 0)
	case FrequencyQuarterly:
		nextDelivery = time.Now().AddDate(0, 3, 0)
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid frequency",
		})
	}

	subscription := Subscription{
		ID:           uuid.New().String(),
		UserEmail:    user.Email,
		Product:      product,
		Frequency:    req.Frequency,
		NextDelivery: nextDelivery,
		Status:       StatusActive,
		CreatedAt:    time.Now(),
	}

	if err := db.CreateSubscription(subscription); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create subscription",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(subscription)
}

type UpdateSubscriptionRequest struct {
	Frequency     SubscriptionFrequency `json:"frequency"`
	Status        SubscriptionStatus    `json:"status"`
	PaymentMethod string                `json:"payment_method"`
}

func updateSubscription(c *fiber.Ctx) error {
	subscriptionId := c.Params("subscriptionId")

	var req UpdateSubscriptionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	subscription, err := db.GetSubscription(subscriptionId)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Subscription not found",
		})
	}

	if req.Frequency != "" {
		subscription.Frequency = req.Frequency
		// Update next delivery date based on new frequency
		switch req.Frequency {
		case FrequencyMonthly:
			subscription.NextDelivery = time.Now().AddDate(0, 1, 0)
		case FrequencyBiMonthly:
			subscription.NextDelivery = time.Now().AddDate(0, 2, 0)
		case FrequencyQuarterly:
			subscription.NextDelivery = time.Now().AddDate(0, 3, 0)
		default:
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid frequency",
			})
		}
	}

	if req.Status != "" {
		subscription.Status = req.Status
	}

	if err := db.UpdateSubscription(subscription); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update subscription",
		})
	}

	return c.JSON(subscription)
}

func getUserOrders(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	var orders []Order
	db.mu.RLock()
	for _, order := range db.Orders {
		sub, exists := db.Subscriptions[order.SubscriptionID]
		if exists && sub.UserEmail == email {
			orders = append(orders, order)
		}
	}
	db.mu.RUnlock()

	return c.JSON(orders)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:         make(map[string]User),
		Products:      make(map[string]Product),
		Subscriptions: make(map[string]Subscription),
		Orders:        make(map[string]Order),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Product routes
	api.Get("/products", getProducts)

	// Subscription routes
	api.Get("/subscriptions", getUserSubscriptions)
	api.Post("/subscriptions", createSubscription)
	api.Put("/subscriptions/:subscriptionId", updateSubscription)

	// Order routes
	api.Get("/orders", getUserOrders)
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

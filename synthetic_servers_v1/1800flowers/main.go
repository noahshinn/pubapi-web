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
	Description string    `json:"description"`
	Category    string    `json:"category"`
	Price       float64   `json:"price"`
	ImageURL    string    `json:"image_url"`
	Available   bool      `json:"available"`
	CreatedAt   time.Time `json:"created_at"`
}

type Recipient struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	City    string `json:"state"`
	State   string `json:"state"`
	ZipCode string `json:"zip_code"`
	Phone   string `json:"phone"`
}

type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusConfirmed OrderStatus = "confirmed"
	OrderStatusDelivered OrderStatus = "delivered"
	OrderStatusCancelled OrderStatus = "cancelled"
)

type Order struct {
	ID           string      `json:"id"`
	UserEmail    string      `json:"user_email"`
	Product      Product     `json:"product"`
	Recipient    Recipient   `json:"recipient"`
	Message      string      `json:"message"`
	DeliveryDate time.Time   `json:"delivery_date"`
	Status       OrderStatus `json:"status"`
	Total        float64     `json:"total"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
}

type DeliveryDate struct {
	Date         time.Time `json:"date"`
	Available    bool      `json:"available"`
	Price        float64   `json:"price"`
	DeliveryType string    `json:"delivery_type"`
}

type User struct {
	Email          string          `json:"email"`
	Name           string          `json:"name"`
	Phone          string          `json:"phone"`
	Address        string          `json:"address"`
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
	Users    map[string]User    `json:"users"`
	Products map[string]Product `json:"products"`
	Orders   map[string]Order   `json:"orders"`
	mu       sync.RWMutex
}

var (
	ErrUserNotFound    = errors.New("user not found")
	ErrProductNotFound = errors.New("product not found")
	ErrOrderNotFound   = errors.New("order not found")
)

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

func (d *Database) GetProduct(id string) (Product, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	product, exists := d.Products[id]
	if !exists {
		return Product{}, ErrProductNotFound
	}
	return product, nil
}

func (d *Database) CreateOrder(order Order) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Orders[order.ID] = order
	return nil
}

func (d *Database) GetOrder(id string) (Order, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	order, exists := d.Orders[id]
	if !exists {
		return Order{}, ErrOrderNotFound
	}
	return order, nil
}

// HTTP Handlers
func getProducts(c *fiber.Ctx) error {
	category := c.Query("category")
	priceRange := c.Query("price_range")

	var filteredProducts []Product

	db.mu.RLock()
	for _, product := range db.Products {
		if !product.Available {
			continue
		}

		if category != "" && product.Category != category {
			continue
		}

		if priceRange != "" {
			switch priceRange {
			case "under_50":
				if product.Price >= 50 {
					continue
				}
			case "50_100":
				if product.Price < 50 || product.Price > 100 {
					continue
				}
			case "over_100":
				if product.Price <= 100 {
					continue
				}
			}
		}

		filteredProducts = append(filteredProducts, product)
	}
	db.mu.RUnlock()

	return c.JSON(filteredProducts)
}

func getDeliveryDates(c *fiber.Ctx) error {
	zipCode := c.Query("zip_code")
	productID := c.Query("product_id")

	if zipCode == "" || productID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "zip_code and product_id are required",
		})
	}

	// Get product to check availability
	product, err := db.GetProduct(productID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if !product.Available {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Product is not available",
		})
	}

	// Generate available delivery dates (next 7 days)
	var dates []DeliveryDate
	baseDate := time.Now().AddDate(0, 0, 1) // Start from tomorrow

	for i := 0; i < 7; i++ {
		date := baseDate.AddDate(0, 0, i)

		// Skip Sundays
		if date.Weekday() == time.Sunday {
			continue
		}

		deliveryType := "standard"
		price := 12.99

		// Premium delivery for Saturdays
		if date.Weekday() == time.Saturday {
			deliveryType = "premium"
			price = 19.99
		}

		dates = append(dates, DeliveryDate{
			Date:         date,
			Available:    true,
			Price:        price,
			DeliveryType: deliveryType,
		})
	}

	return c.JSON(dates)
}

func createOrder(c *fiber.Ctx) error {
	var req struct {
		ProductID       string    `json:"product_id"`
		UserEmail       string    `json:"user_email"`
		Recipient       Recipient `json:"recipient"`
		Message         string    `json:"message"`
		DeliveryDate    string    `json:"delivery_date"`
		PaymentMethodID string    `json:"payment_method_id"`
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

	// Validate product
	product, err := db.GetProduct(req.ProductID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Validate payment method
	validPayment := false
	for _, pm := range user.PaymentMethods {
		if pm.ID == req.PaymentMethodID {
			validPayment = true
			break
		}
	}
	if !validPayment {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid payment method",
		})
	}

	// Parse delivery date
	deliveryDate, err := time.Parse(time.RFC3339, req.DeliveryDate)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid delivery date format",
		})
	}

	// Calculate total (product price + delivery fee)
	deliveryFee := 12.99
	if deliveryDate.Weekday() == time.Saturday {
		deliveryFee = 19.99
	}
	total := product.Price + deliveryFee

	order := Order{
		ID:           uuid.New().String(),
		UserEmail:    req.UserEmail,
		Product:      product,
		Recipient:    req.Recipient,
		Message:      req.Message,
		DeliveryDate: deliveryDate,
		Status:       OrderStatusPending,
		Total:        total,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := db.CreateOrder(order); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create order",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(order)
}

func getUserOrders(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	// Verify user exists
	_, err := db.GetUser(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
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
	api.Get("/products/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		product, err := db.GetProduct(id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.JSON(product)
	})

	// Order routes
	api.Get("/orders", getUserOrders)
	api.Post("/orders", createOrder)
	api.Get("/delivery-dates", getDeliveryDates)

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

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

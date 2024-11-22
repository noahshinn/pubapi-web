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
	"github.com/google/uuid"
)

// Domain Models
type Address struct {
	Street  string `json:"street"`
	City    string `json:"city"`
	State   string `json:"state"`
	ZipCode string `json:"zip_code"`
}

type Person struct {
	Name    string  `json:"name"`
	Email   string  `json:"email"`
	Phone   string  `json:"phone"`
	Address Address `json:"address"`
}

type Product struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Price       float64  `json:"price"`
	Occasions   []string `json:"occasions"`
	ImageURL    string   `json:"image_url"`
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
	Product      Product     `json:"product"`
	Sender       Person      `json:"sender"`
	Recipient    Person      `json:"recipient"`
	DeliveryDate string      `json:"delivery_date"`
	Message      string      `json:"message"`
	Status       OrderStatus `json:"status"`
	Total        float64     `json:"total"`
	CreatedAt    time.Time   `json:"created_at"`
}

// Database represents our in-memory database
type Database struct {
	Users    map[string]Person  `json:"users"`
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
func (d *Database) GetUser(email string) (Person, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	user, exists := d.Users[email]
	if !exists {
		return Person{}, ErrUserNotFound
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

// HTTP Handlers
func getProducts(c *fiber.Ctx) error {
	category := c.Query("category")
	occasion := c.Query("occasion")

	var filteredProducts []Product
	db.mu.RLock()
	for _, product := range db.Products {
		if category != "" && product.Category != category {
			continue
		}
		if occasion != "" {
			hasOccasion := false
			for _, occ := range product.Occasions {
				if occ == occasion {
					hasOccasion = true
					break
				}
			}
			if !hasOccasion {
				continue
			}
		}
		filteredProducts = append(filteredProducts, product)
	}
	db.mu.RUnlock()

	return c.JSON(filteredProducts)
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
		if order.Sender.Email == email {
			userOrders = append(userOrders, order)
		}
	}
	db.mu.RUnlock()

	return c.JSON(userOrders)
}

type CreateOrderRequest struct {
	ProductID    string `json:"product_id"`
	SenderEmail  string `json:"sender_email"`
	Recipient    Person `json:"recipient"`
	DeliveryDate string `json:"delivery_date"`
	Message      string `json:"message"`
}

func createOrder(c *fiber.Ctx) error {
	var req CreateOrderRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate sender
	sender, err := db.GetUser(req.SenderEmail)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Sender not found",
		})
	}

	// Validate product
	product, err := db.GetProduct(req.ProductID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Product not found",
		})
	}

	// Validate delivery date
	deliveryDate, err := time.Parse("2006-01-02", req.DeliveryDate)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid delivery date format",
		})
	}

	if deliveryDate.Before(time.Now()) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Delivery date must be in the future",
		})
	}

	// Create new order
	order := Order{
		ID:           uuid.New().String(),
		Product:      product,
		Sender:       sender,
		Recipient:    req.Recipient,
		DeliveryDate: req.DeliveryDate,
		Message:      req.Message,
		Status:       OrderStatusPending,
		Total:        product.Price,
		CreatedAt:    time.Now(),
	}

	if err := db.CreateOrder(order); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create order",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(order)
}

func getDeliveryDates(c *fiber.Ctx) error {
	zipCode := c.Query("zip_code")
	if zipCode == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "zip_code parameter is required",
		})
	}

	// Calculate available delivery dates
	// In a real implementation, this would check florist availability
	var availableDates []string
	now := time.Now()
	for i := 1; i <= 14; i++ {
		date := now.AddDate(0, 0, i)
		if date.Weekday() != time.Sunday { // No Sunday deliveries
			availableDates = append(availableDates, date.Format("2006-01-02"))
		}
	}

	return c.JSON(availableDates)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:    make(map[string]Person),
		Products: make(map[string]Product),
		Orders:   make(map[string]Order),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	api.Get("/products", getProducts)
	api.Get("/orders", getUserOrders)
	api.Post("/orders", createOrder)
	api.Get("/delivery-dates", getDeliveryDates)
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
	app.Use(cors.New())

	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

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

type Address struct {
	Street  string `json:"street"`
	City    string `json:"city"`
	State   string `json:"state"`
	ZipCode string `json:"zip_code"`
}

type Recipient struct {
	Name    string  `json:"name"`
	Phone   string  `json:"phone"`
	Address Address `json:"address"`
}

type Product struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Occasions   []string `json:"occasions"`
	Price       float64  `json:"price"`
	ImageURL    string   `json:"image_url"`
	Available   bool     `json:"available"`
}

type Order struct {
	ID           string    `json:"id"`
	UserEmail    string    `json:"user_email"`
	Product      Product   `json:"product"`
	Recipient    Recipient `json:"recipient"`
	Message      string    `json:"message"`
	DeliveryDate string    `json:"delivery_date"`
	Status       string    `json:"status"`
	Total        float64   `json:"total"`
	CreatedAt    time.Time `json:"created_at"`
}

type DeliveryDate struct {
	Date        string  `json:"date"`
	Available   bool    `json:"available"`
	DeliveryFee float64 `json:"delivery_fee"`
	CutoffTime  string  `json:"cutoff_time"`
}

type Database struct {
	Products map[string]Product `json:"products"`
	Orders   map[string]Order   `json:"orders"`
	Users    map[string]User    `json:"users"`
	mu       sync.RWMutex
}

type User struct {
	Email          string          `json:"email"`
	Name           string          `json:"name"`
	Phone          string          `json:"phone"`
	Address        Address         `json:"address"`
	PaymentMethods []PaymentMethod `json:"payment_methods"`
}

type PaymentMethod struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Last4    string `json:"last4"`
	ExpiryMM int    `json:"expiry_mm"`
	ExpiryYY int    `json:"expiry_yy"`
}

var db *Database

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Products: make(map[string]Product),
		Orders:   make(map[string]Order),
		Users:    make(map[string]User),
	}

	return json.Unmarshal(data, db)
}

func getProducts(c *fiber.Ctx) error {
	category := c.Query("category")
	occasion := c.Query("occasion")

	db.mu.RLock()
	defer db.mu.RUnlock()

	var products []Product
	for _, product := range db.Products {
		if !product.Available {
			continue
		}

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

		products = append(products, product)
	}

	return c.JSON(products)
}

func getDeliveryDates(c *fiber.Ctx) error {
	zipCode := c.Query("zip_code")
	productID := c.Query("product_id")

	if zipCode == "" || productID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "zip_code and product_id are required",
		})
	}

	// In a real implementation, we would check the zip code against a delivery zone database
	// and the product availability. Here we'll return some sample dates.

	dates := []DeliveryDate{
		{
			Date:        time.Now().AddDate(0, 0, 1).Format("2006-01-02"),
			Available:   true,
			DeliveryFee: 14.99,
			CutoffTime:  "2:00 PM",
		},
		{
			Date:        time.Now().AddDate(0, 0, 2).Format("2006-01-02"),
			Available:   true,
			DeliveryFee: 12.99,
			CutoffTime:  "2:00 PM",
		},
		{
			Date:        time.Now().AddDate(0, 0, 3).Format("2006-01-02"),
			Available:   true,
			DeliveryFee: 9.99,
			CutoffTime:  "2:00 PM",
		},
	}

	return c.JSON(dates)
}

func getUserOrders(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	var userOrders []Order
	for _, order := range db.Orders {
		if order.UserEmail == email {
			userOrders = append(userOrders, order)
		}
	}

	return c.JSON(userOrders)
}

type NewOrderRequest struct {
	UserEmail       string    `json:"user_email"`
	ProductID       string    `json:"product_id"`
	Recipient       Recipient `json:"recipient"`
	Message         string    `json:"message"`
	DeliveryDate    string    `json:"delivery_date"`
	PaymentMethodID string    `json:"payment_method_id"`
}

func createOrder(c *fiber.Ctx) error {
	var req NewOrderRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	// Validate user
	user, exists := db.Users[req.UserEmail]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
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

	// Validate product
	product, exists := db.Products[req.ProductID]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Product not found",
		})
	}

	if !product.Available {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Product is not available",
		})
	}

	// Create order
	order := Order{
		ID:           uuid.New().String(),
		UserEmail:    req.UserEmail,
		Product:      product,
		Recipient:    req.Recipient,
		Message:      req.Message,
		DeliveryDate: req.DeliveryDate,
		Status:       "pending",
		Total:        product.Price + 12.99, // Base price + delivery fee
		CreatedAt:    time.Now(),
	}

	db.Orders[order.ID] = order

	return c.Status(fiber.StatusCreated).JSON(order)
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

	api.Get("/products", getProducts)
	api.Get("/delivery-dates", getDeliveryDates)
	api.Get("/orders", getUserOrders)
	api.Post("/orders", createOrder)
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

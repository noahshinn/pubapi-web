package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"strings"
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
	Country string `json:"country"`
}

type PaymentMethod struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Last4    string `json:"last4"`
	ExpiryMM int    `json:"expiry_mm"`
	ExpiryYY int    `json:"expiry_yy"`
}

type User struct {
	Email          string          `json:"email"`
	Name           string          `json:"name"`
	Address        Address         `json:"address"`
	PaymentMethods []PaymentMethod `json:"payment_methods"`
	PrimeMember    bool            `json:"prime_member"`
}

type Product struct {
	ID                string    `json:"id"`
	Title             string    `json:"title"`
	Description       string    `json:"description"`
	Price             float64   `json:"price"`
	Category          string    `json:"category"`
	Rating            float64   `json:"rating"`
	ReviewsCount      int       `json:"reviews_count"`
	PrimeEligible     bool      `json:"prime_eligible"`
	InStock           bool      `json:"in_stock"`
	EstimatedDelivery string    `json:"estimated_delivery"`
	CreatedAt         time.Time `json:"created_at"`
}

type CartItem struct {
	ProductID string  `json:"product_id"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
}

type Cart struct {
	UserEmail string     `json:"user_email"`
	Items     []CartItem `json:"items"`
	Subtotal  float64    `json:"subtotal"`
	Shipping  float64    `json:"shipping"`
	Tax       float64    `json:"tax"`
	Total     float64    `json:"total"`
}

type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusConfirmed OrderStatus = "confirmed"
	OrderStatusShipped   OrderStatus = "shipped"
	OrderStatusDelivered OrderStatus = "delivered"
	OrderStatusCancelled OrderStatus = "cancelled"
)

type Order struct {
	ID              string      `json:"id"`
	UserEmail       string      `json:"user_email"`
	Items           []CartItem  `json:"items"`
	Status          OrderStatus `json:"status"`
	ShippingAddress Address     `json:"shipping_address"`
	PaymentMethod   string      `json:"payment_method"`
	Subtotal        float64     `json:"subtotal"`
	Shipping        float64     `json:"shipping"`
	Tax             float64     `json:"tax"`
	Total           float64     `json:"total"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

type Database struct {
	Users    map[string]User    `json:"users"`
	Products map[string]Product `json:"products"`
	Carts    map[string]Cart    `json:"carts"`
	Orders   map[string]Order   `json:"orders"`
	mu       sync.RWMutex
}

var db *Database

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:    make(map[string]User),
		Products: make(map[string]Product),
		Carts:    make(map[string]Cart),
		Orders:   make(map[string]Order),
	}

	return json.Unmarshal(data, db)
}

func searchProducts(c *fiber.Ctx) error {
	query := strings.ToLower(c.Query("query"))
	category := c.Query("category")
	page := c.QueryInt("page", 1)
	perPage := 20

	var matchingProducts []Product
	db.mu.RLock()
	for _, product := range db.Products {
		if (query == "" || strings.Contains(strings.ToLower(product.Title), query) ||
			strings.Contains(strings.ToLower(product.Description), query)) &&
			(category == "" || product.Category == category) {
			matchingProducts = append(matchingProducts, product)
		}
	}
	db.mu.RUnlock()

	// Calculate pagination
	start := (page - 1) * perPage
	end := start + perPage
	if end > len(matchingProducts) {
		end = len(matchingProducts)
	}
	if start >= len(matchingProducts) {
		start = 0
		end = 0
	}

	return c.JSON(fiber.Map{
		"products": matchingProducts[start:end],
		"total":    len(matchingProducts),
	})
}

func getCart(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	db.mu.RLock()
	cart, exists := db.Carts[email]
	db.mu.RUnlock()

	if !exists {
		cart = Cart{
			UserEmail: email,
			Items:     make([]CartItem, 0),
		}
	}

	return c.JSON(cart)
}

func addToCart(c *fiber.Ctx) error {
	var req struct {
		UserEmail string `json:"user_email"`
		ProductID string `json:"product_id"`
		Quantity  int    `json:"quantity"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	// Verify product exists and is in stock
	product, exists := db.Products[req.ProductID]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Product not found",
		})
	}

	if !product.InStock {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Product out of stock",
		})
	}

	// Get or create cart
	cart, exists := db.Carts[req.UserEmail]
	if !exists {
		cart = Cart{
			UserEmail: req.UserEmail,
			Items:     make([]CartItem, 0),
		}
	}

	// Add or update item in cart
	itemExists := false
	for i, item := range cart.Items {
		if item.ProductID == req.ProductID {
			cart.Items[i].Quantity += req.Quantity
			itemExists = true
			break
		}
	}

	if !itemExists {
		cart.Items = append(cart.Items, CartItem{
			ProductID: req.ProductID,
			Quantity:  req.Quantity,
			Price:     product.Price,
		})
	}

	// Recalculate totals
	cart.Subtotal = 0
	for _, item := range cart.Items {
		cart.Subtotal += item.Price * float64(item.Quantity)
	}

	// Check if user is Prime member for shipping calculation
	user, exists := db.Users[req.UserEmail]
	if exists && user.PrimeMember {
		cart.Shipping = 0
	} else {
		cart.Shipping = 5.99
	}

	cart.Tax = cart.Subtotal * 0.0825 // 8.25% tax rate
	cart.Total = cart.Subtotal + cart.Shipping + cart.Tax

	db.Carts[req.UserEmail] = cart

	return c.JSON(cart)
}

func getOrders(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
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

func placeOrder(c *fiber.Ctx) error {
	var req struct {
		UserEmail       string  `json:"user_email"`
		ShippingAddress Address `json:"shipping_address"`
		PaymentMethod   string  `json:"payment_method"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	// Verify user exists
	user, exists := db.Users[req.UserEmail]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// Verify payment method
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

	// Get user's cart
	cart, exists := db.Carts[req.UserEmail]
	if !exists || len(cart.Items) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cart is empty",
		})
	}

	// Create order
	order := Order{
		ID:              uuid.New().String(),
		UserEmail:       req.UserEmail,
		Items:           cart.Items,
		Status:          OrderStatusPending,
		ShippingAddress: req.ShippingAddress,
		PaymentMethod:   req.PaymentMethod,
		Subtotal:        cart.Subtotal,
		Shipping:        cart.Shipping,
		Tax:             cart.Tax,
		Total:           cart.Total,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	// Save order and clear cart
	db.Orders[order.ID] = order
	delete(db.Carts, req.UserEmail)

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

	// Product routes
	api.Get("/products", searchProducts)
	api.Get("/products/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		db.mu.RLock()
		product, exists := db.Products[id]
		db.mu.RUnlock()
		if !exists {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Product not found",
			})
		}
		return c.JSON(product)
	})

	// Cart routes
	api.Get("/cart", getCart)
	api.Post("/cart", addToCart)

	// Order routes
	api.Get("/orders", getOrders)
	api.Post("/orders", placeOrder)
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

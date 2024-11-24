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
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	Category      string  `json:"category"`
	Price         float64 `json:"price"`
	SalePrice     float64 `json:"salePrice"`
	OnSale        bool    `json:"onSale"`
	InStock       bool    `json:"inStock"`
	StockQuantity int     `json:"stockQuantity"`
}

type CartItem struct {
	ProductID string  `json:"productId"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
}

type Cart struct {
	ID        string     `json:"id"`
	UserEmail string     `json:"userEmail"`
	Items     []CartItem `json:"items"`
	Subtotal  float64    `json:"subtotal"`
	Tax       float64    `json:"tax"`
	Total     float64    `json:"total"`
}

type Order struct {
	ID              string     `json:"id"`
	UserEmail       string     `json:"userEmail"`
	Items           []CartItem `json:"items"`
	Subtotal        float64    `json:"subtotal"`
	Tax             float64    `json:"tax"`
	Total           float64    `json:"total"`
	Status          string     `json:"status"`
	ShippingAddress Address    `json:"shippingAddress"`
	CreatedAt       time.Time  `json:"createdAt"`
}

type Address struct {
	Street  string `json:"street"`
	City    string `json:"city"`
	State   string `json:"state"`
	ZipCode string `json:"zipCode"`
}

type User struct {
	Email          string   `json:"email"`
	Name           string   `json:"name"`
	Address        Address  `json:"address"`
	PaymentMethods []string `json:"paymentMethods"`
}

type Database struct {
	Products map[string]Product `json:"products"`
	Carts    map[string]Cart    `json:"carts"`
	Orders   map[string]Order   `json:"orders"`
	Users    map[string]User    `json:"users"`
	mu       sync.RWMutex
}

var db *Database

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Products: make(map[string]Product),
		Carts:    make(map[string]Cart),
		Orders:   make(map[string]Order),
		Users:    make(map[string]User),
	}

	return json.Unmarshal(data, db)
}

func getProducts(c *fiber.Ctx) error {
	category := c.Query("category")
	search := c.Query("search")
	onSale := c.Query("onSale") == "true"

	var products []Product
	db.mu.RLock()
	for _, p := range db.Products {
		if category != "" && p.Category != category {
			continue
		}
		if onSale && !p.OnSale {
			continue
		}
		// Simple search implementation
		if search != "" && !contains(p.Name, search) && !contains(p.Description, search) {
			continue
		}
		products = append(products, p)
	}
	db.mu.RUnlock()

	return c.JSON(products)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[0:len(substr)] == substr
}

func getCart(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	for _, cart := range db.Carts {
		if cart.UserEmail == email {
			return c.JSON(cart)
		}
	}

	// Create new cart if none exists
	newCart := Cart{
		ID:        uuid.New().String(),
		UserEmail: email,
		Items:     []CartItem{},
	}
	db.Carts[newCart.ID] = newCart

	return c.JSON(newCart)
}

func addToCart(c *fiber.Ctx) error {
	var req struct {
		UserEmail string `json:"userEmail"`
		ProductID string `json:"productId"`
		Quantity  int    `json:"quantity"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	// Find user's cart
	var userCart *Cart
	for _, cart := range db.Carts {
		if cart.UserEmail == req.UserEmail {
			userCart = &cart
			break
		}
	}

	if userCart == nil {
		userCart = &Cart{
			ID:        uuid.New().String(),
			UserEmail: req.UserEmail,
			Items:     []CartItem{},
		}
	}

	// Verify product exists and is in stock
	product, exists := db.Products[req.ProductID]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Product not found",
		})
	}

	if !product.InStock || product.StockQuantity < req.Quantity {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Product out of stock",
		})
	}

	// Add item to cart
	price := product.Price
	if product.OnSale {
		price = product.SalePrice
	}

	// Update existing item or add new one
	itemExists := false
	for i, item := range userCart.Items {
		if item.ProductID == req.ProductID {
			userCart.Items[i].Quantity += req.Quantity
			userCart.Items[i].Price = price
			itemExists = true
			break
		}
	}

	if !itemExists {
		userCart.Items = append(userCart.Items, CartItem{
			ProductID: req.ProductID,
			Quantity:  req.Quantity,
			Price:     price,
		})
	}

	// Recalculate totals
	userCart.Subtotal = 0
	for _, item := range userCart.Items {
		userCart.Subtotal += item.Price * float64(item.Quantity)
	}
	userCart.Tax = userCart.Subtotal * 0.0825 // 8.25% tax rate
	userCart.Total = userCart.Subtotal + userCart.Tax

	db.Carts[userCart.ID] = *userCart

	return c.JSON(userCart)
}

func getOrders(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	var orders []Order
	db.mu.RLock()
	for _, order := range db.Orders {
		if order.UserEmail == email {
			orders = append(orders, order)
		}
	}
	db.mu.RUnlock()

	return c.JSON(orders)
}

func placeOrder(c *fiber.Ctx) error {
	var req struct {
		UserEmail       string  `json:"userEmail"`
		ShippingAddress Address `json:"shippingAddress"`
		PaymentMethodID string  `json:"paymentMethodId"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	// Find user's cart
	var userCart *Cart
	for _, cart := range db.Carts {
		if cart.UserEmail == req.UserEmail {
			userCart = &cart
			break
		}
	}

	if userCart == nil || len(userCart.Items) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cart is empty",
		})
	}

	// Verify user and payment method
	user, exists := db.Users[req.UserEmail]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	validPayment := false
	for _, pm := range user.PaymentMethods {
		if pm == req.PaymentMethodID {
			validPayment = true
			break
		}
	}
	if !validPayment {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid payment method",
		})
	}

	// Create order
	order := Order{
		ID:              uuid.New().String(),
		UserEmail:       req.UserEmail,
		Items:           userCart.Items,
		Subtotal:        userCart.Subtotal,
		Tax:             userCart.Tax,
		Total:           userCart.Total,
		Status:          "pending",
		ShippingAddress: req.ShippingAddress,
		CreatedAt:       time.Now(),
	}

	// Update inventory
	for _, item := range order.Items {
		product := db.Products[item.ProductID]
		product.StockQuantity -= item.Quantity
		if product.StockQuantity == 0 {
			product.InStock = false
		}
		db.Products[item.ProductID] = product
	}

	// Save order and clear cart
	db.Orders[order.ID] = order
	delete(db.Carts, userCart.ID)

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
	api.Get("/products", getProducts)

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

	app := fiber.New()

	app.Use(logger.New())
	app.Use(cors.New())

	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

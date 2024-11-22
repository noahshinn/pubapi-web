package main

import (
	"encoding/json"
	"fmt"
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

// Models
type Product struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Category      string    `json:"category"`
	Price         float64   `json:"price"`
	SalePrice     *float64  `json:"sale_price,omitempty"`
	InStock       bool      `json:"in_stock"`
	StockQuantity int       `json:"stock_quantity"`
	CreatedAt     time.Time `json:"created_at"`
}

type CartItem struct {
	ProductID string  `json:"product_id"`
	Product   Product `json:"product"`
	Quantity  int     `json:"quantity"`
	UserEmail string  `json:"user_email"`
}

type Cart struct {
	Items    []CartItem `json:"items"`
	Subtotal float64    `json:"subtotal"`
	Total    float64    `json:"total"`
}

type Order struct {
	ID              string     `json:"id"`
	UserEmail       string     `json:"user_email"`
	Items           []CartItem `json:"items"`
	Status          string     `json:"status"`
	Total           float64    `json:"total"`
	ShippingAddress string     `json:"shipping_address"`
	PaymentMethod   string     `json:"payment_method"`
	CreatedAt       time.Time  `json:"created_at"`
}

type User struct {
	Email          string          `json:"email"`
	Name           string          `json:"name"`
	Address        string          `json:"address"`
	PaymentMethods []PaymentMethod `json:"payment_methods"`
	CreatedAt      time.Time       `json:"created_at"`
}

type PaymentMethod struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Last4    string `json:"last4"`
	ExpiryMM int    `json:"expiry_mm"`
	ExpiryYY int    `json:"expiry_yy"`
}

// Database
type Database struct {
	Products map[string]Product    `json:"products"`
	Carts    map[string][]CartItem `json:"carts"` // key: user_email
	Orders   map[string]Order      `json:"orders"`
	Users    map[string]User       `json:"users"`
	mu       sync.RWMutex
}

var db *Database

// Database operations
func (d *Database) GetProduct(id string) (Product, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if product, exists := d.Products[id]; exists {
		return product, nil
	}
	return Product{}, fmt.Errorf("product not found")
}

func (d *Database) GetUserCart(email string) []CartItem {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.Carts[email]
}

func (d *Database) AddToCart(email string, item CartItem) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Validate product exists and is in stock
	product, exists := d.Products[item.ProductID]
	if !exists {
		return fmt.Errorf("product not found")
	}

	if !product.InStock || product.StockQuantity < item.Quantity {
		return fmt.Errorf("product out of stock")
	}

	// Add to cart
	currentCart := d.Carts[email]

	// Check if product already in cart
	for i, cartItem := range currentCart {
		if cartItem.ProductID == item.ProductID {
			currentCart[i].Quantity += item.Quantity
			d.Carts[email] = currentCart
			return nil
		}
	}

	// Add new item
	item.Product = product
	d.Carts[email] = append(currentCart, item)
	return nil
}

func (d *Database) CreateOrder(order Order) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Validate all products and update stock
	for _, item := range order.Items {
		product, exists := d.Products[item.ProductID]
		if !exists {
			return fmt.Errorf("product %s not found", item.ProductID)
		}

		if !product.InStock || product.StockQuantity < item.Quantity {
			return fmt.Errorf("product %s out of stock", item.ProductID)
		}

		// Update stock
		product.StockQuantity -= item.Quantity
		product.InStock = product.StockQuantity > 0
		d.Products[item.ProductID] = product
	}

	// Save order
	d.Orders[order.ID] = order

	// Clear user's cart
	delete(d.Carts, order.UserEmail)

	return nil
}

// Handlers
func getProducts(c *fiber.Ctx) error {
	category := c.Query("category")
	search := strings.ToLower(c.Query("search"))
	onSale := c.QueryBool("onSale")

	var products []Product

	db.mu.RLock()
	for _, product := range db.Products {
		// Apply filters
		if category != "" && product.Category != category {
			continue
		}

		if search != "" && !strings.Contains(strings.ToLower(product.Name), search) {
			continue
		}

		if onSale && product.SalePrice == nil {
			continue
		}

		products = append(products, product)
	}
	db.mu.RUnlock()

	return c.JSON(products)
}

func getCart(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	items := db.GetUserCart(email)

	// Calculate totals
	var subtotal float64
	for _, item := range items {
		price := item.Product.Price
		if item.Product.SalePrice != nil {
			price = *item.Product.SalePrice
		}
		subtotal += price * float64(item.Quantity)
	}

	cart := Cart{
		Items:    items,
		Subtotal: subtotal,
		Total:    subtotal * 1.08, // Adding 8% tax
	}

	return c.JSON(cart)
}

func addToCart(c *fiber.Ctx) error {
	var item CartItem
	if err := c.BodyParser(&item); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if err := db.AddToCart(item.UserEmail, item); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.SendStatus(fiber.StatusCreated)
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

func createOrder(c *fiber.Ctx) error {
	var req struct {
		UserEmail       string `json:"user_email"`
		ShippingAddress string `json:"shipping_address"`
		PaymentMethod   string `json:"payment_method"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Get user's cart
	items := db.GetUserCart(req.UserEmail)
	if len(items) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cart is empty",
		})
	}

	// Calculate total
	var total float64
	for _, item := range items {
		price := item.Product.Price
		if item.Product.SalePrice != nil {
			price = *item.Product.SalePrice
		}
		total += price * float64(item.Quantity)
	}
	total *= 1.08 // Add 8% tax

	order := Order{
		ID:              uuid.New().String(),
		UserEmail:       req.UserEmail,
		Items:           items,
		Status:          "pending",
		Total:           total,
		ShippingAddress: req.ShippingAddress,
		PaymentMethod:   req.PaymentMethod,
		CreatedAt:       time.Now(),
	}

	if err := db.CreateOrder(order); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(order)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Products: make(map[string]Product),
		Carts:    make(map[string][]CartItem),
		Orders:   make(map[string]Order),
		Users:    make(map[string]User),
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

	// Cart routes
	api.Get("/cart", getCart)
	api.Post("/cart", addToCart)

	// Order routes
	api.Get("/orders", getOrders)
	api.Post("/orders", createOrder)
}

func main() {
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

	log.Fatal(app.Listen(":3000"))
}

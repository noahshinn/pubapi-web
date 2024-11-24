package main

import (
	"encoding/json"
	"errors"
	"flag"
	"log"
	"os"
	"strings"
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
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Price         float64   `json:"price"`
	Category      string    `json:"category"`
	Rating        float64   `json:"rating"`
	ReviewsCount  int       `json:"reviews_count"`
	InStock       bool      `json:"in_stock"`
	PrimeEligible bool      `json:"prime_eligible"`
	CreatedAt     time.Time `json:"created_at"`
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
	UpdatedAt time.Time  `json:"updated_at"`
}

type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusPaid      OrderStatus = "paid"
	OrderStatusShipped   OrderStatus = "shipped"
	OrderStatusDelivered OrderStatus = "delivered"
	OrderStatusCancelled OrderStatus = "cancelled"
)

type Order struct {
	ID              string      `json:"id"`
	UserEmail       string      `json:"user_email"`
	Items           []CartItem  `json:"items"`
	Status          OrderStatus `json:"status"`
	ShippingAddress string      `json:"shipping_address"`
	PaymentMethod   string      `json:"payment_method"`
	Subtotal        float64     `json:"subtotal"`
	Shipping        float64     `json:"shipping"`
	Tax             float64     `json:"tax"`
	Total           float64     `json:"total"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

type User struct {
	Email          string    `json:"email"`
	Name           string    `json:"name"`
	PrimeMember    bool      `json:"prime_member"`
	Address        string    `json:"address"`
	PaymentMethods []string  `json:"payment_methods"`
	JoinDate       time.Time `json:"join_date"`
}

// Database represents our in-memory database
type Database struct {
	Users    map[string]User    `json:"users"`
	Products map[string]Product `json:"products"`
	Carts    map[string]Cart    `json:"carts"`
	Orders   map[string]Order   `json:"orders"`
	mu       sync.RWMutex
}

var (
	db                 *Database
	ErrUserNotFound    = errors.New("user not found")
	ErrProductNotFound = errors.New("product not found")
	ErrCartNotFound    = errors.New("cart not found")
	ErrOrderNotFound   = errors.New("order not found")
)

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

func (d *Database) GetCart(email string) (Cart, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	cart, exists := d.Carts[email]
	if !exists {
		return Cart{}, ErrCartNotFound
	}
	return cart, nil
}

func (d *Database) UpdateCart(cart Cart) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Carts[cart.UserEmail] = cart
	return nil
}

func (d *Database) CreateOrder(order Order) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Orders[order.ID] = order
	return nil
}

// HTTP Handlers
func searchProducts(c *fiber.Ctx) error {
	query := c.Query("query")
	category := c.Query("category")

	var results []Product
	db.mu.RLock()
	for _, product := range db.Products {
		if (query == "" || containsIgnoreCase(product.Name, query) ||
			containsIgnoreCase(product.Description, query)) &&
			(category == "" || product.Category == category) {
			results = append(results, product)
		}
	}
	db.mu.RUnlock()

	return c.JSON(results)
}

func getCart(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	cart, err := db.GetCart(email)
	if err != nil {
		if err == ErrCartNotFound {
			// Create empty cart for new users
			cart = Cart{
				UserEmail: email,
				Items:     []CartItem{},
				UpdatedAt: time.Now(),
			}
			db.UpdateCart(cart)
		} else {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": err.Error(),
			})
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

	// Get or create cart
	cart, _ := db.GetCart(req.UserEmail)
	if cart.UserEmail == "" {
		cart = Cart{
			UserEmail: req.UserEmail,
			Items:     []CartItem{},
		}
	}

	// Add or update item in cart
	itemFound := false
	for i, item := range cart.Items {
		if item.ProductID == req.ProductID {
			cart.Items[i].Quantity += req.Quantity
			itemFound = true
			break
		}
	}

	if !itemFound {
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

	cart.Shipping = 0
	if !user.PrimeMember && cart.Subtotal < 25 {
		cart.Shipping = 5.99
	}

	cart.Tax = cart.Subtotal * 0.0825 // 8.25% tax rate
	cart.Total = cart.Subtotal + cart.Shipping + cart.Tax
	cart.UpdatedAt = time.Now()

	// Save updated cart
	if err := db.UpdateCart(cart); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update cart",
		})
	}

	return c.JSON(cart)
}

func placeOrder(c *fiber.Ctx) error {
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
	cart, err := db.GetCart(req.UserEmail)

	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Cart not found",
		})
	}

	if len(cart.Items) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cart is empty",
		})
	}

	// Create new order
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

	// Save order
	if err := db.CreateOrder(order); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create order",
		})
	}

	// Clear cart
	cart.Items = []CartItem{}
	cart.Subtotal = 0
	cart.Shipping = 0
	cart.Tax = 0
	cart.Total = 0
	cart.UpdatedAt = time.Now()
	db.UpdateCart(cart)

	return c.Status(fiber.StatusCreated).JSON(order)
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
		if order.UserEmail == email {
			userOrders = append(userOrders, order)
		}
	}
	db.mu.RUnlock()

	return c.JSON(userOrders)
}

func containsIgnoreCase(s, substr string) bool {
	s, substr = strings.ToLower(s), strings.ToLower(substr)
	return strings.Contains(s, substr)
}

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

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Product routes
	api.Get("/products", searchProducts)
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
	api.Get("/orders", getUserOrders)
	api.Post("/orders", placeOrder)
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

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
type Address struct {
	Street    string  `json:"street"`
	City      string  `json:"city"`
	State     string  `json:"state"`
	ZipCode   string  `json:"zip_code"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type Store struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Address Address `json:"address"`
	Phone   string  `json:"phone"`
	Hours   string  `json:"hours"`
	IsOpen  bool    `json:"is_open"`
}

type Product struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Price       float64        `json:"price"`
	Category    string         `json:"category"`
	Brand       string         `json:"brand"`
	SKU         string         `json:"sku"`
	Inventory   map[string]int `json:"inventory"` // store_id -> quantity
	CreatedAt   time.Time      `json:"created_at"`
}

type User struct {
	Email     string  `json:"email"`
	Name      string  `json:"name"`
	Phone     string  `json:"phone"`
	Address   Address `json:"address"`
	ProMember bool    `json:"pro_member"`
}

type CartItem struct {
	ProductID string  `json:"product_id"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
}

type Cart struct {
	UserEmail string     `json:"user_email"`
	Items     []CartItem `json:"items"`
	StoreID   string     `json:"store_id"`
	Total     float64    `json:"total"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusConfirmed OrderStatus = "confirmed"
	OrderStatusReady     OrderStatus = "ready"
	OrderStatusCompleted OrderStatus = "completed"
	OrderStatusCancelled OrderStatus = "cancelled"
)

type DeliveryMethod string

const (
	DeliveryMethodPickup   DeliveryMethod = "pickup"
	DeliveryMethodDelivery DeliveryMethod = "delivery"
)

type Order struct {
	ID             string         `json:"id"`
	UserEmail      string         `json:"user_email"`
	Items          []CartItem     `json:"items"`
	Status         OrderStatus    `json:"status"`
	StoreID        string         `json:"store_id"`
	DeliveryMethod DeliveryMethod `json:"delivery_method"`
	Subtotal       float64        `json:"subtotal"`
	Tax            float64        `json:"tax"`
	Total          float64        `json:"total"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// Database represents our in-memory database
type Database struct {
	Users    map[string]User    `json:"users"`
	Products map[string]Product `json:"products"`
	Stores   map[string]Store   `json:"stores"`
	Carts    map[string]Cart    `json:"carts"`
	Orders   map[string]Order   `json:"orders"`
	mu       sync.RWMutex
}

// Global database instance
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

func (d *Database) GetStore(id string) (Store, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	store, exists := d.Stores[id]
	if !exists {
		return Store{}, errors.New("store not found")
	}
	return store, nil
}

func (d *Database) GetCart(userEmail string) (Cart, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	cart, exists := d.Carts[userEmail]
	if !exists {
		return Cart{}, errors.New("cart not found")
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
	storeID := c.Query("store_id")

	var products []Product

	db.mu.RLock()
	for _, product := range db.Products {
		// Apply filters
		if query != "" && !contains(product.Name, query) && !contains(product.Description, query) {
			continue
		}
		if category != "" && product.Category != category {
			continue
		}
		if storeID != "" {
			if quantity, exists := product.Inventory[storeID]; !exists || quantity <= 0 {
				continue
			}
		}
		products = append(products, product)
	}
	db.mu.RUnlock()

	return c.JSON(products)
}

func getNearbyStores(c *fiber.Ctx) error {
	lat := c.QueryFloat("latitude", 0)
	lon := c.QueryFloat("longitude", 0)

	if lat == 0 || lon == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "latitude and longitude are required",
		})
	}

	var nearbyStores []Store
	maxDistance := 50.0 // Maximum radius in km

	db.mu.RLock()
	for _, store := range db.Stores {
		distance := calculateDistance(lat, lon,
			store.Address.Latitude,
			store.Address.Longitude)

		if distance <= maxDistance {
			nearbyStores = append(nearbyStores, store)
		}
	}
	db.mu.RUnlock()

	return c.JSON(nearbyStores)
}

func getUserCart(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	cart, err := db.GetCart(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(cart)
}

type AddToCartRequest struct {
	UserEmail string `json:"user_email"`
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
	StoreID   string `json:"store_id"`
}

func addToCart(c *fiber.Ctx) error {
	var req AddToCartRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user
	if _, err := db.GetUser(req.UserEmail); err != nil {
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

	// Check inventory
	if quantity, exists := product.Inventory[req.StoreID]; !exists || quantity < req.Quantity {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Insufficient inventory",
		})
	}

	// Get or create cart
	cart, err := db.GetCart(req.UserEmail)
	if err != nil {
		cart = Cart{
			UserEmail: req.UserEmail,
			StoreID:   req.StoreID,
			Items:     []CartItem{},
			UpdatedAt: time.Now(),
		}
	}

	// Validate store consistency
	if cart.StoreID != "" && cart.StoreID != req.StoreID {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Items must be from the same store",
		})
	}

	// Add or update item
	found := false
	for i, item := range cart.Items {
		if item.ProductID == req.ProductID {
			cart.Items[i].Quantity += req.Quantity
			found = true
			break
		}
	}

	if !found {
		cart.Items = append(cart.Items, CartItem{
			ProductID: req.ProductID,
			Quantity:  req.Quantity,
			Price:     product.Price,
		})
	}

	// Update cart total
	var total float64
	for _, item := range cart.Items {
		product, _ := db.GetProduct(item.ProductID)
		total += product.Price * float64(item.Quantity)
	}
	cart.Total = total
	cart.UpdatedAt = time.Now()

	// Save cart
	if err := db.UpdateCart(cart); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update cart",
		})
	}

	return c.JSON(cart)
}

type CreateOrderRequest struct {
	UserEmail      string         `json:"user_email"`
	DeliveryMethod DeliveryMethod `json:"delivery_method"`
}

func createOrder(c *fiber.Ctx) error {
	var req CreateOrderRequest
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

	// Calculate totals
	subtotal := cart.Total
	tax := subtotal * 0.0825 // 8.25% tax rate
	total := subtotal + tax

	// Create order
	order := Order{
		ID:             uuid.New().String(),
		UserEmail:      req.UserEmail,
		Items:          cart.Items,
		Status:         OrderStatusPending,
		StoreID:        cart.StoreID,
		DeliveryMethod: req.DeliveryMethod,
		Subtotal:       subtotal,
		Tax:            tax,
		Total:          total,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Save order
	if err := db.CreateOrder(order); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create order",
		})
	}

	// Clear cart
	cart.Items = []CartItem{}
	cart.Total = 0
	cart.UpdatedAt = time.Now()
	db.UpdateCart(cart)

	return c.Status(fiber.StatusCreated).JSON(order)
}

func getUserOrders(c *fiber.Ctx) error {
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

// Utility functions
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	// Simplified distance calculation
	return ((lat2 - lat1) * (lat2 - lat1)) + ((lon2 - lon1) * (lon2 - lon1))
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:    make(map[string]User),
		Products: make(map[string]Product),
		Stores:   make(map[string]Store),
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

	// Store routes
	api.Get("/stores", getNearbyStores)
	api.Get("/stores/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		store, err := db.GetStore(id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.JSON(store)
	})

	// Cart routes
	api.Get("/cart", getUserCart)
	api.Post("/cart/items", addToCart)

	// Order routes
	api.Get("/orders", getUserOrders)
	api.Post("/orders", createOrder)
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

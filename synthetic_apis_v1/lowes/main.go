package main

import (
	"encoding/json"
	"errors"
	"flag"
	"log"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

// Domain Models
type Product struct {
	ID             string                 `json:"id"`
	Name           string                 `json:"name"`
	Description    string                 `json:"description"`
	Category       string                 `json:"category"`
	Price          float64                `json:"price"`
	Inventory      int                    `json:"inventory"`
	Brand          string                 `json:"brand"`
	ModelNumber    string                 `json:"model_number"`
	Specifications map[string]interface{} `json:"specifications"`
}

type Store struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Address   string  `json:"address"`
	Phone     string  `json:"phone"`
	Hours     string  `json:"hours"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type CartItem struct {
	ProductID string  `json:"product_id"`
	Product   Product `json:"product"`
	Quantity  int     `json:"quantity"`
	StoreID   string  `json:"store_id"`
}

type Cart struct {
	UserEmail string     `json:"user_email"`
	Items     []CartItem `json:"items"`
	Total     float64    `json:"total"`
}

type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusConfirmed OrderStatus = "confirmed"
	OrderStatusReady     OrderStatus = "ready"
	OrderStatusPickedUp  OrderStatus = "picked_up"
	OrderStatusCancelled OrderStatus = "cancelled"
)

type Order struct {
	ID        string      `json:"id"`
	UserEmail string      `json:"user_email"`
	Items     []CartItem  `json:"items"`
	Total     float64     `json:"total"`
	Status    OrderStatus `json:"status"`
	StoreID   string      `json:"store_id"`
	CreatedAt time.Time   `json:"created_at"`
}

type User struct {
	Email    string   `json:"email"`
	Name     string   `json:"name"`
	Phone    string   `json:"phone"`
	Address  string   `json:"address"`
	StoreID  string   `json:"preferred_store_id"`
	OrderIDs []string `json:"order_ids"`
}

// Database represents our in-memory database
type Database struct {
	Products map[string]Product `json:"products"`
	Stores   map[string]Store   `json:"stores"`
	Carts    map[string]Cart    `json:"carts"`
	Orders   map[string]Order   `json:"orders"`
	Users    map[string]User    `json:"users"`
	mu       sync.RWMutex
}

var db *Database

// Database operations
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

func (d *Database) GetCart(email string) (Cart, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	cart, exists := d.Carts[email]
	if !exists {
		return Cart{UserEmail: email, Items: []CartItem{}}, nil
	}
	return cart, nil
}

func (d *Database) UpdateCart(cart Cart) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Carts[cart.UserEmail] = cart
	return nil
}

// HTTP Handlers
func searchProducts(c *fiber.Ctx) error {
	query := c.Query("query")
	category := c.Query("category")
	storeID := c.Query("store_id")

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

	if storeID != "" {
		// Filter by store inventory if store_id is provided
		var filteredResults []Product
		for _, product := range results {
			if product.Inventory > 0 {
				filteredResults = append(filteredResults, product)
			}
		}
		results = filteredResults
	}

	return c.JSON(results)
}

func findNearbyStores(c *fiber.Ctx) error {
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
		distance := calculateDistance(lat, lon, store.Latitude, store.Longitude)
		if distance <= maxDistance {
			nearbyStores = append(nearbyStores, store)
		}
	}
	db.mu.RUnlock()

	return c.JSON(nearbyStores)
}

func getCart(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	cart, err := db.GetCart(email)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
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

	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	// Get current cart
	cart, err := db.GetCart(email)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Get product details
	product, err := db.GetProduct(item.ProductID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Product not found",
		})
	}

	// Add item to cart
	item.Product = product
	cart.Items = append(cart.Items, item)

	// Recalculate total
	var total float64
	for _, cartItem := range cart.Items {
		total += cartItem.Product.Price * float64(cartItem.Quantity)
	}
	cart.Total = total

	// Update cart in database
	if err := db.UpdateCart(cart); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(cart)
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
func containsIgnoreCase(s, substr string) bool {
	s, substr = strings.ToLower(s), strings.ToLower(substr)
	return strings.Contains(s, substr)
}

func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	// Simple distance calculation (not actual haversine formula)
	return math.Sqrt(math.Pow(lat2-lat1, 2) + math.Pow(lon2-lon1, 2))
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Products: make(map[string]Product),
		Stores:   make(map[string]Store),
		Carts:    make(map[string]Cart),
		Orders:   make(map[string]Order),
		Users:    make(map[string]User),
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
	api.Get("/stores", findNearbyStores)
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
	api.Get("/cart", getCart)
	api.Post("/cart", addToCart)

	// Order routes
	api.Get("/orders", getUserOrders)
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
	app.Use(cors.New())

	// Setup routes
	setupRoutes(app)

	// Start server
	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

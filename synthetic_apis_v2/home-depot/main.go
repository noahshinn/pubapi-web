package main

import (
	"encoding/json"
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
	"github.com/google/uuid"
)

// Data models
type Product struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Price       float64        `json:"price"`
	Category    string         `json:"category"`
	Brand       string         `json:"brand"`
	ModelNumber string         `json:"model_number"`
	Inventory   map[string]int `json:"inventory"` // store_id -> quantity
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

type OrderItem struct {
	ProductID string  `json:"product_id"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
}

type Order struct {
	ID             string      `json:"id"`
	UserEmail      string      `json:"user_email"`
	Items          []OrderItem `json:"items"`
	Total          float64     `json:"total"`
	Status         string      `json:"status"`
	DeliveryMethod string      `json:"delivery_method"`
	StoreID        string      `json:"store_id"`
	CreatedAt      time.Time   `json:"created_at"`
}

type CartItem struct {
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

type Cart struct {
	UserEmail string     `json:"user_email"`
	Items     []CartItem `json:"items"`
	Total     float64    `json:"total"`
}

type User struct {
	Email     string   `json:"email"`
	Name      string   `json:"name"`
	Phone     string   `json:"phone"`
	Address   string   `json:"address"`
	ProXID    string   `json:"pro_x_id,omitempty"`
	SavedList []string `json:"saved_list"` // Product IDs
}

// Database
type Database struct {
	Products map[string]Product `json:"products"`
	Stores   map[string]Store   `json:"stores"`
	Orders   map[string]Order   `json:"orders"`
	Users    map[string]User    `json:"users"`
	Carts    map[string]Cart    `json:"carts"`
	mu       sync.RWMutex
}

var db *Database

// Handlers
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

func findStores(c *fiber.Ctx) error {
	lat := c.QueryFloat("latitude", 0)
	lon := c.QueryFloat("longitude", 0)

	if lat == 0 || lon == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "latitude and longitude are required",
		})
	}

	var stores []Store
	maxDistance := 50.0 // km

	db.mu.RLock()
	for _, store := range db.Stores {
		if distance(lat, lon, store.Latitude, store.Longitude) <= maxDistance {
			stores = append(stores, store)
		}
	}
	db.mu.RUnlock()

	return c.JSON(stores)
}

func getUserOrders(c *fiber.Ctx) error {
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
	var newOrder struct {
		UserEmail      string      `json:"user_email"`
		Items          []OrderItem `json:"items"`
		DeliveryMethod string      `json:"delivery_method"`
		StoreID        string      `json:"store_id"`
	}

	if err := c.BodyParser(&newOrder); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user
	db.mu.RLock()
	if _, exists := db.Users[newOrder.UserEmail]; !exists {
		db.mu.RUnlock()
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// Calculate total and validate inventory
	var total float64
	for _, item := range newOrder.Items {
		product, exists := db.Products[item.ProductID]
		if !exists {
			db.mu.RUnlock()
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Product not found: " + item.ProductID,
			})
		}

		if quantity, exists := product.Inventory[newOrder.StoreID]; !exists || quantity < item.Quantity {
			db.mu.RUnlock()
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Insufficient inventory for product: " + item.ProductID,
			})
		}

		total += product.Price * float64(item.Quantity)
	}
	db.mu.RUnlock()

	order := Order{
		ID:             uuid.New().String(),
		UserEmail:      newOrder.UserEmail,
		Items:          newOrder.Items,
		Total:          total,
		Status:         "pending",
		DeliveryMethod: newOrder.DeliveryMethod,
		StoreID:        newOrder.StoreID,
		CreatedAt:      time.Now(),
	}

	// Update inventory and save order
	db.mu.Lock()
	for _, item := range order.Items {
		product := db.Products[item.ProductID]
		product.Inventory[newOrder.StoreID] -= item.Quantity
		db.Products[item.ProductID] = product
	}
	db.Orders[order.ID] = order
	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(order)
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
			Total:     0,
		}
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

	db.mu.Lock()
	cart, exists := db.Carts[email]
	if !exists {
		cart = Cart{
			UserEmail: email,
			Items:     make([]CartItem, 0),
		}
	}

	// Update or add item
	found := false
	for i, existingItem := range cart.Items {
		if existingItem.ProductID == item.ProductID {
			cart.Items[i].Quantity += item.Quantity
			found = true
			break
		}
	}
	if !found {
		cart.Items = append(cart.Items, item)
	}

	// Recalculate total
	var total float64
	for _, cartItem := range cart.Items {
		if product, exists := db.Products[cartItem.ProductID]; exists {
			total += product.Price * float64(cartItem.Quantity)
		}
	}
	cart.Total = total

	db.Carts[email] = cart
	db.mu.Unlock()

	return c.JSON(cart)
}

// Utility functions
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

func distance(lat1, lon1, lat2, lon2 float64) float64 {
	// Simple Euclidean distance for demo purposes
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
		Orders:   make(map[string]Order),
		Users:    make(map[string]User),
		Carts:    make(map[string]Cart),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	// Serve OpenAPI spec
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

	// Store routes
	api.Get("/stores", findStores)

	// Order routes
	api.Get("/orders", getUserOrders)
	api.Post("/orders", createOrder)

	// Cart routes
	api.Get("/cart", getCart)
	api.Post("/cart", addToCart)
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

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
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

type MembershipType string

const (
	GoldStar      MembershipType = "gold_star"
	BusinessBasic MembershipType = "business"
	ExecutiveGold MembershipType = "executive"
)

type Membership struct {
	ID             string         `json:"id"`
	Type           MembershipType `json:"type"`
	Status         string         `json:"status"`
	ExpirationDate time.Time      `json:"expiration_date"`
	MemberSince    time.Time      `json:"member_since"`
	AutoRenewal    bool           `json:"auto_renewal"`
}

type User struct {
	Email      string     `json:"email"`
	Name       string     `json:"name"`
	Phone      string     `json:"phone"`
	Address    Address    `json:"address"`
	Membership Membership `json:"membership"`
}

type Product struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Category     string  `json:"category"`
	Price        float64 `json:"price"`
	ItemNumber   string  `json:"item_number"`
	Description  string  `json:"description"`
	InStock      bool    `json:"in_stock"`
	IsMemberOnly bool    `json:"is_member_only"`
}

type Warehouse struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Address  Address  `json:"address"`
	Phone    string   `json:"phone"`
	Hours    string   `json:"hours"`
	Services []string `json:"services"`
}

type OrderItem struct {
	ProductID string  `json:"product_id"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
}

type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusPaid      OrderStatus = "paid"
	OrderStatusReady     OrderStatus = "ready"
	OrderStatusCompleted OrderStatus = "completed"
	OrderStatusCancelled OrderStatus = "cancelled"
)

type Order struct {
	ID          string      `json:"id"`
	UserEmail   string      `json:"user_email"`
	Items       []OrderItem `json:"items"`
	Total       float64     `json:"total"`
	Tax         float64     `json:"tax"`
	WarehouseID string      `json:"warehouse_id"`
	Status      OrderStatus `json:"status"`
	OrderDate   time.Time   `json:"order_date"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// Database represents our in-memory database
type Database struct {
	Users      map[string]User      `json:"users"`
	Products   map[string]Product   `json:"products"`
	Warehouses map[string]Warehouse `json:"warehouses"`
	Orders     map[string]Order     `json:"orders"`
	mu         sync.RWMutex
}

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

func (d *Database) GetWarehouse(id string) (Warehouse, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	warehouse, exists := d.Warehouses[id]
	if !exists {
		return Warehouse{}, errors.New("warehouse not found")
	}
	return warehouse, nil
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
	search := c.Query("search")

	var products []Product
	db.mu.RLock()
	for _, product := range db.Products {
		if category != "" && product.Category != category {
			continue
		}
		if search != "" && !contains(product.Name, search) {
			continue
		}
		products = append(products, product)
	}
	db.mu.RUnlock()

	return c.JSON(products)
}

func getMembership(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	user, err := db.GetUser(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(user.Membership)
}

func getWarehouses(c *fiber.Ctx) error {
	lat := c.QueryFloat("latitude", 0)
	lon := c.QueryFloat("longitude", 0)

	if lat == 0 || lon == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "latitude and longitude are required",
		})
	}

	var nearbyWarehouses []Warehouse
	maxDistance := 50.0 // Maximum radius in km

	db.mu.RLock()
	for _, warehouse := range db.Warehouses {
		distance := calculateDistance(lat, lon,
			warehouse.Address.Latitude,
			warehouse.Address.Longitude)

		if distance <= maxDistance {
			nearbyWarehouses = append(nearbyWarehouses, warehouse)
		}
	}
	db.mu.RUnlock()

	return c.JSON(nearbyWarehouses)
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

type CreateOrderRequest struct {
	UserEmail   string      `json:"user_email"`
	WarehouseID string      `json:"warehouse_id"`
	Items       []OrderItem `json:"items"`
}

func createOrder(c *fiber.Ctx) error {
	var req CreateOrderRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user and membership
	user, err := db.GetUser(req.UserEmail)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if user.Membership.Status != "active" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Active membership required",
		})
	}

	// Validate warehouse
	_, err = db.GetWarehouse(req.WarehouseID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Calculate order total
	var total float64
	for _, item := range req.Items {
		product, err := db.GetProduct(item.ProductID)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": fmt.Sprintf("Product %s not found", item.ProductID),
			})
		}

		if !product.InStock {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": fmt.Sprintf("Product %s is out of stock", product.Name),
			})
		}

		if product.IsMemberOnly && user.Membership.Type == GoldStar {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": fmt.Sprintf("Product %s is for Executive members only", product.Name),
			})
		}

		total += product.Price * float64(item.Quantity)
	}

	tax := total * 0.0825 // 8.25% tax rate

	// Create new order
	order := Order{
		ID:          uuid.New().String(),
		UserEmail:   req.UserEmail,
		Items:       req.Items,
		Total:       total,
		Tax:         tax,
		WarehouseID: req.WarehouseID,
		Status:      OrderStatusPending,
		OrderDate:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Save order to database
	if err := db.CreateOrder(order); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create order",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(order)
}

// Helper functions
func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	// Simplified distance calculation
	return ((lat2 - lat1) * (lat2 - lat1)) + ((lon2 - lon1) * (lon2 - lon1))
}

func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:      make(map[string]User),
		Products:   make(map[string]Product),
		Warehouses: make(map[string]Warehouse),
		Orders:     make(map[string]Order),
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

	// Membership routes
	api.Get("/membership", getMembership)

	// Warehouse routes
	api.Get("/warehouses", getWarehouses)
	api.Get("/warehouses/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		warehouse, err := db.GetWarehouse(id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.JSON(warehouse)
	})

	// Order routes
	api.Get("/orders", getUserOrders)
	api.Post("/orders", createOrder)
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

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

type Membership struct {
	ID              string           `json:"id"`
	Type            string           `json:"type"`
	Status          string           `json:"status"`
	ExpirationDate  time.Time        `json:"expiration_date"`
	MemberSince     time.Time        `json:"member_since"`
	AutoRenewal     bool             `json:"auto_renewal"`
	AdditionalCards []AdditionalCard `json:"additional_cards"`
}

type AdditionalCard struct {
	HolderName   string `json:"holder_name"`
	Relationship string `json:"relationship"`
	CardNumber   string `json:"card_number"`
}

type Product struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Category      string  `json:"category"`
	Price         float64 `json:"price"`
	Unit          string  `json:"unit"`
	Stock         int     `json:"stock"`
	IsMembersOnly bool    `json:"is_members_only"`
	Description   string  `json:"description"`
	ImageURL      string  `json:"image_url"`
}

type OrderItem struct {
	ProductID string  `json:"product_id"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
}

type Order struct {
	ID          string      `json:"id"`
	WarehouseID string      `json:"warehouse_id"`
	UserEmail   string      `json:"user_email"`
	Items       []OrderItem `json:"items"`
	Total       float64     `json:"total"`
	Tax         float64     `json:"tax"`
	Date        time.Time   `json:"date"`
	Status      string      `json:"status"`
}

type Warehouse struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Address   string   `json:"address"`
	City      string   `json:"city"`
	State     string   `json:"state"`
	Zip       string   `json:"zip"`
	Latitude  float64  `json:"latitude"`
	Longitude float64  `json:"longitude"`
	Hours     string   `json:"hours"`
	Phone     string   `json:"phone"`
	Services  []string `json:"services"`
}

type Database struct {
	Memberships map[string]Membership `json:"memberships"`
	Products    map[string]Product    `json:"products"`
	Orders      map[string]Order      `json:"orders"`
	Warehouses  map[string]Warehouse  `json:"warehouses"`
	mu          sync.RWMutex
}

var db *Database

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Memberships: make(map[string]Membership),
		Products:    make(map[string]Product),
		Orders:      make(map[string]Order),
		Warehouses:  make(map[string]Warehouse),
	}

	return json.Unmarshal(data, db)
}

func getMembership(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email is required",
		})
	}

	db.mu.RLock()
	membership, exists := db.Memberships[email]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Membership not found",
		})
	}

	return c.JSON(membership)
}

func getProducts(c *fiber.Ctx) error {
	warehouseID := c.Query("warehouse_id")
	category := c.Query("category")

	if warehouseID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Warehouse ID is required",
		})
	}

	var products []Product
	db.mu.RLock()
	for _, product := range db.Products {
		if category != "" && product.Category != category {
			continue
		}
		products = append(products, product)
	}
	db.mu.RUnlock()

	return c.JSON(products)
}

func getOrders(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email is required",
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
		WarehouseID     string      `json:"warehouse_id"`
		Email           string      `json:"email"`
		Items           []OrderItem `json:"items"`
		PaymentMethodID string      `json:"payment_method_id"`
	}

	if err := c.BodyParser(&newOrder); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate membership
	db.mu.RLock()
	_, memberExists := db.Memberships[newOrder.Email]
	db.mu.RUnlock()

	if !memberExists {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Valid membership required",
		})
	}

	// Calculate total and validate products
	var total float64
	for _, item := range newOrder.Items {
		db.mu.RLock()
		product, exists := db.Products[item.ProductID]
		db.mu.RUnlock()

		if !exists {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid product ID: " + item.ProductID,
			})
		}

		if product.Stock < item.Quantity {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Insufficient stock for product: " + product.Name,
			})
		}

		total += product.Price * float64(item.Quantity)
	}

	tax := total * 0.0825 // 8.25% tax rate

	order := Order{
		ID:          uuid.New().String(),
		WarehouseID: newOrder.WarehouseID,
		UserEmail:   newOrder.Email,
		Items:       newOrder.Items,
		Total:       total,
		Tax:         tax,
		Date:        time.Now(),
		Status:      "pending",
	}

	db.mu.Lock()
	db.Orders[order.ID] = order
	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(order)
}

func getWarehouses(c *fiber.Ctx) error {
	lat := c.QueryFloat("latitude", 0)
	lon := c.QueryFloat("longitude", 0)

	if lat == 0 || lon == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Latitude and longitude are required",
		})
	}

	var warehouses []Warehouse
	db.mu.RLock()
	for _, warehouse := range db.Warehouses {
		// Simple distance calculation (not actual haversine formula)
		distance := ((warehouse.Latitude - lat) * (warehouse.Latitude - lat)) +
			((warehouse.Longitude - lon) * (warehouse.Longitude - lon))

		if distance <= 100 { // Arbitrary distance threshold
			warehouses = append(warehouses, warehouse)
		}
	}
	db.mu.RUnlock()

	return c.JSON(warehouses)
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

	// Membership routes
	api.Get("/membership", getMembership)

	// Product routes
	api.Get("/products", getProducts)

	// Order routes
	api.Get("/orders", getOrders)
	api.Post("/orders", createOrder)

	// Warehouse routes
	api.Get("/warehouses", getWarehouses)
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

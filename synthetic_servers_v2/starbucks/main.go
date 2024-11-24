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
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/google/uuid"
)

// Domain Models
type Store struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Address   string   `json:"address"`
	Latitude  float64  `json:"latitude"`
	Longitude float64  `json:"longitude"`
	Hours     string   `json:"hours"`
	Features  []string `json:"features"`
	IsOpen    bool     `json:"is_open"`
}

type CustomizationOption struct {
	Name          string   `json:"name"`
	Options       []string `json:"options"`
	MaxSelections int      `json:"max_selections"`
}

type MenuItem struct {
	ID                   string                `json:"id"`
	Name                 string                `json:"name"`
	Category             string                `json:"category"`
	Description          string                `json:"description"`
	Price                float64               `json:"price"`
	Calories             int                   `json:"calories"`
	CustomizationOptions []CustomizationOption `json:"customization_options"`
}

type Reward struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	StarsRequired int       `json:"stars_required"`
	ExpiresAt     time.Time `json:"expires_at"`
}

type Rewards struct {
	Stars             int      `json:"stars"`
	Tier              string   `json:"tier"`
	StarsToNextReward int      `json:"stars_to_next_reward"`
	AvailableRewards  []Reward `json:"available_rewards"`
}

type OrderItem struct {
	MenuItemID     string                 `json:"menu_item_id"`
	Name           string                 `json:"name"`
	Quantity       int                    `json:"quantity"`
	Price          float64                `json:"price"`
	Customizations map[string]interface{} `json:"customizations"`
}

type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusAccepted  OrderStatus = "accepted"
	OrderStatusPreparing OrderStatus = "preparing"
	OrderStatusReady     OrderStatus = "ready"
	OrderStatusCompleted OrderStatus = "completed"
	OrderStatusCancelled OrderStatus = "cancelled"
)

type Order struct {
	ID          string      `json:"id"`
	UserEmail   string      `json:"user_email"`
	Store       Store       `json:"store"`
	Items       []OrderItem `json:"items"`
	Status      OrderStatus `json:"status"`
	Total       float64     `json:"total"`
	StarsEarned int         `json:"stars_earned"`
	CreatedAt   time.Time   `json:"created_at"`
}

type Database struct {
	Stores    map[string]Store    `json:"stores"`
	MenuItems map[string]MenuItem `json:"menu_items"`
	Orders    map[string]Order    `json:"orders"`
	Rewards   map[string]Rewards  `json:"rewards"`
	mu        sync.RWMutex
}

var db *Database

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Stores:    make(map[string]Store),
		MenuItems: make(map[string]MenuItem),
		Orders:    make(map[string]Order),
		Rewards:   make(map[string]Rewards),
	}

	return json.Unmarshal(data, db)
}

func getStores(c *fiber.Ctx) error {
	lat := c.QueryFloat("latitude", 0)
	lon := c.QueryFloat("longitude", 0)

	if lat == 0 || lon == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "latitude and longitude are required",
		})
	}

	var nearbyStores []Store
	maxDistance := 10.0 // Maximum radius in km

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

func getMenu(c *fiber.Ctx) error {
	category := c.Query("category")

	var menuItems []MenuItem
	db.mu.RLock()
	for _, item := range db.MenuItems {
		if category == "" || item.Category == category {
			menuItems = append(menuItems, item)
		}
	}
	db.mu.RUnlock()

	return c.JSON(menuItems)
}

func getRewards(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	db.mu.RLock()
	rewards, exists := db.Rewards[email]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "user not found",
		})
	}

	return c.JSON(rewards)
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

type NewOrderRequest struct {
	StoreID       string      `json:"store_id"`
	UserEmail     string      `json:"user_email"`
	Items         []OrderItem `json:"items"`
	PaymentMethod string      `json:"payment_method"`
}

func createOrder(c *fiber.Ctx) error {
	var req NewOrderRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.RLock()
	store, exists := db.Stores[req.StoreID]
	if !exists {
		db.mu.RUnlock()
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "store not found",
		})
	}

	if !store.IsOpen {
		db.mu.RUnlock()
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "store is currently closed",
		})
	}

	// Calculate total and validate items
	var total float64
	for _, item := range req.Items {
		menuItem, exists := db.MenuItems[item.MenuItemID]
		if !exists {
			db.mu.RUnlock()
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "invalid menu item",
			})
		}
		total += menuItem.Price * float64(item.Quantity)
	}
	db.mu.RUnlock()

	// Calculate stars earned (2 stars per dollar spent)
	starsEarned := int(total * 2)

	order := Order{
		ID:          uuid.New().String(),
		UserEmail:   req.UserEmail,
		Store:       store,
		Items:       req.Items,
		Status:      OrderStatusPending,
		Total:       total,
		StarsEarned: starsEarned,
		CreatedAt:   time.Now(),
	}

	// Update database
	db.mu.Lock()
	db.Orders[order.ID] = order

	// Update user's rewards
	rewards := db.Rewards[req.UserEmail]
	rewards.Stars += starsEarned
	db.Rewards[req.UserEmail] = rewards
	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(order)
}

func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	// Simplified distance calculation
	return ((lat2 - lat1) * (lat2 - lat1)) + ((lon2 - lon1) * (lon2 - lon1))
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

	// Store routes
	api.Get("/stores", getStores)

	// Menu routes
	api.Get("/menu", getMenu)

	// Rewards routes
	api.Get("/rewards", getRewards)

	// Order routes
	api.Get("/orders", getOrders)
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

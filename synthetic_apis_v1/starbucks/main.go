package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
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

type UserRewards struct {
	Email            string   `json:"email"`
	Stars            int      `json:"stars"`
	Tier             string   `json:"tier"`
	RewardsAvailable []Reward `json:"rewards_available"`
}

type OrderItem struct {
	MenuItemID     string                 `json:"menu_item_id"`
	Quantity       int                    `json:"quantity"`
	Customizations map[string]interface{} `json:"customizations"`
}

type Order struct {
	ID          string      `json:"id"`
	UserEmail   string      `json:"user_email"`
	Store       Store       `json:"store"`
	Items       []OrderItem `json:"items"`
	Status      string      `json:"status"`
	Total       float64     `json:"total"`
	StarsEarned int         `json:"stars_earned"`
	RewardUsed  *Reward     `json:"reward_used,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
}

// Database represents our in-memory database
type Database struct {
	Stores      map[string]Store       `json:"stores"`
	Menu        map[string]MenuItem    `json:"menu"`
	UserRewards map[string]UserRewards `json:"user_rewards"`
	Orders      map[string]Order       `json:"orders"`
	mu          sync.RWMutex
}

// Global database instance
var db *Database

// Database operations
func (d *Database) GetStore(id string) (Store, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	store, exists := d.Stores[id]
	if !exists {
		return Store{}, errors.New("store not found")
	}
	return store, nil
}

func (d *Database) GetNearbyStores(lat, lon float64) []Store {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var nearby []Store
	maxDistance := 10.0 // km

	for _, store := range d.Stores {
		distance := calculateDistance(lat, lon, store.Latitude, store.Longitude)
		if distance <= maxDistance {
			nearby = append(nearby, store)
		}
	}

	return nearby
}

func (d *Database) GetUserRewards(email string) (UserRewards, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	rewards, exists := d.UserRewards[email]
	if !exists {
		return UserRewards{}, errors.New("user not found")
	}
	return rewards, nil
}

func (d *Database) UpdateUserRewards(email string, rewards UserRewards) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.UserRewards[email] = rewards
	return nil
}

func (d *Database) CreateOrder(order Order) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Orders[order.ID] = order
	return nil
}

func (d *Database) GetUserOrders(email string) []Order {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var userOrders []Order
	for _, order := range d.Orders {
		if order.UserEmail == email {
			userOrders = append(userOrders, order)
		}
	}
	return userOrders
}

// Utility functions
func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	// Simplified distance calculation
	return ((lat2 - lat1) * (lat2 - lat1)) + ((lon2 - lon1) * (lon2 - lon1))
}

func calculateStarsEarned(total float64) int {
	// Basic calculation: 2 stars per dollar spent
	return int(total * 2)
}

// HTTP Handlers
func getNearbyStores(c *fiber.Ctx) error {
	lat := c.QueryFloat("latitude", 0)
	lon := c.QueryFloat("longitude", 0)

	if lat == 0 || lon == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "latitude and longitude are required",
		})
	}

	stores := db.GetNearbyStores(lat, lon)
	return c.JSON(stores)
}

func getMenu(c *fiber.Ctx) error {
	category := c.Query("category")

	var menuItems []MenuItem
	db.mu.RLock()
	for _, item := range db.Menu {
		if category == "" || item.Category == category {
			menuItems = append(menuItems, item)
		}
	}
	db.mu.RUnlock()

	return c.JSON(menuItems)
}

func getUserRewards(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	rewards, err := db.GetUserRewards(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(rewards)
}

type CreateOrderRequest struct {
	StoreID        string      `json:"store_id"`
	UserEmail      string      `json:"user_email"`
	Items          []OrderItem `json:"items"`
	PaymentMethod  string      `json:"payment_method"`
	RedeemRewardID string      `json:"redeem_reward_id"`
}

func createOrder(c *fiber.Ctx) error {
	var req CreateOrderRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate store
	store, err := db.GetStore(req.StoreID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Store not found",
		})
	}

	// Calculate total and validate items
	var total float64
	for _, item := range req.Items {
		menuItem, exists := db.Menu[item.MenuItemID]
		if !exists {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": fmt.Sprintf("Menu item %s not found", item.MenuItemID),
			})
		}
		total += menuItem.Price * float64(item.Quantity)
	}

	// Handle reward redemption
	var usedReward *Reward
	if req.RedeemRewardID != "" {
		rewards, err := db.GetUserRewards(req.UserEmail)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "User rewards not found",
			})
		}

		// Find and validate reward
		for _, reward := range rewards.RewardsAvailable {
			if reward.ID == req.RedeemRewardID {
				if reward.ExpiresAt.Before(time.Now()) {
					return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
						"error": "Reward has expired",
					})
				}
				usedReward = &reward
				break
			}
		}

		if usedReward == nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid reward ID",
			})
		}
	}

	// Create order
	order := Order{
		ID:          uuid.New().String(),
		UserEmail:   req.UserEmail,
		Store:       store,
		Items:       req.Items,
		Status:      "pending",
		Total:       total,
		StarsEarned: calculateStarsEarned(total),
		RewardUsed:  usedReward,
		CreatedAt:   time.Now(),
	}

	// Save order
	if err := db.CreateOrder(order); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create order",
		})
	}

	// Update user rewards
	if rewards, err := db.GetUserRewards(req.UserEmail); err == nil {
		rewards.Stars += order.StarsEarned
		if usedReward != nil {
			// Remove used reward
			newRewards := []Reward{}
			for _, r := range rewards.RewardsAvailable {
				if r.ID != usedReward.ID {
					newRewards = append(newRewards, r)
				}
			}
			rewards.RewardsAvailable = newRewards
		}
		db.UpdateUserRewards(req.UserEmail, rewards)
	}

	return c.Status(fiber.StatusCreated).JSON(order)
}

func getUserOrders(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	orders := db.GetUserOrders(email)
	return c.JSON(orders)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Stores:      make(map[string]Store),
		Menu:        make(map[string]MenuItem),
		UserRewards: make(map[string]UserRewards),
		Orders:      make(map[string]Order),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	api.Get("/stores", getNearbyStores)
	api.Get("/menu", getMenu)
	api.Get("/rewards", getUserRewards)
	api.Post("/orders", createOrder)
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

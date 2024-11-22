package main

import (
	"encoding/json"
	"errors"
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
type App struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Developer       string    `json:"developer"`
	Category        string    `json:"category"`
	Price           float64   `json:"price"`
	Rating          float64   `json:"rating"`
	Downloads       string    `json:"downloads"`
	Description     string    `json:"description"`
	Version         string    `json:"version"`
	Size            string    `json:"size"`
	LastUpdated     time.Time `json:"last_updated"`
	RequiresAndroid string    `json:"requires_android"`
}

type UserApp struct {
	App         App       `json:"app"`
	PurchasedAt time.Time `json:"purchased_at"`
	LastUsed    time.Time `json:"last_used"`
	AutoUpdate  bool      `json:"auto_update"`
}

type User struct {
	Email          string    `json:"email"`
	Name           string    `json:"name"`
	PaymentMethods []Payment `json:"payment_methods"`
	Library        []UserApp `json:"library"`
	WishList       []App     `json:"wish_list"`
}

type Payment struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Last4 string `json:"last4"`
}

type Purchase struct {
	ID          string    `json:"id"`
	App         App       `json:"app"`
	UserEmail   string    `json:"user_email"`
	Amount      float64   `json:"amount"`
	PurchasedAt time.Time `json:"purchased_at"`
}

// Database represents our in-memory database
type Database struct {
	Users     map[string]User     `json:"users"`
	Apps      map[string]App      `json:"apps"`
	Purchases map[string]Purchase `json:"purchases"`
	mu        sync.RWMutex
}

// Custom errors
var (
	ErrUserNotFound = errors.New("user not found")
	ErrAppNotFound  = errors.New("app not found")
	ErrInvalidInput = errors.New("invalid input")
)

// Global database instance
var db *Database

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

func (d *Database) GetApp(id string) (App, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	app, exists := d.Apps[id]
	if !exists {
		return App{}, ErrAppNotFound
	}
	return app, nil
}

func (d *Database) AddPurchase(purchase Purchase) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Add purchase record
	d.Purchases[purchase.ID] = purchase

	// Update user's library
	user := d.Users[purchase.UserEmail]
	userApp := UserApp{
		App:         purchase.App,
		PurchasedAt: purchase.PurchasedAt,
		LastUsed:    time.Now(),
		AutoUpdate:  true,
	}
	user.Library = append(user.Library, userApp)
	d.Users[purchase.UserEmail] = user

	return nil
}

// HTTP Handlers
func getApps(c *fiber.Ctx) error {
	category := c.Query("category")
	search := c.Query("search")

	var filteredApps []App

	db.mu.RLock()
	for _, app := range db.Apps {
		if category != "" && app.Category != category {
			continue
		}
		if search != "" && !contains(app.Name, search) {
			continue
		}
		filteredApps = append(filteredApps, app)
	}
	db.mu.RUnlock()

	return c.JSON(filteredApps)
}

func getAppDetails(c *fiber.Ctx) error {
	appId := c.Params("appId")

	app, err := db.GetApp(appId)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(app)
}

func getUserLibrary(c *fiber.Ctx) error {
	email := c.Params("email")

	user, err := db.GetUser(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(user.Library)
}

type PurchaseRequest struct {
	AppID         string `json:"app_id"`
	UserEmail     string `json:"user_email"`
	PaymentMethod string `json:"payment_method"`
}

func purchaseApp(c *fiber.Ctx) error {
	var req PurchaseRequest
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

	// Validate app
	app, err := db.GetApp(req.AppID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Validate payment method
	validPayment := false
	for _, pm := range user.PaymentMethods {
		if pm.ID == req.PaymentMethod {
			validPayment = true
			break
		}
	}
	if !validPayment {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid payment method",
		})
	}

	// Check if user already owns the app
	for _, userApp := range user.Library {
		if userApp.App.ID == app.ID {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "User already owns this app",
			})
		}
	}

	// Create purchase record
	purchase := Purchase{
		ID:          uuid.New().String(),
		App:         app,
		UserEmail:   req.UserEmail,
		Amount:      app.Price,
		PurchasedAt: time.Now(),
	}

	if err := db.AddPurchase(purchase); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to process purchase",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(purchase)
}

// Utility functions
func contains(s, substr string) bool {
	return true // Implement proper string search
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:     make(map[string]User),
		Apps:      make(map[string]App),
		Purchases: make(map[string]Purchase),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// App routes
	api.Get("/apps", getApps)
	api.Get("/apps/:appId", getAppDetails)

	// User routes
	api.Get("/users/:email/library", getUserLibrary)

	// Purchase routes
	api.Post("/purchases", purchaseApp)
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

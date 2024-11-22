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
)

// Domain Models
type Account struct {
	AccountID      string        `json:"account_id"`
	CustomerName   string        `json:"customer_name"`
	Email          string        `json:"email"`
	PhoneNumbers   []PhoneLine   `json:"phone_numbers"`
	BillingAddress string        `json:"billing_address"`
	PaymentMethod  PaymentMethod `json:"payment_method"`
}

type PhoneLine struct {
	PhoneNumber string `json:"phone_number"`
	Plan        Plan   `json:"plan"`
	Device      Device `json:"device"`
}

type Plan struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	DataLimit    int      `json:"data_limit"`
	TalkMinutes  int      `json:"talk_minutes"`
	TextMessages int      `json:"text_messages"`
	Price        float64  `json:"price"`
	Features     []string `json:"features"`
}

type Device struct {
	ID           string `json:"id"`
	Model        string `json:"model"`
	Manufacturer string `json:"manufacturer"`
	IMEI         string `json:"imei"`
	Status       string `json:"status"`
}

type Usage struct {
	PhoneNumber     string    `json:"phone_number"`
	DataUsed        float64   `json:"data_used"`
	DataRemaining   float64   `json:"data_remaining"`
	MinutesUsed     int       `json:"minutes_used"`
	TextsSent       int       `json:"texts_sent"`
	BillingCycleEnd time.Time `json:"billing_cycle_end"`
}

type Bill struct {
	ID            string     `json:"id"`
	AccountID     string     `json:"account_id"`
	Amount        float64    `json:"amount"`
	DueDate       time.Time  `json:"due_date"`
	Status        string     `json:"status"`
	StatementDate time.Time  `json:"statement_date"`
	Items         []BillItem `json:"items"`
}

type BillItem struct {
	Description string  `json:"description"`
	Amount      float64 `json:"amount"`
	Type        string  `json:"type"`
}

type PaymentMethod struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Last4    string `json:"last4"`
	ExpiryMM int    `json:"expiry_mm"`
	ExpiryYY int    `json:"expiry_yy"`
}

// Database represents our in-memory database
type Database struct {
	Accounts map[string]Account `json:"accounts"`
	Usage    map[string]Usage   `json:"usage"`
	Bills    map[string][]Bill  `json:"bills"`
	Plans    []Plan             `json:"plans"`
	mu       sync.RWMutex
}

var db *Database

// Handlers
func getAccount(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	for _, account := range db.Accounts {
		if account.Email == email {
			return c.JSON(account)
		}
	}

	return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
		"error": "account not found",
	})
}

func getPlans(c *fiber.Ctx) error {
	db.mu.RLock()
	defer db.mu.RUnlock()

	return c.JSON(db.Plans)
}

func getUsage(c *fiber.Ctx) error {
	phoneNumber := c.Query("phone_number")
	if phoneNumber == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "phone_number parameter is required",
		})
	}

	db.mu.RLock()
	usage, exists := db.Usage[phoneNumber]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "usage data not found",
		})
	}

	return c.JSON(usage)
}

func getBills(c *fiber.Ctx) error {
	accountID := c.Query("account_id")
	if accountID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "account_id parameter is required",
		})
	}

	db.mu.RLock()
	bills, exists := db.Bills[accountID]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "bills not found",
		})
	}

	return c.JSON(bills)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Accounts: make(map[string]Account),
		Usage:    make(map[string]Usage),
		Bills:    make(map[string][]Bill),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Account routes
	api.Get("/account", getAccount)

	// Plans routes
	api.Get("/plans", getPlans)

	// Usage routes
	api.Get("/usage", getUsage)

	// Bills routes
	api.Get("/bills", getBills)
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

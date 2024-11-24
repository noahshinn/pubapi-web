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

// Data models
type Usage struct {
	DataUsed        float64   `json:"data_used"`
	DataLimit       float64   `json:"data_limit"`
	VoiceMinutes    int       `json:"voice_minutes"`
	TextMessages    int       `json:"text_messages"`
	BillingCycleEnd time.Time `json:"billing_cycle_end"`
}

type Bill struct {
	ID            string    `json:"id"`
	Amount        float64   `json:"amount"`
	DueDate       time.Time `json:"due_date"`
	Status        string    `json:"status"`
	StatementDate time.Time `json:"statement_date"`
}

type Plan struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	DataLimit float64  `json:"data_limit"`
	Price     float64  `json:"price"`
	Features  []string `json:"features"`
}

type Device struct {
	ID              string    `json:"id"`
	PhoneNumber     string    `json:"phone_number"`
	Model           string    `json:"model"`
	Status          string    `json:"status"`
	ContractEndDate time.Time `json:"contract_end_date"`
}

type Account struct {
	Email          string          `json:"email"`
	Name           string          `json:"name"`
	Address        string          `json:"address"`
	CurrentPlan    Plan            `json:"current_plan"`
	Devices        []Device        `json:"devices"`
	Usage          Usage           `json:"usage"`
	Bills          []Bill          `json:"bills"`
	PaymentMethods []PaymentMethod `json:"payment_methods"`
}

type PaymentMethod struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Last4    string `json:"last4"`
	ExpiryMM int    `json:"expiry_mm"`
	ExpiryYY int    `json:"expiry_yy"`
}

// Database struct
type Database struct {
	Accounts map[string]Account `json:"accounts"`
	Plans    []Plan             `json:"plans"`
	mu       sync.RWMutex
}

var db *Database

// Handler functions
func getAccountUsage(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email is required",
		})
	}

	db.mu.RLock()
	account, exists := db.Accounts[email]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Account not found",
		})
	}

	return c.JSON(account.Usage)
}

func getBillingHistory(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email is required",
		})
	}

	db.mu.RLock()
	account, exists := db.Accounts[email]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Account not found",
		})
	}

	return c.JSON(account.Bills)
}

func getAvailablePlans(c *fiber.Ctx) error {
	db.mu.RLock()
	plans := db.Plans
	db.mu.RUnlock()

	return c.JSON(plans)
}

func getDevices(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email is required",
		})
	}

	db.mu.RLock()
	account, exists := db.Accounts[email]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Account not found",
		})
	}

	return c.JSON(account.Devices)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Accounts: make(map[string]Account),
	}

	return json.Unmarshal(data, db)
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

	// Account routes
	api.Get("/account/usage", getAccountUsage)
	api.Get("/account/bills", getBillingHistory)
	api.Get("/account/devices", getDevices)

	// Plans route
	api.Get("/plans", getAvailablePlans)
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

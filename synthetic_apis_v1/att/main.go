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
type Usage struct {
	DataUsed        float64   `json:"data_used"`
	DataLimit       float64   `json:"data_limit"`
	VoiceMinutes    int       `json:"voice_minutes"`
	TextsSent       int       `json:"texts_sent"`
	BillingCycleEnd time.Time `json:"billing_cycle_end"`
}

type Bill struct {
	ID            string     `json:"id"`
	Amount        float64    `json:"amount"`
	DueDate       time.Time  `json:"due_date"`
	Status        string     `json:"status"`
	StatementDate time.Time  `json:"statement_date"`
	Items         []BillItem `json:"items"`
}

type BillItem struct {
	Description string  `json:"description"`
	Amount      float64 `json:"amount"`
}

type Plan struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	DataLimit float64  `json:"data_limit"`
	Price     float64  `json:"price"`
	Features  []string `json:"features"`
}

type Device struct {
	ID          string `json:"id"`
	PhoneNumber string `json:"phone_number"`
	Model       string `json:"model"`
	Status      string `json:"status"`
	PlanID      string `json:"plan_id"`
}

type Account struct {
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Devices   []Device  `json:"devices"`
	Usage     Usage     `json:"usage"`
	Bills     []Bill    `json:"bills"`
	CreatedAt time.Time `json:"created_at"`
}

// Database represents our in-memory database
type Database struct {
	Accounts map[string]Account `json:"accounts"`
	Plans    []Plan             `json:"plans"`
	mu       sync.RWMutex
}

var db *Database

// Database operations
func (d *Database) GetAccount(email string) (Account, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	account, exists := d.Accounts[email]
	if !exists {
		return Account{}, fiber.NewError(fiber.StatusNotFound, "Account not found")
	}
	return account, nil
}

func (d *Database) GetPlans() []Plan {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.Plans
}

// HTTP Handlers
func getAccountUsage(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email parameter is required",
		})
	}

	account, err := db.GetAccount(email)
	if err != nil {
		return err
	}

	return c.JSON(account.Usage)
}

func getBillingHistory(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email parameter is required",
		})
	}

	account, err := db.GetAccount(email)
	if err != nil {
		return err
	}

	return c.JSON(account.Bills)
}

func getPlans(c *fiber.Ctx) error {
	plans := db.GetPlans()
	return c.JSON(plans)
}

func getDevices(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email parameter is required",
		})
	}

	account, err := db.GetAccount(email)
	if err != nil {
		return err
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
	api := app.Group("/api/v1")

	// Account routes
	api.Get("/account/usage", getAccountUsage)
	api.Get("/account/bills", getBillingHistory)
	api.Get("/account/devices", getDevices)

	// Plans routes
	api.Get("/plans", getPlans)
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

	// Middleware
	app.Use(logger.New())
	app.Use(recover.New())
	app.Use(cors.New())

	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

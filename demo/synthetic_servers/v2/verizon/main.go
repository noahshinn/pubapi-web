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
	AccountID        string      `json:"account_id"`
	Owner            Owner       `json:"owner"`
	Plan             Plan        `json:"plan"`
	Lines            []PhoneLine `json:"lines"`
	AutoPay          bool        `json:"autopay"`
	PaperlessBilling bool        `json:"paperless_billing"`
}

type Owner struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

type PhoneLine struct {
	PhoneNumber string `json:"phone_number"`
	Device      Device `json:"device"`
	Plan        Plan   `json:"plan"`
	Status      string `json:"status"`
}

type Device struct {
	Model        string `json:"model"`
	Manufacturer string `json:"manufacturer"`
	IMEI         string `json:"imei"`
	Status       string `json:"status"`
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

type Usage struct {
	BillingCycle     BillingCycle `json:"billing_cycle"`
	DataUsed         float64      `json:"data_used"`
	DataRemaining    float64      `json:"data_remaining"`
	TalkMinutesUsed  int          `json:"talk_minutes_used"`
	TextMessagesSent int          `json:"text_messages_sent"`
}

type BillingCycle struct {
	StartDate time.Time `json:"start_date"`
	EndDate   time.Time `json:"end_date"`
}

type Bill struct {
	ID          string     `json:"id"`
	BillingDate time.Time  `json:"billing_date"`
	DueDate     time.Time  `json:"due_date"`
	Amount      float64    `json:"amount"`
	Status      string     `json:"status"`
	Items       []BillItem `json:"items"`
}

type BillItem struct {
	Description string  `json:"description"`
	Amount      float64 `json:"amount"`
}

// Database represents our in-memory database
type Database struct {
	Accounts map[string]Account `json:"accounts"`
	Usage    map[string]Usage   `json:"usage"`
	Bills    map[string][]Bill  `json:"bills"`
	Plans    map[string]Plan    `json:"plans"`
	mu       sync.RWMutex
}

var db *Database

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Accounts: make(map[string]Account),
		Usage:    make(map[string]Usage),
		Bills:    make(map[string][]Bill),
		Plans:    make(map[string]Plan),
	}

	return json.Unmarshal(data, db)
}

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
		if account.Owner.Email == email {
			return c.JSON(account)
		}
	}

	return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
		"error": "account not found",
	})
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

func getPlans(c *fiber.Ctx) error {
	db.mu.RLock()
	var plans []Plan
	for _, plan := range db.Plans {
		plans = append(plans, plan)
	}
	db.mu.RUnlock()

	return c.JSON(plans)
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
	api.Get("/account", getAccount)

	// Usage routes
	api.Get("/usage", getUsage)

	// Bills routes
	api.Get("/bills", getBills)

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

	app.Use(logger.New())
	app.Use(recover.New())
	app.Use(cors.New())

	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

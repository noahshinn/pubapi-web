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

// Data models
type InternetUsage struct {
	TotalUsageGB     float64   `json:"total_usage_gb"`
	DataCapGB        float64   `json:"data_cap_gb"`
	RemainingGB      float64   `json:"remaining_gb"`
	CurrentSpeedMbps float64   `json:"current_speed_mbps"`
	BillingCycleEnd  time.Time `json:"billing_cycle_end"`
}

type ServiceCharge struct {
	Name   string  `json:"name"`
	Amount float64 `json:"amount"`
}

type Bill struct {
	ID       string          `json:"id"`
	Amount   float64         `json:"amount"`
	DueDate  time.Time       `json:"due_date"`
	Status   string          `json:"status"`
	Services []ServiceCharge `json:"services"`
}

type Recording struct {
	ID              string    `json:"id"`
	ShowName        string    `json:"show_name"`
	Channel         string    `json:"channel"`
	StartTime       time.Time `json:"start_time"`
	DurationMinutes int       `json:"duration_minutes"`
	Status          string    `json:"status"`
	SeriesRecording bool      `json:"series_recording"`
}

type SupportTicket struct {
	ID          string    `json:"id"`
	Subject     string    `json:"subject"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	Priority    string    `json:"priority"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Account struct {
	ID             string          `json:"id"`
	Email          string          `json:"email"`
	Name           string          `json:"name"`
	Address        string          `json:"address"`
	Phone          string          `json:"phone"`
	InternetPlan   string          `json:"internet_plan"`
	TVPlan         string          `json:"tv_plan"`
	InternetUsage  InternetUsage   `json:"internet_usage"`
	Bills          []Bill          `json:"bills"`
	Recordings     []Recording     `json:"recordings"`
	SupportTickets []SupportTicket `json:"support_tickets"`
}

// Database
type Database struct {
	Accounts map[string]Account `json:"accounts"`
	mu       sync.RWMutex
}

var db *Database

// Handlers
func getInternetUsage(c *fiber.Ctx) error {
	accountID := c.Query("account_id")
	if accountID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "account_id is required",
		})
	}

	db.mu.RLock()
	account, exists := db.Accounts[accountID]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "account not found",
		})
	}

	return c.JSON(account.InternetUsage)
}

func getBillingHistory(c *fiber.Ctx) error {
	accountID := c.Query("account_id")
	if accountID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "account_id is required",
		})
	}

	db.mu.RLock()
	account, exists := db.Accounts[accountID]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "account not found",
		})
	}

	return c.JSON(account.Bills)
}

func getRecordings(c *fiber.Ctx) error {
	accountID := c.Query("account_id")
	if accountID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "account_id is required",
		})
	}

	db.mu.RLock()
	account, exists := db.Accounts[accountID]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "account not found",
		})
	}

	return c.JSON(account.Recordings)
}

func scheduleRecording(c *fiber.Ctx) error {
	accountID := c.Query("account_id")
	if accountID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "account_id is required",
		})
	}

	var newRecording Recording
	if err := c.BodyParser(&newRecording); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	newRecording.ID = uuid.New().String()
	newRecording.Status = "scheduled"

	db.mu.Lock()
	account, exists := db.Accounts[accountID]
	if !exists {
		db.mu.Unlock()
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "account not found",
		})
	}

	account.Recordings = append(account.Recordings, newRecording)
	db.Accounts[accountID] = account
	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(newRecording)
}

func getSupportTickets(c *fiber.Ctx) error {
	accountID := c.Query("account_id")
	if accountID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "account_id is required",
		})
	}

	db.mu.RLock()
	account, exists := db.Accounts[accountID]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "account not found",
		})
	}

	return c.JSON(account.SupportTickets)
}

func createSupportTicket(c *fiber.Ctx) error {
	accountID := c.Query("account_id")
	if accountID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "account_id is required",
		})
	}

	var newTicket SupportTicket
	if err := c.BodyParser(&newTicket); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	newTicket.ID = uuid.New().String()
	newTicket.Status = "open"
	newTicket.CreatedAt = time.Now()
	newTicket.UpdatedAt = time.Now()

	db.mu.Lock()
	account, exists := db.Accounts[accountID]
	if !exists {
		db.mu.Unlock()
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "account not found",
		})
	}

	account.SupportTickets = append(account.SupportTickets, newTicket)
	db.Accounts[accountID] = account
	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(newTicket)
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
	api.Get("/account/usage", getInternetUsage)
	api.Get("/account/bills", getBillingHistory)

	// TV routes
	api.Get("/tv/recordings", getRecordings)
	api.Post("/tv/recordings", scheduleRecording)

	// Support routes
	api.Get("/support/tickets", getSupportTickets)
	api.Post("/support/tickets", createSupportTicket)
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

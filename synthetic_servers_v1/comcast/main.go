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
type InternetPlan struct {
	Name        string  `json:"name"`
	Speed       string  `json:"speed"`
	DataLimit   int     `json:"data_limit"`
	MonthlyCost float64 `json:"monthly_cost"`
}

type TVPackage struct {
	Name        string   `json:"name"`
	Channels    int      `json:"channels"`
	Features    []string `json:"features"`
	MonthlyCost float64  `json:"monthly_cost"`
}

type User struct {
	Email          string    `json:"email"`
	Name           string    `json:"name"`
	Phone          string    `json:"phone"`
	Address        string    `json:"address"`
	AccountNumber  string    `json:"account_number"`
	InternetPlan   string    `json:"internet_plan"`
	TVPackage      string    `json:"tv_package"`
	AccountCreated time.Time `json:"account_created"`
	PaymentMethod  string    `json:"payment_method"`
	AutoPay        bool      `json:"auto_pay"`
}

type WatchlistItem struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Type      string    `json:"type"`
	AddedDate time.Time `json:"added_date"`
}

type UsagePeriod struct {
	StartDate time.Time `json:"start_date"`
	EndDate   time.Time `json:"end_date"`
	DataUsed  float64   `json:"data_used"`
	DataLimit float64   `json:"data_limit"`
}

type BillingRecord struct {
	ID       string    `json:"id"`
	Date     time.Time `json:"date"`
	Amount   float64   `json:"amount"`
	Status   string    `json:"status"`
	Services []Service `json:"services"`
}

type Service struct {
	Name string  `json:"name"`
	Cost float64 `json:"cost"`
}

// Database represents our in-memory database
type Database struct {
	Users          map[string]User            `json:"users"`
	InternetPlans  map[string]InternetPlan    `json:"internet_plans"`
	TVPackages     map[string]TVPackage       `json:"tv_packages"`
	Watchlists     map[string][]WatchlistItem `json:"watchlists"`
	UsageHistory   map[string][]UsagePeriod   `json:"usage_history"`
	BillingHistory map[string][]BillingRecord `json:"billing_history"`
	mu             sync.RWMutex
}

// Custom errors
var (
	ErrUserNotFound = errors.New("user not found")
	ErrInvalidPlan  = errors.New("invalid plan")
	ErrUnauthorized = errors.New("unauthorized")
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

func (d *Database) GetUserServices(email string) (map[string]interface{}, error) {
	user, err := d.GetUser(email)
	if err != nil {
		return nil, err
	}

	internetPlan := d.InternetPlans[user.InternetPlan]
	tvPackage := d.TVPackages[user.TVPackage]

	return map[string]interface{}{
		"internet":           internetPlan,
		"tv":                 tvPackage,
		"total_monthly_cost": internetPlan.MonthlyCost + tvPackage.MonthlyCost,
	}, nil
}

func (d *Database) GetUserUsage(email string) ([]UsagePeriod, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	usage, exists := d.UsageHistory[email]
	if !exists {
		return nil, ErrUserNotFound
	}
	return usage, nil
}

func (d *Database) GetWatchlist(email string) ([]WatchlistItem, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	watchlist, exists := d.Watchlists[email]
	if !exists {
		return nil, ErrUserNotFound
	}
	return watchlist, nil
}

func (d *Database) AddToWatchlist(email string, item WatchlistItem) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.Users[email]; !exists {
		return ErrUserNotFound
	}

	d.Watchlists[email] = append(d.Watchlists[email], item)
	return nil
}

func (d *Database) GetBillingHistory(email string) ([]BillingRecord, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	history, exists := d.BillingHistory[email]
	if !exists {
		return nil, ErrUserNotFound
	}
	return history, nil
}

// HTTP Handlers
func getServices(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	services, err := db.GetUserServices(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(services)
}

func getUsage(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	usage, err := db.GetUserUsage(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(usage)
}

func getWatchlist(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	watchlist, err := db.GetWatchlist(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(watchlist)
}

type AddWatchlistRequest struct {
	ContentID string `json:"content_id"`
	UserEmail string `json:"user_email"`
}

func addToWatchlist(c *fiber.Ctx) error {
	var req AddWatchlistRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	item := WatchlistItem{
		ID:        uuid.New().String(),
		Title:     "Sample Title", // In real implementation, would look up content details
		Type:      "show",
		AddedDate: time.Now(),
	}

	if err := db.AddToWatchlist(req.UserEmail, item); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(item)
}

func getBillingHistory(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	history, err := db.GetBillingHistory(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(history)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:          make(map[string]User),
		InternetPlans:  make(map[string]InternetPlan),
		TVPackages:     make(map[string]TVPackage),
		Watchlists:     make(map[string][]WatchlistItem),
		UsageHistory:   make(map[string][]UsagePeriod),
		BillingHistory: make(map[string][]BillingRecord),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Account routes
	account := api.Group("/account")
	account.Get("/services", getServices)
	account.Get("/usage", getUsage)

	// TV routes
	tv := api.Group("/tv")
	tv.Get("/watchlist", getWatchlist)
	tv.Post("/watchlist", addToWatchlist)

	// Billing routes
	billing := api.Group("/billing")
	billing.Get("/history", getBillingHistory)
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
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE",
		AllowHeaders: "Origin, Content-Type, Accept",
	}))

	// Setup routes
	setupRoutes(app)

	// Start server
	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

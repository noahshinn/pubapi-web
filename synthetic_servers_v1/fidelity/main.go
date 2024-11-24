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
type Position struct {
	Symbol          string  `json:"symbol"`
	Quantity        float64 `json:"quantity"`
	CurrentPrice    float64 `json:"current_price"`
	MarketValue     float64 `json:"market_value"`
	CostBasis       float64 `json:"cost_basis"`
	GainLoss        float64 `json:"gain_loss"`
	GainLossPercent float64 `json:"gain_loss_percent"`
}

type Account struct {
	ID        string     `json:"id"`
	Type      string     `json:"type"`
	Name      string     `json:"name"`
	Balance   float64    `json:"balance"`
	Positions []Position `json:"positions"`
}

type Portfolio struct {
	TotalValue           float64   `json:"total_value"`
	TotalGainLoss        float64   `json:"total_gain_loss"`
	TotalGainLossPercent float64   `json:"total_gain_loss_percent"`
	Accounts             []Account `json:"accounts"`
}

type OrderType string

const (
	Market OrderType = "MARKET"
	Limit  OrderType = "LIMIT"
	Stop   OrderType = "STOP"
)

type TimeInForce string

const (
	Day           TimeInForce = "DAY"
	GTC           TimeInForce = "GTC"
	ExtendedHours TimeInForce = "EXTENDED_HOURS"
)

type TradeOrder struct {
	ID          string      `json:"id"`
	AccountID   string      `json:"account_id"`
	Symbol      string      `json:"symbol"`
	OrderType   OrderType   `json:"order_type"`
	Quantity    float64     `json:"quantity"`
	Price       float64     `json:"price"`
	TimeInForce TimeInForce `json:"time_in_force"`
	Status      string      `json:"status"`
	CreatedAt   time.Time   `json:"created_at"`
}

type Transaction struct {
	ID        string    `json:"id"`
	AccountID string    `json:"account_id"`
	Type      string    `json:"type"`
	Symbol    string    `json:"symbol"`
	Quantity  float64   `json:"quantity"`
	Price     float64   `json:"price"`
	Amount    float64   `json:"amount"`
	Date      time.Time `json:"date"`
}

type User struct {
	Email    string    `json:"email"`
	Name     string    `json:"name"`
	Accounts []Account `json:"accounts"`
}

// Database represents our in-memory database
type Database struct {
	Users        map[string]User        `json:"users"`
	Transactions map[string]Transaction `json:"transactions"`
	TradeOrders  map[string]TradeOrder  `json:"trade_orders"`
	mu           sync.RWMutex
}

// Global database instance
var db *Database

// Custom errors
var (
	ErrUserNotFound      = errors.New("user not found")
	ErrAccountNotFound   = errors.New("account not found")
	ErrInvalidOrder      = errors.New("invalid order")
	ErrInsufficientFunds = errors.New("insufficient funds")
)

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

func (d *Database) GetAccount(accountID string) (Account, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	for _, user := range d.Users {
		for _, account := range user.Accounts {
			if account.ID == accountID {
				return account, nil
			}
		}
	}
	return Account{}, ErrAccountNotFound
}

func (d *Database) CreateTradeOrder(order TradeOrder) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.TradeOrders[order.ID] = order
	return nil
}

// HTTP Handlers
func getPortfolio(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	user, err := db.GetUser(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	var totalValue, totalGainLoss float64
	for _, account := range user.Accounts {
		for _, position := range account.Positions {
			totalValue += position.MarketValue
			totalGainLoss += position.GainLoss
		}
	}

	portfolio := Portfolio{
		TotalValue:           totalValue,
		TotalGainLoss:        totalGainLoss,
		TotalGainLossPercent: (totalGainLoss / (totalValue - totalGainLoss)) * 100,
		Accounts:             user.Accounts,
	}

	return c.JSON(portfolio)
}

func getAccounts(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	user, err := db.GetUser(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(user.Accounts)
}

func placeTrade(c *fiber.Ctx) error {
	var order TradeOrder
	if err := c.BodyParser(&order); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate account exists
	account, err := db.GetAccount(order.AccountID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Validate order
	if order.Quantity <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Quantity must be positive",
		})
	}

	// For market orders, validate sufficient funds
	if order.OrderType == Market {
		requiredFunds := order.Quantity * order.Price
		if account.Balance < requiredFunds {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": ErrInsufficientFunds.Error(),
			})
		}
	}

	// Generate order ID and set metadata
	order.ID = uuid.New().String()
	order.Status = "PENDING"
	order.CreatedAt = time.Now()

	// Save order
	if err := db.CreateTradeOrder(order); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create order",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(order)
}

func getTransactions(c *fiber.Ctx) error {
	accountID := c.Query("account_id")
	if accountID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "account_id parameter is required",
		})
	}

	// Validate account exists
	if _, err := db.GetAccount(accountID); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	var transactions []Transaction
	db.mu.RLock()
	for _, tx := range db.Transactions {
		if tx.AccountID == accountID {
			if startDate != "" {
				start, _ := time.Parse("2006-01-02", startDate)
				if tx.Date.Before(start) {
					continue
				}
			}
			if endDate != "" {
				end, _ := time.Parse("2006-01-02", endDate)
				if tx.Date.After(end) {
					continue
				}
			}
			transactions = append(transactions, tx)
		}
	}
	db.mu.RUnlock()

	return c.JSON(transactions)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:        make(map[string]User),
		Transactions: make(map[string]Transaction),
		TradeOrders:  make(map[string]TradeOrder),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	api.Get("/portfolio", getPortfolio)
	api.Get("/accounts", getAccounts)
	api.Post("/trades", placeTrade)
	api.Get("/transactions", getTransactions)
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

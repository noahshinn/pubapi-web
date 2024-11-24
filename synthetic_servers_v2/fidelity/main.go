package main

import (
	"encoding/json"
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
type AccountType string

const (
	Individual AccountType = "individual"
	IRA        AccountType = "ira"
	Roth       AccountType = "roth"
)

type Account struct {
	ID          string      `json:"id"`
	UserEmail   string      `json:"user_email"`
	Type        AccountType `json:"type"`
	Name        string      `json:"name"`
	Balance     float64     `json:"balance"`
	BuyingPower float64     `json:"buying_power"`
	CreatedAt   time.Time   `json:"created_at"`
}

type Position struct {
	AccountID       string    `json:"account_id"`
	Symbol          string    `json:"symbol"`
	Quantity        float64   `json:"quantity"`
	AveragePrice    float64   `json:"average_price"`
	CurrentPrice    float64   `json:"current_price"`
	MarketValue     float64   `json:"market_value"`
	UnrealizedPL    float64   `json:"unrealized_pl"`
	UnrealizedPLPct float64   `json:"unrealized_pl_percent"`
	LastUpdated     time.Time `json:"last_updated"`
}

type OrderSide string
type OrderType string
type OrderStatus string

const (
	Buy  OrderSide = "buy"
	Sell OrderSide = "sell"

	Market OrderType = "market"
	Limit  OrderType = "limit"

	Pending   OrderStatus = "pending"
	Executed  OrderStatus = "executed"
	Cancelled OrderStatus = "cancelled"
)

type Order struct {
	ID        string      `json:"id"`
	AccountID string      `json:"account_id"`
	Symbol    string      `json:"symbol"`
	Side      OrderSide   `json:"side"`
	Type      OrderType   `json:"type"`
	Quantity  float64     `json:"quantity"`
	Price     float64     `json:"price"`
	Status    OrderStatus `json:"status"`
	CreatedAt time.Time   `json:"created_at"`
}

type Quote struct {
	Symbol        string    `json:"symbol"`
	CompanyName   string    `json:"company_name"`
	LastPrice     float64   `json:"last_price"`
	Change        float64   `json:"change"`
	ChangePercent float64   `json:"change_percent"`
	Volume        int64     `json:"volume"`
	Bid           float64   `json:"bid"`
	Ask           float64   `json:"ask"`
	Timestamp     time.Time `json:"timestamp"`
}

// Database represents our in-memory database
type Database struct {
	Users     map[string]User     `json:"users"`
	Accounts  map[string]Account  `json:"accounts"`
	Positions map[string]Position `json:"positions"`
	Orders    map[string]Order    `json:"orders"`
	Quotes    map[string]Quote    `json:"quotes"`
	mu        sync.RWMutex
}

type User struct {
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

var db *Database

// Database operations
func (d *Database) GetUser(email string) (User, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	user, exists := d.Users[email]
	if !exists {
		return User{}, fmt.Errorf("user not found")
	}
	return user, nil
}

func (d *Database) GetUserAccounts(email string) []Account {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var accounts []Account
	for _, acc := range d.Accounts {
		if acc.UserEmail == email {
			accounts = append(accounts, acc)
		}
	}
	return accounts
}

func (d *Database) GetAccount(id string) (Account, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	acc, exists := d.Accounts[id]
	if !exists {
		return Account{}, fmt.Errorf("account not found")
	}
	return acc, nil
}

func (d *Database) GetPositions(accountId string) []Position {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var positions []Position
	for _, pos := range d.Positions {
		if pos.AccountID == accountId {
			positions = append(positions, pos)
		}
	}
	return positions
}

func (d *Database) GetOrders(accountId string) []Order {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var orders []Order
	for _, order := range d.Orders {
		if order.AccountID == accountId {
			orders = append(orders, order)
		}
	}
	return orders
}

func (d *Database) CreateOrder(order Order) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Orders[order.ID] = order
	return nil
}

func (d *Database) GetQuote(symbol string) (Quote, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	quote, exists := d.Quotes[symbol]
	if !exists {
		return Quote{}, fmt.Errorf("quote not found")
	}
	return quote, nil
}

// HTTP Handlers
func getAccounts(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	accounts := db.GetUserAccounts(email)
	return c.JSON(accounts)
}

func getPositions(c *fiber.Ctx) error {
	accountId := c.Params("accountId")
	if accountId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "account ID is required",
		})
	}

	positions := db.GetPositions(accountId)
	return c.JSON(positions)
}

func getOrders(c *fiber.Ctx) error {
	accountId := c.Params("accountId")
	if accountId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "account ID is required",
		})
	}

	orders := db.GetOrders(accountId)
	return c.JSON(orders)
}

type NewOrderRequest struct {
	Symbol   string    `json:"symbol"`
	Side     OrderSide `json:"side"`
	Type     OrderType `json:"type"`
	Quantity float64   `json:"quantity"`
	Price    float64   `json:"price"`
}

func placeOrder(c *fiber.Ctx) error {
	accountId := c.Params("accountId")
	if accountId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "account ID is required",
		})
	}

	var req NewOrderRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate account exists
	account, err := db.GetAccount(accountId)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Get current quote
	quote, err := db.GetQuote(req.Symbol)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Symbol not found",
		})
	}

	// Calculate order cost
	orderCost := req.Quantity * quote.LastPrice
	if req.Side == Buy && orderCost > account.BuyingPower {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Insufficient buying power",
		})
	}

	order := Order{
		ID:        uuid.New().String(),
		AccountID: accountId,
		Symbol:    req.Symbol,
		Side:      req.Side,
		Type:      req.Type,
		Quantity:  req.Quantity,
		Price:     req.Price,
		Status:    Pending,
		CreatedAt: time.Now(),
	}

	if err := db.CreateOrder(order); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create order",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(order)
}

func getQuote(c *fiber.Ctx) error {
	symbol := c.Params("symbol")
	if symbol == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "symbol is required",
		})
	}

	quote, err := db.GetQuote(symbol)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(quote)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:     make(map[string]User),
		Accounts:  make(map[string]Account),
		Positions: make(map[string]Position),
		Orders:    make(map[string]Order),
		Quotes:    make(map[string]Quote),
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
	api.Get("/accounts", getAccounts)
	api.Get("/accounts/:accountId/positions", getPositions)
	api.Get("/accounts/:accountId/orders", getOrders)
	api.Post("/accounts/:accountId/orders", placeOrder)

	// Quote routes
	api.Get("/quotes/:symbol", getQuote)
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

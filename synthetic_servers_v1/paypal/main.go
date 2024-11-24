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
type Balance struct {
	Available float64 `json:"available"`
	Pending   float64 `json:"pending"`
	Currency  string  `json:"currency"`
}

type TransactionType string
type TransactionStatus string

const (
	TransactionTypePayment  TransactionType = "payment"
	TransactionTypeRefund   TransactionType = "refund"
	TransactionTypeTransfer TransactionType = "transfer"

	TransactionStatusCompleted TransactionStatus = "completed"
	TransactionStatusPending   TransactionStatus = "pending"
	TransactionStatusFailed    TransactionStatus = "failed"
)

type Transaction struct {
	ID          string            `json:"id"`
	Type        TransactionType   `json:"type"`
	Status      TransactionStatus `json:"status"`
	Amount      float64           `json:"amount"`
	Currency    string            `json:"currency"`
	Sender      string            `json:"sender"`
	Recipient   string            `json:"recipient"`
	Description string            `json:"description"`
	CreatedAt   time.Time         `json:"created_at"`
}

type PaymentMethodType string

const (
	PaymentMethodBank       PaymentMethodType = "bank_account"
	PaymentMethodCreditCard PaymentMethodType = "credit_card"
	PaymentMethodDebitCard  PaymentMethodType = "debit_card"
)

type PaymentMethod struct {
	ID        string            `json:"id"`
	Type      PaymentMethodType `json:"type"`
	Last4     string            `json:"last4"`
	BankName  string            `json:"bank_name,omitempty"`
	IsDefault bool              `json:"is_default"`
	CreatedAt time.Time         `json:"created_at"`
}

type User struct {
	Email          string          `json:"email"`
	Name           string          `json:"name"`
	Balance        Balance         `json:"balance"`
	PaymentMethods []PaymentMethod `json:"payment_methods"`
}

// Database represents our in-memory database
type Database struct {
	Users        map[string]User        `json:"users"`
	Transactions map[string]Transaction `json:"transactions"`
	mu           sync.RWMutex
}

// Global database instance
var db *Database

// Custom errors
var (
	ErrUserNotFound         = errors.New("user not found")
	ErrInsufficientFunds    = errors.New("insufficient funds")
	ErrInvalidPaymentMethod = errors.New("invalid payment method")
	ErrInvalidAmount        = errors.New("invalid amount")
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

func (d *Database) UpdateUserBalance(email string, amount float64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	user, exists := d.Users[email]
	if !exists {
		return ErrUserNotFound
	}

	if user.Balance.Available+amount < 0 {
		return ErrInsufficientFunds
	}

	user.Balance.Available += amount
	d.Users[email] = user
	return nil
}

func (d *Database) CreateTransaction(tx Transaction) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Transactions[tx.ID] = tx
	return nil
}

// HTTP Handlers
func getBalance(c *fiber.Ctx) error {
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

	return c.JSON(user.Balance)
}

func getTransactions(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	var transactions []Transaction
	db.mu.RLock()
	for _, tx := range db.Transactions {
		if tx.Sender == email || tx.Recipient == email {
			if startDate != "" && tx.CreatedAt.Format("2006-01-02") < startDate {
				continue
			}
			if endDate != "" && tx.CreatedAt.Format("2006-01-02") > endDate {
				continue
			}
			transactions = append(transactions, tx)
		}
	}
	db.mu.RUnlock()

	return c.JSON(transactions)
}

type PaymentRequest struct {
	SenderEmail     string  `json:"sender_email"`
	RecipientEmail  string  `json:"recipient_email"`
	Amount          float64 `json:"amount"`
	Currency        string  `json:"currency"`
	Description     string  `json:"description"`
	PaymentMethodID string  `json:"payment_method_id"`
}

func processPayment(c *fiber.Ctx) error {
	var req PaymentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.Amount <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Amount must be positive",
		})
	}

	// Verify sender
	sender, err := db.GetUser(req.SenderEmail)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Sender not found",
		})
	}

	// Verify recipient
	_, err = db.GetUser(req.RecipientEmail)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Recipient not found",
		})
	}

	// Verify payment method
	validPayment := false
	for _, pm := range sender.PaymentMethods {
		if pm.ID == req.PaymentMethodID {
			validPayment = true
			break
		}
	}
	if !validPayment {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid payment method",
		})
	}

	// Create transaction
	tx := Transaction{
		ID:          uuid.New().String(),
		Type:        TransactionTypePayment,
		Status:      TransactionStatusPending,
		Amount:      req.Amount,
		Currency:    req.Currency,
		Sender:      req.SenderEmail,
		Recipient:   req.RecipientEmail,
		Description: req.Description,
		CreatedAt:   time.Now(),
	}

	// Update balances
	if err := db.UpdateUserBalance(req.SenderEmail, -req.Amount); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if err := db.UpdateUserBalance(req.RecipientEmail, req.Amount); err != nil {
		// Rollback sender's balance
		_ = db.UpdateUserBalance(req.SenderEmail, req.Amount)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to process payment",
		})
	}

	tx.Status = TransactionStatusCompleted
	if err := db.CreateTransaction(tx); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to record transaction",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(tx)
}

func getPaymentMethods(c *fiber.Ctx) error {
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

	return c.JSON(user.PaymentMethods)
}

type NewPaymentMethod struct {
	Type          PaymentMethodType `json:"type"`
	AccountNumber string            `json:"account_number"`
	RoutingNumber string            `json:"routing_number"`
	CardNumber    string            `json:"card_number"`
	ExpiryMonth   int               `json:"expiry_month"`
	ExpiryYear    int               `json:"expiry_year"`
	CVV           string            `json:"cvv"`
}

func addPaymentMethod(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	var req NewPaymentMethod
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	user, exists := db.Users[email]
	if !exists {
		db.mu.Unlock()
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	var last4 string
	switch req.Type {
	case PaymentMethodBank:
		last4 = req.AccountNumber[len(req.AccountNumber)-4:]
	case PaymentMethodCreditCard, PaymentMethodDebitCard:
		last4 = req.CardNumber[len(req.CardNumber)-4:]
	default:
		db.mu.Unlock()
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid payment method type",
		})
	}

	pm := PaymentMethod{
		ID:        uuid.New().String(),
		Type:      req.Type,
		Last4:     last4,
		IsDefault: len(user.PaymentMethods) == 0,
		CreatedAt: time.Now(),
	}

	user.PaymentMethods = append(user.PaymentMethods, pm)
	db.Users[email] = user
	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(pm)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:        make(map[string]User),
		Transactions: make(map[string]Transaction),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	api.Get("/balance", getBalance)
	api.Get("/transactions", getTransactions)
	api.Post("/transactions", processPayment)
	api.Get("/payment-methods", getPaymentMethods)
	api.Post("/payment-methods", addPaymentMethod)
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

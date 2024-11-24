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
	"github.com/google/uuid"
)

// Models
type Balance struct {
	Available float64 `json:"available"`
	Pending   float64 `json:"pending"`
	Currency  string  `json:"currency"`
}

type Transaction struct {
	ID             string    `json:"id"`
	Type           string    `json:"type"`
	Status         string    `json:"status"`
	Amount         float64   `json:"amount"`
	Currency       string    `json:"currency"`
	SenderEmail    string    `json:"sender_email"`
	RecipientEmail string    `json:"recipient_email"`
	Description    string    `json:"description"`
	CreatedAt      time.Time `json:"created_at"`
}

type PaymentMethod struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Last4       string `json:"last4"`
	BankName    string `json:"bank_name,omitempty"`
	CardType    string `json:"card_type,omitempty"`
	ExpiryMonth int    `json:"expiry_month,omitempty"`
	ExpiryYear  int    `json:"expiry_year,omitempty"`
	IsDefault   bool   `json:"is_default"`
}

type User struct {
	Email          string          `json:"email"`
	Name           string          `json:"name"`
	Balance        Balance         `json:"balance"`
	PaymentMethods []PaymentMethod `json:"payment_methods"`
	CreatedAt      time.Time       `json:"created_at"`
	LastActivityAt time.Time       `json:"last_activity_at"`
}

// Database
type Database struct {
	Users        map[string]User        `json:"users"`
	Transactions map[string]Transaction `json:"transactions"`
	mu           sync.RWMutex
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

func (d *Database) UpdateUser(user User) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Users[user.Email] = user
	return nil
}

func (d *Database) AddTransaction(tx Transaction) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Transactions[tx.ID] = tx
	return nil
}

func (d *Database) GetTransactions(email string, limit, offset int) []Transaction {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var userTransactions []Transaction
	for _, tx := range d.Transactions {
		if tx.SenderEmail == email || tx.RecipientEmail == email {
			userTransactions = append(userTransactions, tx)
		}
	}

	// Apply pagination
	start := offset
	end := offset + limit
	if start >= len(userTransactions) {
		return []Transaction{}
	}
	if end > len(userTransactions) {
		end = len(userTransactions)
	}

	return userTransactions[start:end]
}

// Handlers
func getBalance(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
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
			"error": "email is required",
		})
	}

	limit := c.QueryInt("limit", 10)
	offset := c.QueryInt("offset", 0)

	transactions := db.GetTransactions(email, limit, offset)
	return c.JSON(transactions)
}

type SendMoneyRequest struct {
	SenderEmail     string  `json:"sender_email"`
	RecipientEmail  string  `json:"recipient_email"`
	Amount          float64 `json:"amount"`
	Currency        string  `json:"currency"`
	Description     string  `json:"description"`
	PaymentMethodID string  `json:"payment_method_id"`
}

func sendMoney(c *fiber.Ctx) error {
	var req SendMoneyRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate request
	if req.Amount <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Amount must be positive",
		})
	}

	// Get sender
	sender, err := db.GetUser(req.SenderEmail)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Sender not found",
		})
	}

	// Get recipient
	recipient, err := db.GetUser(req.RecipientEmail)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Recipient not found",
		})
	}

	// Check balance
	if sender.Balance.Available < req.Amount {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Insufficient funds",
		})
	}

	// Create transaction
	tx := Transaction{
		ID:             uuid.New().String(),
		Type:           "payment",
		Status:         "completed",
		Amount:         req.Amount,
		Currency:       req.Currency,
		SenderEmail:    req.SenderEmail,
		RecipientEmail: req.RecipientEmail,
		Description:    req.Description,
		CreatedAt:      time.Now(),
	}

	// Update balances
	sender.Balance.Available -= req.Amount
	recipient.Balance.Available += req.Amount

	// Save changes
	if err := db.UpdateUser(sender); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update sender balance",
		})
	}

	if err := db.UpdateUser(recipient); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update recipient balance",
		})
	}

	if err := db.AddTransaction(tx); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to save transaction",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(tx)
}

func getPaymentMethods(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
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
	Type          string `json:"type"`
	AccountNumber string `json:"account_number"`
	RoutingNumber string `json:"routing_number"`
	CardNumber    string `json:"card_number"`
	ExpiryMonth   int    `json:"expiry_month"`
	ExpiryYear    int    `json:"expiry_year"`
	CVV           string `json:"cvv"`
}

func addPaymentMethod(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	var req NewPaymentMethod
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	user, err := db.GetUser(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Create new payment method
	pm := PaymentMethod{
		ID:        uuid.New().String(),
		Type:      req.Type,
		IsDefault: len(user.PaymentMethods) == 0,
	}

	// Set type-specific fields
	switch req.Type {
	case "bank_account":
		pm.Last4 = req.AccountNumber[len(req.AccountNumber)-4:]
		pm.BankName = "Bank Name" // In real implementation, would look up bank name from routing number
	case "credit_card", "debit_card":
		pm.Last4 = req.CardNumber[len(req.CardNumber)-4:]
		pm.ExpiryMonth = req.ExpiryMonth
		pm.ExpiryYear = req.ExpiryYear
		pm.CardType = getCardType(req.CardNumber) // Helper function to determine card type
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid payment method type",
		})
	}

	// Add payment method to user
	user.PaymentMethods = append(user.PaymentMethods, pm)
	if err := db.UpdateUser(user); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to save payment method",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(pm)
}

func getCardType(cardNumber string) string {
	// Simplified card type detection
	switch cardNumber[0] {
	case '4':
		return "Visa"
	case '5':
		return "MasterCard"
	case '3':
		return "American Express"
	default:
		return "Unknown"
	}
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

	api.Get("/balance", getBalance)
	api.Get("/transactions", getTransactions)
	api.Post("/transactions", sendMoney)
	api.Get("/payment-methods", getPaymentMethods)
	api.Post("/payment-methods", addPaymentMethod)
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

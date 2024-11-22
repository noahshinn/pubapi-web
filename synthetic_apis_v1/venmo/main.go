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
type User struct {
	Email          string    `json:"email"`
	Name           string    `json:"name"`
	Username       string    `json:"username"`
	ProfilePicture string    `json:"profile_picture"`
	Balance        float64   `json:"balance"`
	PendingBalance float64   `json:"pending_balance"`
	Friends        []string  `json:"friends"` // List of friend emails
	CreatedAt      time.Time `json:"created_at"`
}

type TransactionStatus string

const (
	TransactionStatusPending  TransactionStatus = "pending"
	TransactionStatusComplete TransactionStatus = "complete"
	TransactionStatusFailed   TransactionStatus = "failed"
)

type TransactionVisibility string

const (
	TransactionVisibilityPublic  TransactionVisibility = "public"
	TransactionVisibilityPrivate TransactionVisibility = "private"
	TransactionVisibilityFriends TransactionVisibility = "friends"
)

type Transaction struct {
	ID         string                `json:"id"`
	Sender     User                  `json:"sender"`
	Recipient  User                  `json:"recipient"`
	Amount     float64               `json:"amount"`
	Note       string                `json:"note"`
	Visibility TransactionVisibility `json:"visibility"`
	Status     TransactionStatus     `json:"status"`
	CreatedAt  time.Time             `json:"created_at"`
}

// Database represents our in-memory database
type Database struct {
	Users        map[string]User        `json:"users"`
	Transactions map[string]Transaction `json:"transactions"`
	mu           sync.RWMutex
}

// Custom errors
var (
	ErrUserNotFound        = errors.New("user not found")
	ErrInsufficientFunds   = errors.New("insufficient funds")
	ErrInvalidAmount       = errors.New("invalid amount")
	ErrTransactionNotFound = errors.New("transaction not found")
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

func (d *Database) UpdateUserBalance(email string, amount float64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	user, exists := d.Users[email]
	if !exists {
		return ErrUserNotFound
	}

	user.Balance += amount
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
func getUserTransactions(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	limit := c.QueryInt("limit", 20)

	var userTransactions []Transaction
	db.mu.RLock()
	for _, tx := range db.Transactions {
		if tx.Sender.Email == email || tx.Recipient.Email == email {
			userTransactions = append(userTransactions, tx)
		}
		if len(userTransactions) >= limit {
			break
		}
	}
	db.mu.RUnlock()

	return c.JSON(userTransactions)
}

type CreateTransactionRequest struct {
	SenderEmail    string                `json:"sender_email"`
	RecipientEmail string                `json:"recipient_email"`
	Amount         float64               `json:"amount"`
	Note           string                `json:"note"`
	Visibility     TransactionVisibility `json:"visibility"`
}

func createTransaction(c *fiber.Ctx) error {
	var req CreateTransactionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.Amount <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": ErrInvalidAmount.Error(),
		})
	}

	// Get sender and recipient
	sender, err := db.GetUser(req.SenderEmail)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Sender " + err.Error(),
		})
	}

	recipient, err := db.GetUser(req.RecipientEmail)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Recipient " + err.Error(),
		})
	}

	// Check sender has sufficient funds
	if sender.Balance < req.Amount {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": ErrInsufficientFunds.Error(),
		})
	}

	// Create transaction
	tx := Transaction{
		ID:         uuid.New().String(),
		Sender:     sender,
		Recipient:  recipient,
		Amount:     req.Amount,
		Note:       req.Note,
		Visibility: req.Visibility,
		Status:     TransactionStatusComplete,
		CreatedAt:  time.Now(),
	}

	// Update balances
	if err := db.UpdateUserBalance(req.SenderEmail, -req.Amount); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update sender balance",
		})
	}

	if err := db.UpdateUserBalance(req.RecipientEmail, req.Amount); err != nil {
		// Rollback sender's balance
		db.UpdateUserBalance(req.SenderEmail, req.Amount)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update recipient balance",
		})
	}

	// Save transaction
	if err := db.CreateTransaction(tx); err != nil {
		// Rollback balances
		db.UpdateUserBalance(req.SenderEmail, req.Amount)
		db.UpdateUserBalance(req.RecipientEmail, -req.Amount)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create transaction",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(tx)
}

func getUserBalance(c *fiber.Ctx) error {
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

	return c.JSON(fiber.Map{
		"available": user.Balance,
		"pending":   user.PendingBalance,
	})
}

func getUserFriends(c *fiber.Ctx) error {
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

	var friends []User
	db.mu.RLock()
	for _, friendEmail := range user.Friends {
		if friend, exists := db.Users[friendEmail]; exists {
			friends = append(friends, friend)
		}
	}
	db.mu.RUnlock()

	return c.JSON(friends)
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

	// Transaction routes
	api.Get("/transactions", getUserTransactions)
	api.Post("/transactions", createTransaction)

	// Balance routes
	api.Get("/balance", getUserBalance)

	// Friend routes
	api.Get("/friends", getUserFriends)
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

	// Setup routes
	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

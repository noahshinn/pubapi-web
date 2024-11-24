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
type AccountType string
type TransactionType string
type TransactionStatus string

const (
	AccountTypeChecking AccountType = "CHECKING"
	AccountTypeSavings  AccountType = "SAVINGS"
	AccountTypeCredit   AccountType = "CREDIT"

	TransactionTypeDebit  TransactionType = "DEBIT"
	TransactionTypeCredit TransactionType = "CREDIT"

	TransactionStatusPending   TransactionStatus = "PENDING"
	TransactionStatusCompleted TransactionStatus = "COMPLETED"
	TransactionStatusFailed    TransactionStatus = "FAILED"
)

type Account struct {
	ID        string      `json:"id"`
	UserEmail string      `json:"user_email"`
	Type      AccountType `json:"type"`
	Name      string      `json:"name"`
	Balance   float64     `json:"balance"`
	Currency  string      `json:"currency"`
	Last4     string      `json:"last4"`
	Status    string      `json:"status"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

type Transaction struct {
	ID          string            `json:"id"`
	AccountID   string            `json:"account_id"`
	Date        time.Time         `json:"date"`
	Description string            `json:"description"`
	Amount      float64           `json:"amount"`
	Type        TransactionType   `json:"type"`
	Category    string            `json:"category"`
	Status      TransactionStatus `json:"status"`
	Reference   string            `json:"reference"`
}

type Transfer struct {
	ID          string            `json:"id"`
	FromAccount string            `json:"from_account"`
	ToAccount   string            `json:"to_account"`
	Amount      float64           `json:"amount"`
	Description string            `json:"description"`
	Status      TransactionStatus `json:"status"`
	CreatedAt   time.Time         `json:"created_at"`
}

type Bill struct {
	ID        string    `json:"id"`
	UserEmail string    `json:"user_email"`
	Payee     string    `json:"payee"`
	Amount    float64   `json:"amount"`
	DueDate   time.Time `json:"due_date"`
	Status    string    `json:"status"`
	Autopay   bool      `json:"autopay"`
}

// Database represents our in-memory database
type Database struct {
	Accounts     map[string]Account     `json:"accounts"`
	Transactions map[string]Transaction `json:"transactions"`
	Transfers    map[string]Transfer    `json:"transfers"`
	Bills        map[string]Bill        `json:"bills"`
	mu           sync.RWMutex
}

var (
	ErrAccountNotFound   = errors.New("account not found")
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrInvalidAmount     = errors.New("invalid amount")
	ErrUnauthorized      = errors.New("unauthorized")
)

var db *Database

// Database operations
func (d *Database) GetAccount(id string) (Account, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	account, exists := d.Accounts[id]
	if !exists {
		return Account{}, ErrAccountNotFound
	}
	return account, nil
}

func (d *Database) GetUserAccounts(email string) []Account {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var accounts []Account
	for _, account := range d.Accounts {
		if account.UserEmail == email {
			accounts = append(accounts, account)
		}
	}
	return accounts
}

func (d *Database) GetAccountTransactions(accountId string, startDate, endDate time.Time) []Transaction {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var transactions []Transaction
	for _, tx := range d.Transactions {
		if tx.AccountID == accountId {
			if (startDate.IsZero() || !tx.Date.Before(startDate)) &&
				(endDate.IsZero() || !tx.Date.After(endDate)) {
				transactions = append(transactions, tx)
			}
		}
	}
	return transactions
}

func (d *Database) CreateTransfer(transfer Transfer) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Validate accounts
	fromAccount, exists := d.Accounts[transfer.FromAccount]
	if !exists {
		return ErrAccountNotFound
	}

	toAccount, exists := d.Accounts[transfer.ToAccount]
	if !exists {
		return ErrAccountNotFound
	}

	// Check sufficient funds
	if fromAccount.Balance < transfer.Amount {
		return ErrInsufficientFunds
	}

	// Update account balances
	fromAccount.Balance -= transfer.Amount
	toAccount.Balance += transfer.Amount

	// Save updated accounts
	d.Accounts[transfer.FromAccount] = fromAccount
	d.Accounts[transfer.ToAccount] = toAccount

	// Create transactions
	txId1 := uuid.New().String()
	txId2 := uuid.New().String()

	d.Transactions[txId1] = Transaction{
		ID:          txId1,
		AccountID:   transfer.FromAccount,
		Date:        transfer.CreatedAt,
		Description: transfer.Description,
		Amount:      -transfer.Amount,
		Type:        TransactionTypeDebit,
		Status:      TransactionStatusCompleted,
		Reference:   transfer.ID,
	}

	d.Transactions[txId2] = Transaction{
		ID:          txId2,
		AccountID:   transfer.ToAccount,
		Date:        transfer.CreatedAt,
		Description: transfer.Description,
		Amount:      transfer.Amount,
		Type:        TransactionTypeCredit,
		Status:      TransactionStatusCompleted,
		Reference:   transfer.ID,
	}

	// Save transfer
	d.Transfers[transfer.ID] = transfer

	return nil
}

func (d *Database) GetUserBills(email string) []Bill {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var bills []Bill
	for _, bill := range d.Bills {
		if bill.UserEmail == email {
			bills = append(bills, bill)
		}
	}
	return bills
}

// HTTP Handlers
func getUserAccounts(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	accounts := db.GetUserAccounts(email)
	return c.JSON(accounts)
}

func getAccountTransactions(c *fiber.Ctx) error {
	accountId := c.Params("accountId")
	if accountId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "account ID is required",
		})
	}

	startDateStr := c.Query("startDate")
	endDateStr := c.Query("endDate")

	var startDate, endDate time.Time
	var err error

	if startDateStr != "" {
		startDate, err = time.Parse("2006-01-02", startDateStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "invalid start date format",
			})
		}
	}

	if endDateStr != "" {
		endDate, err = time.Parse("2006-01-02", endDateStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "invalid end date format",
			})
		}
	}

	transactions := db.GetAccountTransactions(accountId, startDate, endDate)
	return c.JSON(transactions)
}

type TransferRequest struct {
	FromAccount string  `json:"from_account"`
	ToAccount   string  `json:"to_account"`
	Amount      float64 `json:"amount"`
	Description string  `json:"description"`
}

func createTransfer(c *fiber.Ctx) error {
	var req TransferRequest
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

	transfer := Transfer{
		ID:          uuid.New().String(),
		FromAccount: req.FromAccount,
		ToAccount:   req.ToAccount,
		Amount:      req.Amount,
		Description: req.Description,
		Status:      TransactionStatusCompleted,
		CreatedAt:   time.Now(),
	}

	if err := db.CreateTransfer(transfer); err != nil {
		switch err {
		case ErrAccountNotFound:
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": err.Error(),
			})
		case ErrInsufficientFunds:
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": err.Error(),
			})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to process transfer",
			})
		}
	}

	return c.Status(fiber.StatusCreated).JSON(transfer)
}

func getUserBills(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	bills := db.GetUserBills(email)
	return c.JSON(bills)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Accounts:     make(map[string]Account),
		Transactions: make(map[string]Transaction),
		Transfers:    make(map[string]Transfer),
		Bills:        make(map[string]Bill),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Account routes
	api.Get("/accounts", getUserAccounts)
	api.Get("/accounts/:accountId", func(c *fiber.Ctx) error {
		accountId := c.Params("accountId")
		account, err := db.GetAccount(accountId)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.JSON(account)
	})
	api.Get("/accounts/:accountId/transactions", getAccountTransactions)

	// Transfer routes
	api.Post("/transfers", createTransfer)

	// Bill routes
	api.Get("/bills", getUserBills)
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

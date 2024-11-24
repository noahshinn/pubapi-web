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

const (
	AccountTypeChecking AccountType = "CHECKING"
	AccountTypeSavings  AccountType = "SAVINGS"
	AccountTypeCredit   AccountType = "CREDIT"
)

type AccountStatus string

const (
	AccountStatusActive   AccountStatus = "ACTIVE"
	AccountStatusInactive AccountStatus = "INACTIVE"
	AccountStatusFrozen   AccountStatus = "FROZEN"
)

type TransactionType string

const (
	TransactionTypeDebit  TransactionType = "DEBIT"
	TransactionTypeCredit TransactionType = "CREDIT"
)

type TransactionStatus string

const (
	TransactionStatusPending   TransactionStatus = "PENDING"
	TransactionStatusCompleted TransactionStatus = "COMPLETED"
	TransactionStatusFailed    TransactionStatus = "FAILED"
)

type Account struct {
	ID          string        `json:"id"`
	UserEmail   string        `json:"user_email"`
	Type        AccountType   `json:"type"`
	Name        string        `json:"name"`
	Balance     float64       `json:"balance"`
	Currency    string        `json:"currency"`
	Status      AccountStatus `json:"status"`
	CreatedAt   time.Time     `json:"created_at"`
	LastUpdated time.Time     `json:"last_updated"`
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
	ID            string            `json:"id"`
	FromAccountID string            `json:"from_account_id"`
	ToAccountID   string            `json:"to_account_id"`
	Amount        float64           `json:"amount"`
	Description   string            `json:"description"`
	Status        TransactionStatus `json:"status"`
	CreatedAt     time.Time         `json:"created_at"`
}

type Bill struct {
	ID        string    `json:"id"`
	UserEmail string    `json:"user_email"`
	Payee     string    `json:"payee"`
	Amount    float64   `json:"amount"`
	DueDate   time.Time `json:"due_date"`
	Status    string    `json:"status"`
	AutoPay   bool      `json:"autopay"`
	AccountID string    `json:"account_id"`
}

// Database represents our in-memory database
type Database struct {
	Accounts     map[string]Account     `json:"accounts"`
	Transactions map[string]Transaction `json:"transactions"`
	Transfers    map[string]Transfer    `json:"transfers"`
	Bills        map[string]Bill        `json:"bills"`
	mu           sync.RWMutex
}

// Custom errors
var (
	ErrAccountNotFound   = errors.New("account not found")
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrInvalidAmount     = errors.New("invalid amount")
	ErrInvalidTransfer   = errors.New("invalid transfer")
)

// Global database instance
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

func (d *Database) GetAccountTransactions(accountID string) []Transaction {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var transactions []Transaction
	for _, tx := range d.Transactions {
		if tx.AccountID == accountID {
			transactions = append(transactions, tx)
		}
	}
	return transactions
}

func (d *Database) CreateTransfer(transfer Transfer) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Validate accounts exist
	fromAccount, exists := d.Accounts[transfer.FromAccountID]
	if !exists {
		return ErrAccountNotFound
	}

	toAccount, exists := d.Accounts[transfer.ToAccountID]
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

	// Update accounts
	d.Accounts[fromAccount.ID] = fromAccount
	d.Accounts[toAccount.ID] = toAccount

	// Create transactions
	debitTx := Transaction{
		ID:          uuid.New().String(),
		AccountID:   fromAccount.ID,
		Date:        time.Now(),
		Description: transfer.Description,
		Amount:      -transfer.Amount,
		Type:        TransactionTypeDebit,
		Status:      TransactionStatusCompleted,
		Reference:   transfer.ID,
	}

	creditTx := Transaction{
		ID:          uuid.New().String(),
		AccountID:   toAccount.ID,
		Date:        time.Now(),
		Description: transfer.Description,
		Amount:      transfer.Amount,
		Type:        TransactionTypeCredit,
		Status:      TransactionStatusCompleted,
		Reference:   transfer.ID,
	}

	// Save transactions
	d.Transactions[debitTx.ID] = debitTx
	d.Transactions[creditTx.ID] = creditTx

	// Save transfer
	d.Transfers[transfer.ID] = transfer

	return nil
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
	accountID := c.Params("accountId")
	if accountID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "account ID is required",
		})
	}

	// Verify account exists
	if _, err := db.GetAccount(accountID); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	transactions := db.GetAccountTransactions(accountID)
	return c.JSON(transactions)
}

type TransferRequest struct {
	FromAccountID string  `json:"from_account_id"`
	ToAccountID   string  `json:"to_account_id"`
	Amount        float64 `json:"amount"`
	Description   string  `json:"description"`
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
		ID:            uuid.New().String(),
		FromAccountID: req.FromAccountID,
		ToAccountID:   req.ToAccountID,
		Amount:        req.Amount,
		Description:   req.Description,
		Status:        TransactionStatusCompleted,
		CreatedAt:     time.Now(),
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

	db.mu.RLock()
	var bills []Bill
	for _, bill := range db.Bills {
		if bill.UserEmail == email {
			bills = append(bills, bill)
		}
	}
	db.mu.RUnlock()

	return c.JSON(bills)
}

type BillPaymentRequest struct {
	BillID        string    `json:"bill_id"`
	AccountID     string    `json:"account_id"`
	Amount        float64   `json:"amount"`
	ScheduledDate time.Time `json:"scheduled_date"`
}

func payBill(c *fiber.Ctx) error {
	var req BillPaymentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	// Verify bill exists
	bill, exists := db.Bills[req.BillID]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Bill not found",
		})
	}

	// Verify account exists and has sufficient funds
	account, exists := db.Accounts[req.AccountID]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Account not found",
		})
	}

	if account.Balance < req.Amount {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Insufficient funds",
		})
	}

	// Process payment
	account.Balance -= req.Amount
	bill.Status = "PAID"

	// Create transaction
	tx := Transaction{
		ID:          uuid.New().String(),
		AccountID:   account.ID,
		Date:        time.Now(),
		Description: "Bill Payment - " + bill.Payee,
		Amount:      -req.Amount,
		Type:        TransactionTypeDebit,
		Category:    "BILL_PAYMENT",
		Status:      TransactionStatusCompleted,
	}

	// Update database
	db.Accounts[account.ID] = account
	db.Bills[bill.ID] = bill
	db.Transactions[tx.ID] = tx

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message":        "Bill payment successful",
		"transaction_id": tx.ID,
	})
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
	api.Post("/bills/pay", payBill)
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

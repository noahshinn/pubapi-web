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
	LastFour  string      `json:"last_four"`
	Status    string      `json:"status"`
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
	Merchant    string            `json:"merchant"`
}

type Transfer struct {
	ID          string    `json:"id"`
	FromAccount string    `json:"from_account"`
	ToAccount   string    `json:"to_account"`
	Amount      float64   `json:"amount"`
	Memo        string    `json:"memo"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type Bill struct {
	ID        string    `json:"id"`
	Payee     string    `json:"payee"`
	Amount    float64   `json:"amount"`
	DueDate   time.Time `json:"due_date"`
	Status    string    `json:"status"`
	AutoPay   bool      `json:"autopay"`
	UserEmail string    `json:"user_email"`
}

type Database struct {
	Accounts     map[string]Account     `json:"accounts"`
	Transactions map[string]Transaction `json:"transactions"`
	Transfers    map[string]Transfer    `json:"transfers"`
	Bills        map[string]Bill        `json:"bills"`
	mu           sync.RWMutex
}

var (
	db                    *Database
	ErrAccountNotFound    = errors.New("account not found")
	ErrInsufficientFunds  = errors.New("insufficient funds")
	ErrInvalidTransaction = errors.New("invalid transaction")
	ErrUnauthorized       = errors.New("unauthorized")
)

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

func getAccounts(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	var userAccounts []Account
	db.mu.RLock()
	for _, account := range db.Accounts {
		if account.UserEmail == email {
			userAccounts = append(userAccounts, account)
		}
	}
	db.mu.RUnlock()

	return c.JSON(userAccounts)
}

func getTransactions(c *fiber.Ctx) error {
	accountId := c.Params("accountId")
	if accountId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "account ID is required",
		})
	}

	startDate := c.Query("startDate")
	endDate := c.Query("endDate")

	var start, end time.Time
	var err error
	if startDate != "" {
		start, err = time.Parse("2006-01-02", startDate)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "invalid start date format",
			})
		}
	}
	if endDate != "" {
		end, err = time.Parse("2006-01-02", endDate)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "invalid end date format",
			})
		}
	}

	var transactions []Transaction
	db.mu.RLock()
	for _, tx := range db.Transactions {
		if tx.AccountID == accountId {
			if startDate != "" && tx.Date.Before(start) {
				continue
			}
			if endDate != "" && tx.Date.After(end) {
				continue
			}
			transactions = append(transactions, tx)
		}
	}
	db.mu.RUnlock()

	return c.JSON(transactions)
}

type TransferRequest struct {
	FromAccount string  `json:"from_account"`
	ToAccount   string  `json:"to_account"`
	Amount      float64 `json:"amount"`
	Memo        string  `json:"memo"`
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

	db.mu.Lock()
	defer db.mu.Unlock()

	// Verify accounts exist
	fromAcc, exists := db.Accounts[req.FromAccount]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Source account not found",
		})
	}

	toAcc, exists := db.Accounts[req.ToAccount]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Destination account not found",
		})
	}

	// Check sufficient funds
	if fromAcc.Balance < req.Amount {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Insufficient funds",
		})
	}

	// Create transfer
	transfer := Transfer{
		ID:          uuid.New().String(),
		FromAccount: req.FromAccount,
		ToAccount:   req.ToAccount,
		Amount:      req.Amount,
		Memo:        req.Memo,
		Status:      "COMPLETED",
		CreatedAt:   time.Now(),
	}

	// Update account balances
	fromAcc.Balance -= req.Amount
	toAcc.Balance += req.Amount
	db.Accounts[req.FromAccount] = fromAcc
	db.Accounts[req.ToAccount] = toAcc

	// Save transfer
	db.Transfers[transfer.ID] = transfer

	// Create transactions
	debitTx := Transaction{
		ID:          uuid.New().String(),
		AccountID:   req.FromAccount,
		Date:        time.Now(),
		Description: "Transfer: " + req.Memo,
		Amount:      -req.Amount,
		Type:        TransactionTypeDebit,
		Category:    "Transfer",
		Status:      TransactionStatusCompleted,
	}

	creditTx := Transaction{
		ID:          uuid.New().String(),
		AccountID:   req.ToAccount,
		Date:        time.Now(),
		Description: "Transfer: " + req.Memo,
		Amount:      req.Amount,
		Type:        TransactionTypeCredit,
		Category:    "Transfer",
		Status:      TransactionStatusCompleted,
	}

	db.Transactions[debitTx.ID] = debitTx
	db.Transactions[creditTx.ID] = creditTx

	return c.Status(fiber.StatusCreated).JSON(transfer)
}

func getBills(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	var userBills []Bill
	db.mu.RLock()
	for _, bill := range db.Bills {
		if bill.UserEmail == email {
			userBills = append(userBills, bill)
		}
	}
	db.mu.RUnlock()

	return c.JSON(userBills)
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
	api.Get("/accounts/:accountId/transactions", getTransactions)

	// Transfer routes
	api.Post("/transfers", createTransfer)

	// Bill routes
	api.Get("/bills", getBills)
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
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE",
		AllowHeaders: "Origin, Content-Type, Accept",
	}))

	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

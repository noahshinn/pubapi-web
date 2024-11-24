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
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/google/uuid"
)

type Account struct {
	ID        string  `json:"id"`
	Type      string  `json:"type"`
	Name      string  `json:"name"`
	Balance   float64 `json:"balance"`
	Currency  string  `json:"currency"`
	Status    string  `json:"status"`
	LastFour  string  `json:"last_four"`
	UserEmail string  `json:"user_email"`
}

type Transaction struct {
	ID          string    `json:"id"`
	AccountID   string    `json:"account_id"`
	Date        time.Time `json:"date"`
	Description string    `json:"description"`
	Amount      float64   `json:"amount"`
	Type        string    `json:"type"`
	Category    string    `json:"category"`
	Status      string    `json:"status"`
}

type Transfer struct {
	ID          string    `json:"id"`
	FromAccount string    `json:"from_account"`
	ToAccount   string    `json:"to_account"`
	Amount      float64   `json:"amount"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	Date        time.Time `json:"date"`
}

type Bill struct {
	ID        string    `json:"id"`
	Payee     string    `json:"payee"`
	Amount    float64   `json:"amount"`
	DueDate   time.Time `json:"due_date"`
	Status    string    `json:"status"`
	Autopay   bool      `json:"autopay"`
	UserEmail string    `json:"user_email"`
}

type BillPayment struct {
	ID          string    `json:"id"`
	BillID      string    `json:"bill_id"`
	AccountID   string    `json:"account_id"`
	Amount      float64   `json:"amount"`
	PaymentDate time.Time `json:"payment_date"`
	Status      string    `json:"status"`
}

type Database struct {
	Accounts     map[string]Account     `json:"accounts"`
	Transactions map[string]Transaction `json:"transactions"`
	Bills        map[string]Bill        `json:"bills"`
	BillPayments map[string]BillPayment `json:"bill_payments"`
	Transfers    map[string]Transfer    `json:"transfers"`
	mu           sync.RWMutex
}

var db *Database

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Accounts:     make(map[string]Account),
		Transactions: make(map[string]Transaction),
		Bills:        make(map[string]Bill),
		BillPayments: make(map[string]BillPayment),
		Transfers:    make(map[string]Transfer),
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
	accountID := c.Params("accountId")
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
		if tx.AccountID == accountID {
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
	Description string  `json:"description"`
}

func createTransfer(c *fiber.Ctx) error {
	var req TransferRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate accounts exist and have sufficient funds
	db.mu.Lock()
	defer db.mu.Unlock()

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
		Description: req.Description,
		Status:      "completed",
		Date:        time.Now(),
	}

	// Update account balances
	fromAcc.Balance -= req.Amount
	toAcc.Balance += req.Amount
	db.Accounts[req.FromAccount] = fromAcc
	db.Accounts[req.ToAccount] = toAcc

	// Save transfer
	db.Transfers[transfer.ID] = transfer

	// Create transactions
	fromTx := Transaction{
		ID:          uuid.New().String(),
		AccountID:   req.FromAccount,
		Date:        time.Now(),
		Description: req.Description,
		Amount:      -req.Amount,
		Type:        "transfer_out",
		Category:    "transfer",
		Status:      "completed",
	}
	toTx := Transaction{
		ID:          uuid.New().String(),
		AccountID:   req.ToAccount,
		Date:        time.Now(),
		Description: req.Description,
		Amount:      req.Amount,
		Type:        "transfer_in",
		Category:    "transfer",
		Status:      "completed",
	}

	db.Transactions[fromTx.ID] = fromTx
	db.Transactions[toTx.ID] = toTx

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

type BillPaymentRequest struct {
	BillID      string    `json:"bill_id"`
	AccountID   string    `json:"account_id"`
	Amount      float64   `json:"amount"`
	PaymentDate time.Time `json:"payment_date"`
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

	// Validate bill exists
	bill, exists := db.Bills[req.BillID]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Bill not found",
		})
	}

	// Validate account exists and has sufficient funds
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

	// Create bill payment
	payment := BillPayment{
		ID:          uuid.New().String(),
		BillID:      req.BillID,
		AccountID:   req.AccountID,
		Amount:      req.Amount,
		PaymentDate: req.PaymentDate,
		Status:      "completed",
	}

	// Update account balance
	account.Balance -= req.Amount
	db.Accounts[req.AccountID] = account

	// Update bill status
	bill.Status = "paid"
	db.Bills[req.BillID] = bill

	// Save payment
	db.BillPayments[payment.ID] = payment

	// Create transaction
	tx := Transaction{
		ID:          uuid.New().String(),
		AccountID:   req.AccountID,
		Date:        time.Now(),
		Description: "Bill payment - " + bill.Payee,
		Amount:      -req.Amount,
		Type:        "bill_payment",
		Category:    "bills",
		Status:      "completed",
	}
	db.Transactions[tx.ID] = tx

	return c.Status(fiber.StatusCreated).JSON(payment)
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
	api.Post("/bills", payBill)
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

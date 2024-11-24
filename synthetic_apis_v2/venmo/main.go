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
	"github.com/google/uuid"
)

type User struct {
	Email          string   `json:"email"`
	Name           string   `json:"name"`
	Username       string   `json:"username"`
	ProfilePicture string   `json:"profile_picture"`
	Friends        []string `json:"friends"` // List of friend emails
	Balance        float64  `json:"balance"`
	PendingBalance float64  `json:"pending_balance"`
}

type Transaction struct {
	ID        string    `json:"id"`
	Sender    User      `json:"sender"`
	Recipient User      `json:"recipient"`
	Amount    float64   `json:"amount"`
	Note      string    `json:"note"`
	Privacy   string    `json:"privacy"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type Database struct {
	Users        map[string]User        `json:"users"`
	Transactions map[string]Transaction `json:"transactions"`
	mu           sync.RWMutex
}

var db *Database

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

func getTransactions(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	limit := c.QueryInt("limit", 20)

	db.mu.RLock()
	defer db.mu.RUnlock()

	var userTransactions []Transaction
	for _, tx := range db.Transactions {
		if tx.Sender.Email == email || tx.Recipient.Email == email {
			userTransactions = append(userTransactions, tx)
		}
		if len(userTransactions) >= limit {
			break
		}
	}

	return c.JSON(userTransactions)
}

func createTransaction(c *fiber.Ctx) error {
	var req struct {
		SenderEmail    string  `json:"sender_email"`
		RecipientEmail string  `json:"recipient_email"`
		Amount         float64 `json:"amount"`
		Note           string  `json:"note"`
		Privacy        string  `json:"privacy"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	sender, exists := db.Users[req.SenderEmail]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Sender not found",
		})
	}

	recipient, exists := db.Users[req.RecipientEmail]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Recipient not found",
		})
	}

	if req.Amount <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Amount must be positive",
		})
	}

	if sender.Balance < req.Amount {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Insufficient funds",
		})
	}

	// Create transaction
	tx := Transaction{
		ID:        uuid.New().String(),
		Sender:    sender,
		Recipient: recipient,
		Amount:    req.Amount,
		Note:      req.Note,
		Privacy:   req.Privacy,
		Status:    "completed",
		CreatedAt: time.Now(),
	}

	// Update balances
	sender.Balance -= req.Amount
	recipient.Balance += req.Amount

	db.Users[req.SenderEmail] = sender
	db.Users[req.RecipientEmail] = recipient
	db.Transactions[tx.ID] = tx

	return c.Status(fiber.StatusCreated).JSON(tx)
}

func getFriends(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	db.mu.RLock()
	user, exists := db.Users[email]
	if !exists {
		db.mu.RUnlock()
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	var friends []User
	for _, friendEmail := range user.Friends {
		if friend, ok := db.Users[friendEmail]; ok {
			friends = append(friends, friend)
		}
	}
	db.mu.RUnlock()

	return c.JSON(friends)
}

func getBalance(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	db.mu.RLock()
	user, exists := db.Users[email]
	if !exists {
		db.mu.RUnlock()
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}
	db.mu.RUnlock()

	return c.JSON(fiber.Map{
		"available": user.Balance,
		"pending":   user.PendingBalance,
	})
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

	api.Get("/transactions", getTransactions)
	api.Post("/transactions", createTransaction)
	api.Get("/friends", getFriends)
	api.Get("/balance", getBalance)
}

func main() {
	port := flag.String("port", "3000", "Port to run the server on")
	flag.Parse()

	if err := loadDatabase(); err != nil {
		log.Fatal(err)
	}

	app := fiber.New()

	app.Use(logger.New())
	app.Use(cors.New())

	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

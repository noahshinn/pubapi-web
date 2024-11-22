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
)

// Domain Models
type CreditScore struct {
	Score       int       `json:"score"`
	LastUpdated time.Time `json:"last_updated"`
}

type CreditScores struct {
	TransUnion CreditScore `json:"transunion"`
	Equifax    CreditScore `json:"equifax"`
}

type CreditFactor struct {
	Name           string `json:"name"`
	Impact         string `json:"impact"`
	Description    string `json:"description"`
	Recommendation string `json:"recommendation"`
}

type FinancialProduct struct {
	Type         string   `json:"type"`
	Provider     string   `json:"provider"`
	Name         string   `json:"name"`
	APR          float64  `json:"apr"`
	Terms        string   `json:"terms"`
	ApprovalOdds string   `json:"approval_odds"`
	Benefits     []string `json:"benefits"`
}

type PaymentHistory struct {
	Date   time.Time `json:"date"`
	Status string    `json:"status"`
	Amount float64   `json:"amount"`
}

type CreditAccount struct {
	Name           string           `json:"name"`
	Type           string           `json:"type"`
	Balance        float64          `json:"balance"`
	CreditLimit    float64          `json:"credit_limit"`
	PaymentStatus  string           `json:"payment_status"`
	PaymentHistory []PaymentHistory `json:"payment_history"`
}

type CreditInquiry struct {
	Date     time.Time `json:"date"`
	Creditor string    `json:"creditor"`
	Type     string    `json:"type"`
}

type PersonalInfo struct {
	Name      string   `json:"name"`
	Addresses []string `json:"addresses"`
	Employers []string `json:"employers"`
}

type CreditReport struct {
	Accounts     []CreditAccount `json:"accounts"`
	Inquiries    []CreditInquiry `json:"inquiries"`
	PersonalInfo PersonalInfo    `json:"personal_info"`
}

type UserProfile struct {
	Email         string         `json:"email"`
	CreditScores  CreditScores   `json:"credit_scores"`
	CreditFactors []CreditFactor `json:"credit_factors"`
	CreditReport  CreditReport   `json:"credit_report"`
}

// Database represents our in-memory database
type Database struct {
	Users map[string]UserProfile `json:"users"`
	mu    sync.RWMutex
}

var db *Database

// Database operations
func (d *Database) GetUser(email string) (UserProfile, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	user, exists := d.Users[email]
	if !exists {
		return UserProfile{}, fiber.NewError(fiber.StatusNotFound, "User not found")
	}
	return user, nil
}

// HTTP Handlers
func getCreditScores(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Email is required")
	}

	user, err := db.GetUser(email)
	if err != nil {
		return err
	}

	return c.JSON(user.CreditScores)
}

func getCreditFactors(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Email is required")
	}

	user, err := db.GetUser(email)
	if err != nil {
		return err
	}

	return c.JSON(user.CreditFactors)
}

func getRecommendations(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Email is required")
	}

	user, err := db.GetUser(email)
	if err != nil {
		return err
	}

	// Generate recommendations based on credit score
	var recommendations []FinancialProduct
	transunionScore := user.CreditScores.TransUnion.Score

	if transunionScore >= 740 {
		recommendations = append(recommendations, FinancialProduct{
			Type:         "credit_card",
			Provider:     "Premium Bank",
			Name:         "Premium Rewards Card",
			APR:          14.99,
			Terms:        "No annual fee",
			ApprovalOdds: "Excellent",
			Benefits:     []string{"5% cashback on travel", "3% on dining", "1% on everything else"},
		})
	} else if transunionScore >= 670 {
		recommendations = append(recommendations, FinancialProduct{
			Type:         "credit_card",
			Provider:     "Midtier Bank",
			Name:         "Cash Rewards Card",
			APR:          19.99,
			Terms:        "$95 annual fee",
			ApprovalOdds: "Good",
			Benefits:     []string{"2% cashback on all purchases", "Welcome bonus"},
		})
	} else {
		recommendations = append(recommendations, FinancialProduct{
			Type:         "credit_card",
			Provider:     "Secure Bank",
			Name:         "Secured Credit Builder Card",
			APR:          24.99,
			Terms:        "$200 minimum deposit",
			ApprovalOdds: "Very Good",
			Benefits:     []string{"Build credit history", "Graduate to unsecured card"},
		})
	}

	return c.JSON(recommendations)
}

func getCreditReport(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Email is required")
	}

	user, err := db.GetUser(email)
	if err != nil {
		return err
	}

	return c.JSON(user.CreditReport)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users: make(map[string]UserProfile),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	api.Get("/credit-scores", getCreditScores)
	api.Get("/credit-factors", getCreditFactors)
	api.Get("/recommendations", getRecommendations)
	api.Get("/credit-report", getCreditReport)
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

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

// Domain Models
type InsuranceType string

const (
	Auto    InsuranceType = "auto"
	Home    InsuranceType = "home"
	Life    InsuranceType = "life"
	Renters InsuranceType = "renters"
)

type PolicyStatus string

const (
	Active    PolicyStatus = "active"
	Expired   PolicyStatus = "expired"
	Cancelled PolicyStatus = "cancelled"
	Pending   PolicyStatus = "pending"
)

type ClaimStatus string

const (
	Filed     ClaimStatus = "filed"
	Review    ClaimStatus = "under_review"
	Approved  ClaimStatus = "approved"
	Denied    ClaimStatus = "denied"
	Completed ClaimStatus = "completed"
)

type Vehicle struct {
	Make         string `json:"make"`
	Model        string `json:"model"`
	Year         int    `json:"year"`
	VIN          string `json:"vin"`
	LicensePlate string `json:"license_plate"`
}

type Property struct {
	Type          string  `json:"type"` // house, apartment, condo
	Address       string  `json:"address"`
	BuildYear     int     `json:"build_year"`
	SquareFootage float64 `json:"square_footage"`
	NumBedrooms   int     `json:"num_bedrooms"`
	NumBathrooms  float64 `json:"num_bathrooms"`
}

type Policy struct {
	ID             string        `json:"id"`
	UserEmail      string        `json:"user_email"`
	Type           InsuranceType `json:"type"`
	Status         PolicyStatus  `json:"status"`
	CoverageAmount float64       `json:"coverage_amount"`
	Premium        float64       `json:"premium"`
	StartDate      time.Time     `json:"start_date"`
	EndDate        time.Time     `json:"end_date"`
	Vehicle        *Vehicle      `json:"vehicle,omitempty"`
	Property       *Property     `json:"property,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
}

type Claim struct {
	ID             string      `json:"id"`
	PolicyID       string      `json:"policy_id"`
	UserEmail      string      `json:"user_email"`
	Type           string      `json:"type"`
	Status         ClaimStatus `json:"status"`
	Description    string      `json:"description"`
	Amount         float64     `json:"amount"`
	DateOfIncident time.Time   `json:"date_of_incident"`
	DateFiled      time.Time   `json:"date_filed"`
	Documents      []string    `json:"documents"`
}

type Quote struct {
	ID              string        `json:"id"`
	InsuranceType   InsuranceType `json:"insurance_type"`
	CoverageAmount  float64       `json:"coverage_amount"`
	MonthlyPremium  float64       `json:"monthly_premium"`
	CoverageDetails interface{}   `json:"coverage_details"`
	ValidUntil      time.Time     `json:"valid_until"`
	CreatedAt       time.Time     `json:"created_at"`
}

// Database represents our in-memory database
type Database struct {
	Policies map[string]Policy `json:"policies"`
	Claims   map[string]Claim  `json:"claims"`
	Quotes   map[string]Quote  `json:"quotes"`
	mu       sync.RWMutex
}

var db *Database

// Database operations
func (d *Database) GetPoliciesByUser(email string) []Policy {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var policies []Policy
	for _, policy := range d.Policies {
		if policy.UserEmail == email {
			policies = append(policies, policy)
		}
	}
	return policies
}

func (d *Database) GetClaimsByUser(email string) []Claim {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var claims []Claim
	for _, claim := range d.Claims {
		if claim.UserEmail == email {
			claims = append(claims, claim)
		}
	}
	return claims
}

func (d *Database) CreateClaim(claim Claim) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Claims[claim.ID] = claim
	return nil
}

func (d *Database) CreateQuote(quote Quote) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Quotes[quote.ID] = quote
	return nil
}

// HTTP Handlers
func getPolicies(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	policies := db.GetPoliciesByUser(email)
	return c.JSON(policies)
}

func getClaims(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	claims := db.GetClaimsByUser(email)
	return c.JSON(claims)
}

type NewClaimRequest struct {
	PolicyID       string    `json:"policy_id"`
	Type           string    `json:"type"`
	Description    string    `json:"description"`
	DateOfIncident time.Time `json:"date_of_incident"`
	Amount         float64   `json:"amount"`
}

func createClaim(c *fiber.Ctx) error {
	var req NewClaimRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate policy exists
	var policy Policy
	var found bool
	for _, p := range db.Policies {
		if p.ID == req.PolicyID {
			policy = p
			found = true
			break
		}
	}

	if !found {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Policy not found",
		})
	}

	claim := Claim{
		ID:             uuid.New().String(),
		PolicyID:       req.PolicyID,
		UserEmail:      policy.UserEmail,
		Type:           req.Type,
		Status:         Filed,
		Description:    req.Description,
		Amount:         req.Amount,
		DateOfIncident: req.DateOfIncident,
		DateFiled:      time.Now(),
		Documents:      []string{},
	}

	if err := db.CreateClaim(claim); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create claim",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(claim)
}

type QuoteRequest struct {
	InsuranceType  InsuranceType `json:"insurance_type"`
	CoverageAmount float64       `json:"coverage_amount"`
	PersonalInfo   interface{}   `json:"personal_info"`
}

func getQuote(c *fiber.Ctx) error {
	var req QuoteRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Calculate premium (simplified)
	var monthlyPremium float64
	switch req.InsuranceType {
	case Auto:
		monthlyPremium = req.CoverageAmount * 0.004
	case Home:
		monthlyPremium = req.CoverageAmount * 0.002
	case Life:
		monthlyPremium = req.CoverageAmount * 0.003
	case Renters:
		monthlyPremium = req.CoverageAmount * 0.001
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid insurance type",
		})
	}

	quote := Quote{
		ID:             uuid.New().String(),
		InsuranceType:  req.InsuranceType,
		CoverageAmount: req.CoverageAmount,
		MonthlyPremium: monthlyPremium,
		CoverageDetails: map[string]interface{}{
			"deductible":      req.CoverageAmount * 0.01,
			"coverage_limits": req.CoverageAmount,
		},
		ValidUntil: time.Now().Add(30 * 24 * time.Hour),
		CreatedAt:  time.Now(),
	}

	if err := db.CreateQuote(quote); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create quote",
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
		Policies: make(map[string]Policy),
		Claims:   make(map[string]Claim),
		Quotes:   make(map[string]Quote),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Policy routes
	api.Get("/policies", getPolicies)

	// Claims routes
	api.Get("/claims", getClaims)
	api.Post("/claims", createClaim)

	// Quote routes
	api.Post("/quotes", getQuote)
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

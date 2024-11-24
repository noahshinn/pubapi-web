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
type Vehicle struct {
	Year  int    `json:"year"`
	Make  string `json:"make"`
	Model string `json:"model"`
	VIN   string `json:"vin"`
}

type Coverage struct {
	Type               string  `json:"type"`
	LiabilityLimit     float64 `json:"liability_limit"`
	Deductible         float64 `json:"deductible"`
	CollisionCover     bool    `json:"collision_cover"`
	ComprehensiveCover bool    `json:"comprehensive_cover"`
}

type Policy struct {
	PolicyNumber string    `json:"policy_number"`
	UserEmail    string    `json:"user_email"`
	Type         string    `json:"type"`
	Status       string    `json:"status"`
	Vehicle      Vehicle   `json:"vehicle"`
	Coverage     Coverage  `json:"coverage"`
	Premium      float64   `json:"premium"`
	StartDate    time.Time `json:"start_date"`
	EndDate      time.Time `json:"end_date"`
}

type ClaimStatus string

const (
	ClaimStatusPending   ClaimStatus = "pending"
	ClaimStatusReviewing ClaimStatus = "reviewing"
	ClaimStatusApproved  ClaimStatus = "approved"
	ClaimStatusDenied    ClaimStatus = "denied"
	ClaimStatusClosed    ClaimStatus = "closed"
)

type Claim struct {
	ClaimNumber  string      `json:"claim_number"`
	PolicyNumber string      `json:"policy_number"`
	UserEmail    string      `json:"user_email"`
	Type         string      `json:"type"`
	Status       ClaimStatus `json:"status"`
	Description  string      `json:"description"`
	IncidentDate time.Time   `json:"incident_date"`
	FiledDate    time.Time   `json:"filed_date"`
	Documents    []Document  `json:"documents"`
}

type Document struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	URL        string    `json:"url"`
	UploadedAt time.Time `json:"uploaded_at"`
}

type QuoteRequest struct {
	Email    string   `json:"email"`
	Vehicle  Vehicle  `json:"vehicle"`
	Coverage Coverage `json:"coverage"`
}

type Quote struct {
	QuoteID        string    `json:"quote_id"`
	UserEmail      string    `json:"user_email"`
	Vehicle        Vehicle   `json:"vehicle"`
	Coverage       Coverage  `json:"coverage"`
	MonthlyPremium float64   `json:"monthly_premium"`
	ExpiresAt      time.Time `json:"expires_at"`
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
func (d *Database) GetPolicy(policyNumber string) (Policy, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	policy, exists := d.Policies[policyNumber]
	if !exists {
		return Policy{}, errors.New("policy not found")
	}
	return policy, nil
}

func (d *Database) GetUserPolicies(email string) []Policy {
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

func (d *Database) CreateClaim(claim Claim) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Claims[claim.ClaimNumber] = claim
	return nil
}

func (d *Database) GetUserClaims(email string) []Claim {
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

func (d *Database) SaveQuote(quote Quote) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Quotes[quote.QuoteID] = quote
	return nil
}

// Handlers
func getAutoQuote(c *fiber.Ctx) error {
	var req QuoteRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Calculate premium based on vehicle and coverage
	premium := calculatePremium(req.Vehicle, req.Coverage)

	quote := Quote{
		QuoteID:        uuid.New().String(),
		UserEmail:      req.Email,
		Vehicle:        req.Vehicle,
		Coverage:       req.Coverage,
		MonthlyPremium: premium,
		ExpiresAt:      time.Now().Add(30 * 24 * time.Hour), // Quote valid for 30 days
	}

	if err := db.SaveQuote(quote); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to save quote",
		})
	}

	return c.JSON(quote)
}

func calculatePremium(vehicle Vehicle, coverage Coverage) float64 {
	// Basic premium calculation logic
	basePremium := 100.0

	// Adjust for vehicle age
	vehicleAge := time.Now().Year() - vehicle.Year
	if vehicleAge > 10 {
		basePremium *= 1.2
	} else if vehicleAge < 3 {
		basePremium *= 1.4
	}

	// Adjust for coverage
	if coverage.CollisionCover {
		basePremium *= 1.3
	}
	if coverage.ComprehensiveCover {
		basePremium *= 1.4
	}

	// Adjust for liability limit
	basePremium *= (coverage.LiabilityLimit / 50000.0)

	// Adjust for deductible
	basePremium *= (1000.0 / coverage.Deductible)

	return basePremium
}

func getUserPolicies(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email parameter is required",
		})
	}

	policies := db.GetUserPolicies(email)
	return c.JSON(policies)
}

func getUserClaims(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email parameter is required",
		})
	}

	claims := db.GetUserClaims(email)
	return c.JSON(claims)
}

func fileClaim(c *fiber.Ctx) error {
	var newClaim struct {
		PolicyNumber string `json:"policy_number"`
		Type         string `json:"type"`
		Description  string `json:"description"`
		IncidentDate string `json:"incident_date"`
	}

	if err := c.BodyParser(&newClaim); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate policy exists
	policy, err := db.GetPolicy(newClaim.PolicyNumber)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Policy not found",
		})
	}

	incidentDate, err := time.Parse(time.RFC3339, newClaim.IncidentDate)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid incident date format",
		})
	}

	claim := Claim{
		ClaimNumber:  "CLM-" + uuid.New().String(),
		PolicyNumber: newClaim.PolicyNumber,
		UserEmail:    policy.UserEmail,
		Type:         newClaim.Type,
		Status:       ClaimStatusPending,
		Description:  newClaim.Description,
		IncidentDate: incidentDate,
		FiledDate:    time.Now(),
		Documents:    []Document{},
	}

	if err := db.CreateClaim(claim); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create claim",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(claim)
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

	// Quote routes
	api.Post("/quotes/auto", getAutoQuote)

	// Policy routes
	api.Get("/policies", getUserPolicies)

	// Claim routes
	api.Get("/claims", getUserClaims)
	api.Post("/claims", fileClaim)
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

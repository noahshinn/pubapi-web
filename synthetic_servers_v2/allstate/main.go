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

type PolicyType string
type PolicyStatus string
type ClaimStatus string

const (
	PolicyTypeAuto    PolicyType = "auto"
	PolicyTypeHome    PolicyType = "home"
	PolicyTypeLife    PolicyType = "life"
	PolicyTypeRenters PolicyType = "renters"

	PolicyStatusActive   PolicyStatus = "active"
	PolicyStatusPending  PolicyStatus = "pending"
	PolicyStatusExpired  PolicyStatus = "expired"
	PolicyStatusCanceled PolicyStatus = "canceled"

	ClaimStatusSubmitted ClaimStatus = "submitted"
	ClaimStatusReview    ClaimStatus = "under_review"
	ClaimStatusApproved  ClaimStatus = "approved"
	ClaimStatusDenied    ClaimStatus = "denied"
	ClaimStatusPaid      ClaimStatus = "paid"
)

type Vehicle struct {
	Make         string `json:"make"`
	Model        string `json:"model"`
	Year         int    `json:"year"`
	VIN          string `json:"vin"`
	LicensePlate string `json:"license_plate"`
}

type Property struct {
	Address     string  `json:"address"`
	Type        string  `json:"type"`
	YearBuilt   int     `json:"year_built"`
	SquareFeet  int     `json:"square_feet"`
	NumBedrooms int     `json:"num_bedrooms"`
	NumBaths    float64 `json:"num_baths"`
}

type PolicyDetails struct {
	Vehicle  *Vehicle  `json:"vehicle,omitempty"`
	Property *Property `json:"property,omitempty"`
}

type Policy struct {
	ID             string        `json:"id"`
	UserEmail      string        `json:"user_email"`
	Type           PolicyType    `json:"type"`
	Status         PolicyStatus  `json:"status"`
	StartDate      time.Time     `json:"start_date"`
	EndDate        time.Time     `json:"end_date"`
	Premium        float64       `json:"premium"`
	CoverageAmount float64       `json:"coverage_amount"`
	Deductible     float64       `json:"deductible"`
	Details        PolicyDetails `json:"details"`
}

type Document struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	URL        string    `json:"url"`
	UploadedAt time.Time `json:"uploaded_at"`
}

type Claim struct {
	ID           string      `json:"id"`
	PolicyID     string      `json:"policy_id"`
	UserEmail    string      `json:"user_email"`
	Type         string      `json:"type"`
	Status       ClaimStatus `json:"status"`
	IncidentDate time.Time   `json:"incident_date"`
	Description  string      `json:"description"`
	Amount       float64     `json:"amount"`
	Documents    []Document  `json:"documents"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
}

type Database struct {
	Policies map[string]Policy `json:"policies"`
	Claims   map[string]Claim  `json:"claims"`
	mu       sync.RWMutex
}

var db *Database

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Policies: make(map[string]Policy),
		Claims:   make(map[string]Claim),
	}

	return json.Unmarshal(data, db)
}

func getPolicies(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	var userPolicies []Policy
	db.mu.RLock()
	for _, policy := range db.Policies {
		if policy.UserEmail == email {
			userPolicies = append(userPolicies, policy)
		}
	}
	db.mu.RUnlock()

	return c.JSON(userPolicies)
}

type PolicyQuoteRequest struct {
	Type           PolicyType    `json:"type"`
	UserEmail      string        `json:"user_email"`
	CoverageAmount float64       `json:"coverage_amount"`
	Details        PolicyDetails `json:"details"`
}

func createPolicyQuote(c *fiber.Ctx) error {
	var req PolicyQuoteRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Calculate premium based on policy type and coverage amount
	var premium float64
	var deductible float64

	switch req.Type {
	case PolicyTypeAuto:
		premium = req.CoverageAmount * 0.05
		deductible = 500
	case PolicyTypeHome:
		premium = req.CoverageAmount * 0.03
		deductible = 1000
	case PolicyTypeLife:
		premium = req.CoverageAmount * 0.02
		deductible = 0
	case PolicyTypeRenters:
		premium = req.CoverageAmount * 0.04
		deductible = 250
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid policy type",
		})
	}

	quote := Policy{
		ID:             uuid.New().String(),
		UserEmail:      req.UserEmail,
		Type:           req.Type,
		Status:         PolicyStatusPending,
		Premium:        premium,
		CoverageAmount: req.CoverageAmount,
		Deductible:     deductible,
		Details:        req.Details,
	}

	return c.Status(fiber.StatusCreated).JSON(quote)
}

func getClaims(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	var userClaims []Claim
	db.mu.RLock()
	for _, claim := range db.Claims {
		if claim.UserEmail == email {
			userClaims = append(userClaims, claim)
		}
	}
	db.mu.RUnlock()

	return c.JSON(userClaims)
}

type NewClaimRequest struct {
	PolicyID     string    `json:"policy_id"`
	Type         string    `json:"type"`
	IncidentDate time.Time `json:"incident_date"`
	Description  string    `json:"description"`
	Amount       float64   `json:"amount"`
}

func createClaim(c *fiber.Ctx) error {
	var req NewClaimRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.RLock()
	policy, exists := db.Policies[req.PolicyID]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Policy not found",
		})
	}

	claim := Claim{
		ID:           uuid.New().String(),
		PolicyID:     req.PolicyID,
		UserEmail:    policy.UserEmail,
		Type:         req.Type,
		Status:       ClaimStatusSubmitted,
		IncidentDate: req.IncidentDate,
		Description:  req.Description,
		Amount:       req.Amount,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	db.mu.Lock()
	db.Claims[claim.ID] = claim
	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(claim)
}

func getClaimDetails(c *fiber.Ctx) error {
	claimID := c.Params("claimId")
	if claimID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "claim ID is required",
		})
	}

	db.mu.RLock()
	claim, exists := db.Claims[claimID]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Claim not found",
		})
	}

	return c.JSON(claim)
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

	// Policy routes
	api.Get("/policies", getPolicies)
	api.Post("/policies", createPolicyQuote)

	// Claims routes
	api.Get("/claims", getClaims)
	api.Post("/claims", createClaim)
	api.Get("/claims/:claimId", getClaimDetails)
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

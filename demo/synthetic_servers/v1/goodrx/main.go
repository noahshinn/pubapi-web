package main

import (
	"encoding/json"
	"errors"
	"flag"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/google/uuid"
)

// Domain Models
type Drug struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	GenericName  string   `json:"genericName"`
	Form         string   `json:"form"`
	Strength     string   `json:"strength"`
	Manufacturer string   `json:"manufacturer"`
	Categories   []string `json:"categories"`
}

type Pharmacy struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Phone   string `json:"phone"`
	Hours   string `json:"hours"`
	ZipCode string `json:"zipCode"`
}

type DrugPrice struct {
	Pharmacy        Pharmacy `json:"pharmacy"`
	Price           float64  `json:"price"`
	Quantity        int      `json:"quantity"`
	DiscountPrice   float64  `json:"discountPrice"`
	DiscountPercent float64  `json:"discountPercent"`
}

type Prescription struct {
	ID             string    `json:"id"`
	UserEmail      string    `json:"userEmail"`
	Drug           Drug      `json:"drug"`
	Prescriber     string    `json:"prescriber"`
	Quantity       int       `json:"quantity"`
	Refills        int       `json:"refills"`
	ExpirationDate time.Time `json:"expirationDate"`
	CreatedAt      time.Time `json:"createdAt"`
}

type Coupon struct {
	ID             string    `json:"id"`
	DrugID         string    `json:"drugId"`
	PharmacyID     string    `json:"pharmacyId"`
	DiscountPrice  float64   `json:"discountPrice"`
	OriginalPrice  float64   `json:"originalPrice"`
	ExpirationDate time.Time `json:"expirationDate"`
	BarcodeData    string    `json:"barcodeData"`
}

type User struct {
	Email         string         `json:"email"`
	Name          string         `json:"name"`
	Prescriptions []Prescription `json:"prescriptions"`
	SearchHistory []SearchEntry  `json:"searchHistory"`
}

type SearchEntry struct {
	Query     string    `json:"query"`
	Timestamp time.Time `json:"timestamp"`
}

// Database represents our in-memory database
type Database struct {
	Users         map[string]User         `json:"users"`
	Drugs         map[string]Drug         `json:"drugs"`
	Pharmacies    map[string]Pharmacy     `json:"pharmacies"`
	Prescriptions map[string]Prescription `json:"prescriptions"`
	Coupons       map[string]Coupon       `json:"coupons"`
	mu            sync.RWMutex
}

// Custom errors
var (
	ErrUserNotFound         = errors.New("user not found")
	ErrDrugNotFound         = errors.New("drug not found")
	ErrPharmacyNotFound     = errors.New("pharmacy not found")
	ErrPrescriptionNotFound = errors.New("prescription not found")
	ErrInvalidInput         = errors.New("invalid input")
)

// Global database instance
var db *Database

// Database operations
func (d *Database) SearchDrugs(query string) []Drug {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var results []Drug
	query = strings.ToLower(query)

	for _, drug := range d.Drugs {
		if strings.Contains(strings.ToLower(drug.Name), query) ||
			strings.Contains(strings.ToLower(drug.GenericName), query) {
			results = append(results, drug)
		}
	}

	return results
}

func (d *Database) GetDrugPrices(drugID string, zipCode string) []DrugPrice {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var prices []DrugPrice

	for _, pharmacy := range d.Pharmacies {
		if pharmacy.ZipCode == zipCode {
			// Simulate price calculation based on pharmacy
			basePrice := 100.0 // Base price for simulation
			price := DrugPrice{
				Pharmacy:        pharmacy,
				Price:           basePrice,
				Quantity:        30,
				DiscountPrice:   basePrice * 0.7,
				DiscountPercent: 30,
			}
			prices = append(prices, price)
		}
	}

	return prices
}

func (d *Database) GetUserPrescriptions(email string) ([]Prescription, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	user, exists := d.Users[email]
	if !exists {
		return nil, ErrUserNotFound
	}

	return user.Prescriptions, nil
}

func (d *Database) AddPrescription(prescription Prescription) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	user, exists := d.Users[prescription.UserEmail]
	if !exists {
		return ErrUserNotFound
	}

	user.Prescriptions = append(user.Prescriptions, prescription)
	d.Users[prescription.UserEmail] = user
	d.Prescriptions[prescription.ID] = prescription

	return nil
}

func (d *Database) GetCoupon(drugID string, pharmacyID string) (Coupon, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	for _, coupon := range d.Coupons {
		if coupon.DrugID == drugID && coupon.PharmacyID == pharmacyID {
			return coupon, nil
		}
	}

	return Coupon{}, errors.New("coupon not found")
}

// HTTP Handlers
func searchDrugs(c *fiber.Ctx) error {
	query := c.Query("query")
	if query == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "search query is required",
		})
	}

	results := db.SearchDrugs(query)
	return c.JSON(results)
}

func getDrugPrices(c *fiber.Ctx) error {
	drugID := c.Params("drugId")
	zipCode := c.Query("zipCode")

	if zipCode == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "zipCode is required",
		})
	}

	prices := db.GetDrugPrices(drugID, zipCode)
	return c.JSON(prices)
}

func getUserPrescriptions(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	prescriptions, err := db.GetUserPrescriptions(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(prescriptions)
}

type NewPrescriptionRequest struct {
	DrugID         string    `json:"drugId"`
	UserEmail      string    `json:"userEmail"`
	Prescriber     string    `json:"prescriber"`
	Quantity       int       `json:"quantity"`
	Refills        int       `json:"refills"`
	ExpirationDate time.Time `json:"expirationDate"`
}

func addPrescription(c *fiber.Ctx) error {
	var req NewPrescriptionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	drug, exists := db.Drugs[req.DrugID]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Drug not found",
		})
	}

	prescription := Prescription{
		ID:             uuid.New().String(),
		UserEmail:      req.UserEmail,
		Drug:           drug,
		Prescriber:     req.Prescriber,
		Quantity:       req.Quantity,
		Refills:        req.Refills,
		ExpirationDate: req.ExpirationDate,
		CreatedAt:      time.Now(),
	}

	if err := db.AddPrescription(prescription); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(prescription)
}

func getCoupon(c *fiber.Ctx) error {
	drugID := c.Params("drugId")
	pharmacyID := c.Query("pharmacyId")

	if pharmacyID == "" {

		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "pharmacyId is required",
		})
	}

	coupon, err := db.GetCoupon(drugID, pharmacyID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(coupon)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:         make(map[string]User),
		Drugs:         make(map[string]Drug),
		Pharmacies:    make(map[string]Pharmacy),
		Prescriptions: make(map[string]Prescription),
		Coupons:       make(map[string]Coupon),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Drug routes
	api.Get("/drugs/search", searchDrugs)
	api.Get("/drugs/:drugId/prices", getDrugPrices)

	// Prescription routes
	api.Get("/prescriptions", getUserPrescriptions)
	api.Post("/prescriptions", addPrescription)

	// Coupon routes
	api.Get("/coupons/:drugId", getCoupon)
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
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE",
		AllowHeaders: "Origin, Content-Type, Accept",
	}))

	// Setup routes
	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

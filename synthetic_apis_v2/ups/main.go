package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/google/uuid"
)

type Address struct {
	Street     string `json:"street"`
	City       string `json:"city"`
	State      string `json:"state"`
	PostalCode string `json:"postal_code"`
	Country    string `json:"country"`
}

type Package struct {
	Weight float64 `json:"weight"`
	Length float64 `json:"length"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	Value  float64 `json:"declared_value"`
}

type TrackingEvent struct {
	Timestamp   time.Time `json:"timestamp"`
	Location    string    `json:"location"`
	Status      string    `json:"status"`
	Description string    `json:"description"`
}

type Shipment struct {
	ID             string          `json:"id"`
	TrackingNumber string          `json:"tracking_number"`
	UserEmail      string          `json:"user_email"`
	FromAddress    Address         `json:"from_address"`
	ToAddress      Address         `json:"to_address"`
	Packages       []Package       `json:"packages"`
	ServiceLevel   string          `json:"service_level"`
	Status         string          `json:"status"`
	Cost           float64         `json:"cost"`
	CreatedAt      time.Time       `json:"created_at"`
	Events         []TrackingEvent `json:"events"`
	LabelURL       string          `json:"label_url"`
}

type Database struct {
	Shipments map[string]Shipment `json:"shipments"`
	mu        sync.RWMutex
}

var db *Database

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Shipments: make(map[string]Shipment),
	}

	return json.Unmarshal(data, db)
}

func generateTrackingNumber() string {
	return fmt.Sprintf("1Z%s", uuid.New().String()[:12])
}

func calculateShippingRate(from, to Address, packages []Package, serviceLevel string) float64 {
	// Simplified rate calculation
	baseRate := 10.0
	var totalWeight float64

	for _, pkg := range packages {
		totalWeight += pkg.Weight
	}

	switch serviceLevel {
	case "GROUND":
		return baseRate + (totalWeight * 0.5)
	case "2DAY":
		return baseRate + (totalWeight * 0.75) + 15
	case "NEXTDAY":
		return baseRate + (totalWeight * 1.0) + 25
	default:
		return baseRate + (totalWeight * 0.5)
	}
}

// Handlers
func trackPackage(c *fiber.Ctx) error {
	trackingNumber := c.Params("trackingNumber")

	db.mu.RLock()
	defer db.mu.RUnlock()

	for _, shipment := range db.Shipments {
		if shipment.TrackingNumber == trackingNumber {
			return c.JSON(fiber.Map{
				"tracking_number": shipment.TrackingNumber,
				"status":          shipment.Status,
				"service_level":   shipment.ServiceLevel,
				"from":            shipment.FromAddress,
				"to":              shipment.ToAddress,
				"events":          shipment.Events,
			})
		}
	}

	return c.Status(404).JSON(fiber.Map{
		"error": "Tracking number not found",
	})
}

type ShipmentRequest struct {
	FromAddress  Address   `json:"from_address"`
	ToAddress    Address   `json:"to_address"`
	Packages     []Package `json:"packages"`
	ServiceLevel string    `json:"service_level"`
	UserEmail    string    `json:"user_email"`
}

func createShipment(c *fiber.Ctx) error {
	var req ShipmentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate request
	if req.UserEmail == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "User email is required",
		})
	}

	if len(req.Packages) == 0 {
		return c.Status(400).JSON(fiber.Map{
			"error": "At least one package is required",
		})
	}

	// Calculate shipping rate
	cost := calculateShippingRate(req.FromAddress, req.ToAddress, req.Packages, req.ServiceLevel)

	// Create new shipment
	shipment := Shipment{
		ID:             uuid.New().String(),
		TrackingNumber: generateTrackingNumber(),
		UserEmail:      req.UserEmail,
		FromAddress:    req.FromAddress,
		ToAddress:      req.ToAddress,
		Packages:       req.Packages,
		ServiceLevel:   req.ServiceLevel,
		Status:         "LABEL_CREATED",
		Cost:           cost,
		CreatedAt:      time.Now(),
		Events: []TrackingEvent{
			{
				Timestamp:   time.Now(),
				Location:    req.FromAddress.City,
				Status:      "LABEL_CREATED",
				Description: "Shipping label has been created",
			},
		},
		LabelURL: fmt.Sprintf("https://ups.com/labels/%s.pdf", uuid.New().String()),
	}

	// Save to database
	db.mu.Lock()
	db.Shipments[shipment.ID] = shipment
	db.mu.Unlock()

	return c.Status(201).JSON(shipment)
}

func getUserShipments(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Email parameter is required",
		})
	}

	var shipments []Shipment

	db.mu.RLock()
	for _, shipment := range db.Shipments {
		if shipment.UserEmail == email {
			shipments = append(shipments, shipment)
		}
	}
	db.mu.RUnlock()

	return c.JSON(shipments)
}

type RateRequest struct {
	FromAddress Address   `json:"from_address"`
	ToAddress   Address   `json:"to_address"`
	Packages    []Package `json:"packages"`
}

func calculateRates(c *fiber.Ctx) error {
	var req RateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Calculate rates for different service levels
	rates := []fiber.Map{
		{
			"service_level":       "GROUND",
			"cost":                calculateShippingRate(req.FromAddress, req.ToAddress, req.Packages, "GROUND"),
			"currency":            "USD",
			"delivery_days":       5,
			"guaranteed_delivery": false,
		},
		{
			"service_level":       "2DAY",
			"cost":                calculateShippingRate(req.FromAddress, req.ToAddress, req.Packages, "2DAY"),
			"currency":            "USD",
			"delivery_days":       2,
			"guaranteed_delivery": true,
		},
		{
			"service_level":       "NEXTDAY",
			"cost":                calculateShippingRate(req.FromAddress, req.ToAddress, req.Packages, "NEXTDAY"),
			"currency":            "USD",
			"delivery_days":       1,
			"guaranteed_delivery": true,
		},
	}

	return c.JSON(rates)
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

	// Tracking
	api.Get("/tracking/:trackingNumber", trackPackage)

	// Shipments
	api.Post("/shipments", createShipment)
	api.Get("/shipments", getUserShipments)

	// Rates
	api.Post("/rates", calculateRates)
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
	app.Use(cors.New())

	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

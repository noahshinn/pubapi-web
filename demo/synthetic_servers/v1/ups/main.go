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
type Address struct {
	Street     string `json:"street"`
	City       string `json:"city"`
	State      string `json:"state"`
	PostalCode string `json:"postal_code"`
	Country    string `json:"country"`
}

type Dimensions struct {
	Length float64 `json:"length"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type Package struct {
	Weight        float64    `json:"weight"`
	Dimensions    Dimensions `json:"dimensions"`
	DeclaredValue float64    `json:"declared_value"`
}

type TrackingEvent struct {
	Timestamp   time.Time `json:"timestamp"`
	Location    string    `json:"location"`
	Status      string    `json:"status"`
	Description string    `json:"description"`
}

type ShipmentStatus string

const (
	ShipmentStatusCreated   ShipmentStatus = "created"
	ShipmentStatusPickedUp  ShipmentStatus = "picked_up"
	ShipmentStatusInTransit ShipmentStatus = "in_transit"
	ShipmentStatusDelivered ShipmentStatus = "delivered"
	ShipmentStatusException ShipmentStatus = "exception"
)

type Shipment struct {
	ID             string          `json:"id"`
	TrackingNumber string          `json:"tracking_number"`
	UserEmail      string          `json:"user_email"`
	FromAddress    Address         `json:"from_address"`
	ToAddress      Address         `json:"to_address"`
	ServiceLevel   string          `json:"service_level"`
	PackageDetails Package         `json:"package_details"`
	Status         ShipmentStatus  `json:"status"`
	CreatedAt      time.Time       `json:"created_at"`
	TrackingEvents []TrackingEvent `json:"tracking_events"`
}

type Rate struct {
	ServiceLevel string    `json:"service_level"`
	Rate         float64   `json:"rate"`
	DeliveryDate time.Time `json:"delivery_date"`
	Guaranteed   bool      `json:"guaranteed"`
}

// Database represents our in-memory database
type Database struct {
	Shipments map[string]Shipment `json:"shipments"`
	mu        sync.RWMutex
}

// Custom errors
var (
	ErrShipmentNotFound = errors.New("shipment not found")
	ErrInvalidInput     = errors.New("invalid input")
)

// Global database instance
var db *Database

// Database operations
func (d *Database) GetShipment(id string) (Shipment, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	shipment, exists := d.Shipments[id]
	if !exists {
		return Shipment{}, ErrShipmentNotFound
	}
	return shipment, nil
}

func (d *Database) CreateShipment(shipment Shipment) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Shipments[shipment.ID] = shipment
	return nil
}

func (d *Database) GetShipmentByTracking(trackingNumber string) (Shipment, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	for _, shipment := range d.Shipments {
		if shipment.TrackingNumber == trackingNumber {
			return shipment, nil
		}
	}
	return Shipment{}, ErrShipmentNotFound
}

func (d *Database) GetUserShipments(email string) []Shipment {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var shipments []Shipment
	for _, shipment := range d.Shipments {
		if shipment.UserEmail == email {
			shipments = append(shipments, shipment)
		}
	}
	return shipments
}

// Business logic helpers
func generateTrackingNumber() string {
	return "1Z" + uuid.New().String()[:16]
}

func calculateShippingRate(from, to Address, pkg Package, serviceLevel string) float64 {
	// Simplified rate calculation
	baseRate := 10.0
	weightRate := pkg.Weight * 0.5
	volumeRate := (pkg.Dimensions.Length * pkg.Dimensions.Width * pkg.Dimensions.Height) * 0.001

	switch serviceLevel {
	case "ground":
		return baseRate + weightRate + volumeRate
	case "2day":
		return (baseRate + weightRate + volumeRate) * 1.5
	case "nextday":
		return (baseRate + weightRate + volumeRate) * 2.0
	default:
		return baseRate + weightRate + volumeRate
	}
}

// HTTP Handlers
func trackPackage(c *fiber.Ctx) error {
	trackingNumber := c.Params("trackingNumber")
	if trackingNumber == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Tracking number is required",
		})
	}

	shipment, err := db.GetShipmentByTracking(trackingNumber)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Package not found",
		})
	}

	return c.JSON(fiber.Map{
		"tracking_number": shipment.TrackingNumber,
		"status":          shipment.Status,
		"tracking_events": shipment.TrackingEvents,
		"from":            shipment.FromAddress,
		"to":              shipment.ToAddress,
		"service_level":   shipment.ServiceLevel,
	})
}

func getUserShipments(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email is required",
		})
	}

	shipments := db.GetUserShipments(email)
	return c.JSON(shipments)
}

type CreateShipmentRequest struct {
	UserEmail    string  `json:"user_email"`
	FromAddress  Address `json:"from_address"`
	ToAddress    Address `json:"to_address"`
	ServiceLevel string  `json:"service_level"`
	Package      Package `json:"package"`
}

func createShipment(c *fiber.Ctx) error {
	var req CreateShipmentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate input
	if req.UserEmail == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "User email is required",
		})
	}

	// Create new shipment
	shipment := Shipment{
		ID:             uuid.New().String(),
		TrackingNumber: generateTrackingNumber(),
		UserEmail:      req.UserEmail,
		FromAddress:    req.FromAddress,
		ToAddress:      req.ToAddress,
		ServiceLevel:   req.ServiceLevel,
		PackageDetails: req.Package,
		Status:         ShipmentStatusCreated,
		CreatedAt:      time.Now(),
		TrackingEvents: []TrackingEvent{
			{
				Timestamp:   time.Now(),
				Location:    req.FromAddress.City,
				Status:      string(ShipmentStatusCreated),
				Description: "Shipping label created",
			},
		},
	}

	if err := db.CreateShipment(shipment); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create shipment",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(shipment)
}

type RateRequest struct {
	FromAddress Address `json:"from_address"`
	ToAddress   Address `json:"to_address"`
	Package     Package `json:"package"`
}

func calculateRates(c *fiber.Ctx) error {
	var req RateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Calculate rates for different service levels
	rates := []Rate{
		{
			ServiceLevel: "ground",
			Rate:         calculateShippingRate(req.FromAddress, req.ToAddress, req.Package, "ground"),
			DeliveryDate: time.Now().AddDate(0, 0, 5),
			Guaranteed:   false,
		},
		{
			ServiceLevel: "2day",
			Rate:         calculateShippingRate(req.FromAddress, req.ToAddress, req.Package, "2day"),
			DeliveryDate: time.Now().AddDate(0, 0, 2),
			Guaranteed:   true,
		},
		{
			ServiceLevel: "nextday",
			Rate:         calculateShippingRate(req.FromAddress, req.ToAddress, req.Package, "nextday"),
			DeliveryDate: time.Now().AddDate(0, 0, 1),
			Guaranteed:   true,
		},
	}

	return c.JSON(rates)
}

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

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Tracking routes
	api.Get("/tracking/:trackingNumber", trackPackage)

	// Shipment routes
	api.Get("/shipments", getUserShipments)
	api.Post("/shipments", createShipment)

	// Rate calculation
	api.Post("/rates", calculateRates)
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

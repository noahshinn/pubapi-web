package main

import (
	"encoding/json"
	"flag"
	"log"
	"math"
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
type Location struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Address   string  `json:"address"`
}

type User struct {
	Email          string          `json:"email"`
	Name           string          `json:"name"`
	Phone          string          `json:"phone"`
	PaymentMethods []PaymentMethod `json:"payment_methods"`
	Rating         float64         `json:"rating"`
}

type PaymentMethod struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Last4    string `json:"last4"`
	ExpiryMM int    `json:"expiry_mm"`
	ExpiryYY int    `json:"expiry_yy"`
}

type Car struct {
	Make         string `json:"make"`
	Model        string `json:"model"`
	Color        string `json:"color"`
	LicensePlate string `json:"license_plate"`
}

type Driver struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Phone  string  `json:"phone"`
	Rating float64 `json:"rating"`
	Car    Car     `json:"car"`
}

type ServiceType string

const (
	UberX       ServiceType = "UberX"
	UberXL      ServiceType = "UberXL"
	UberBlack   ServiceType = "UberBlack"
	UberComfort ServiceType = "UberComfort"
)

type RideStatus string

const (
	RideStatusRequested RideStatus = "requested"
	RideStatusAccepted  RideStatus = "accepted"
	RideStatusArrived   RideStatus = "arrived"
	RideStatusStarted   RideStatus = "started"
	RideStatusCompleted RideStatus = "completed"
	RideStatusCancelled RideStatus = "cancelled"
)

type Ride struct {
	ID          string      `json:"id"`
	UserEmail   string      `json:"user_email"`
	Driver      *Driver     `json:"driver,omitempty"`
	ServiceType ServiceType `json:"service_type"`
	Status      RideStatus  `json:"status"`
	Pickup      Location    `json:"pickup"`
	Destination Location    `json:"destination"`
	Price       float64     `json:"price"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

type RideEstimate struct {
	ServiceType       ServiceType `json:"service_type"`
	EstimatedPrice    float64     `json:"estimated_price"`
	EstimatedDuration int         `json:"estimated_duration"` // in minutes
	EstimatedDistance float64     `json:"estimated_distance"` // in miles
}

// Database represents our in-memory database
type Database struct {
	Users   map[string]User   `json:"users"`
	Drivers map[string]Driver `json:"drivers"`
	Rides   map[string]Ride   `json:"rides"`
	mu      sync.RWMutex
}

var db *Database

// Helper functions
func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371 // Earth's radius in kilometers

	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	deltaLat := (lat2 - lat1) * math.Pi / 180
	deltaLon := (lon2 - lon1) * math.Pi / 180

	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(deltaLon/2)*math.Sin(deltaLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c * 0.621371 // Convert to miles
}

func calculatePrice(distance float64, serviceType ServiceType) float64 {
	basePrices := map[ServiceType]float64{
		UberX:       2.55,
		UberXL:      3.85,
		UberBlack:   7.00,
		UberComfort: 3.25,
	}

	perMilePrices := map[ServiceType]float64{
		UberX:       1.75,
		UberXL:      2.85,
		UberBlack:   4.50,
		UberComfort: 2.25,
	}

	basePrice := basePrices[serviceType]
	perMilePrice := perMilePrices[serviceType]

	return basePrice + (distance * perMilePrice)
}

// Handlers
func getRideEstimate(c *fiber.Ctx) error {
	var req struct {
		Pickup      Location `json:"pickup"`
		Destination Location `json:"destination"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	distance := calculateDistance(
		req.Pickup.Latitude,
		req.Pickup.Longitude,
		req.Destination.Latitude,
		req.Destination.Longitude,
	)

	estimates := []RideEstimate{
		{
			ServiceType:       UberX,
			EstimatedPrice:    calculatePrice(distance, UberX),
			EstimatedDuration: int(distance * 3), // Rough estimate: 3 minutes per mile
			EstimatedDistance: distance,
		},
		{
			ServiceType:       UberXL,
			EstimatedPrice:    calculatePrice(distance, UberXL),
			EstimatedDuration: int(distance * 3),
			EstimatedDistance: distance,
		},
		{
			ServiceType:       UberBlack,
			EstimatedPrice:    calculatePrice(distance, UberBlack),
			EstimatedDuration: int(distance * 3),
			EstimatedDistance: distance,
		},
		{
			ServiceType:       UberComfort,
			EstimatedPrice:    calculatePrice(distance, UberComfort),
			EstimatedDuration: int(distance * 3),
			EstimatedDistance: distance,
		},
	}

	return c.JSON(estimates)
}

func requestRide(c *fiber.Ctx) error {
	var req struct {
		UserEmail       string      `json:"user_email"`
		ServiceType     ServiceType `json:"service_type"`
		Pickup          Location    `json:"pickup"`
		Destination     Location    `json:"destination"`
		PaymentMethodID string      `json:"payment_method_id"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Verify user exists
	db.mu.RLock()
	user, exists := db.Users[req.UserEmail]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// Verify payment method
	validPayment := false
	for _, pm := range user.PaymentMethods {
		if pm.ID == req.PaymentMethodID {
			validPayment = true
			break
		}
	}
	if !validPayment {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid payment method",
		})
	}

	// Calculate price
	distance := calculateDistance(
		req.Pickup.Latitude,
		req.Pickup.Longitude,
		req.Destination.Latitude,
		req.Destination.Longitude,
	)
	price := calculatePrice(distance, req.ServiceType)

	// Create new ride
	ride := Ride{
		ID:          uuid.New().String(),
		UserEmail:   req.UserEmail,
		ServiceType: req.ServiceType,
		Status:      RideStatusRequested,
		Pickup:      req.Pickup,
		Destination: req.Destination,
		Price:       price,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Save ride
	db.mu.Lock()
	db.Rides[ride.ID] = ride
	db.mu.Unlock()

	// In a real implementation, we would now:
	// 1. Notify nearby drivers
	// 2. Handle driver acceptance
	// 3. Set up real-time location tracking

	return c.Status(fiber.StatusCreated).JSON(ride)
}

func getRideHistory(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	var userRides []Ride
	db.mu.RLock()
	for _, ride := range db.Rides {
		if ride.UserEmail == email {
			userRides = append(userRides, ride)
		}
	}
	db.mu.RUnlock()

	return c.JSON(userRides)
}

func getRideStatus(c *fiber.Ctx) error {
	rideID := c.Params("rideId")
	if rideID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "ride ID is required",
		})

	}

	db.mu.RLock()
	ride, exists := db.Rides[rideID]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Ride not found",
		})
	}

	return c.JSON(ride)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:   make(map[string]User),
		Drivers: make(map[string]Driver),
		Rides:   make(map[string]Ride),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Ride routes
	api.Post("/rides/estimate", getRideEstimate)
	api.Post("/rides", requestRide)
	api.Get("/rides", getRideHistory)
	api.Get("/rides/:rideId", getRideStatus)
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

	// Start server
	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

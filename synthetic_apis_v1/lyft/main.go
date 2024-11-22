package main

import (
	"encoding/json"
	"errors"
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

type Car struct {
	Make         string `json:"make"`
	Model        string `json:"model"`
	Year         int    `json:"year"`
	Color        string `json:"color"`
	LicensePlate string `json:"license_plate"`
}

type Driver struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Phone  string  `json:"phone"`
	Rating float64 `json:"rating"`
	Car    Car     `json:"car"`
	// Current location for nearby driver matching
	CurrentLocation Location `json:"current_location"`
	IsAvailable     bool     `json:"is_available"`
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

type RideType string

const (
	RideTypeStandard RideType = "standard"
	RideTypeXL       RideType = "xl"
	RideTypeLux      RideType = "lux"
)

type RideStatus string

const (
	RideStatusRequested  RideStatus = "requested"
	RideStatusAccepted   RideStatus = "accepted"
	RideStatusArrived    RideStatus = "arrived"
	RideStatusInProgress RideStatus = "in_progress"
	RideStatusCompleted  RideStatus = "completed"
	RideStatusCancelled  RideStatus = "cancelled"
)

type Ride struct {
	ID              string     `json:"id"`
	UserEmail       string     `json:"user_email"`
	Driver          *Driver    `json:"driver,omitempty"`
	PickupLocation  Location   `json:"pickup_location"`
	DropoffLocation Location   `json:"dropoff_location"`
	Status          RideStatus `json:"status"`
	RideType        RideType   `json:"ride_type"`
	Price           float64    `json:"price"`
	Distance        float64    `json:"distance"`
	Duration        int        `json:"duration"` // in minutes
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type RideEstimate struct {
	RideType          RideType `json:"ride_type"`
	EstimatedPrice    Price    `json:"estimated_price"`
	EstimatedDuration int      `json:"estimated_duration"` // in minutes
	EstimatedDistance float64  `json:"estimated_distance"` // in miles
}

type Price struct {
	MinAmount float64 `json:"min_amount"`
	MaxAmount float64 `json:"max_amount"`
	Currency  string  `json:"currency"`
}

// Database represents our in-memory database
type Database struct {
	Users   map[string]User   `json:"users"`
	Drivers map[string]Driver `json:"drivers"`
	Rides   map[string]Ride   `json:"rides"`
	mu      sync.RWMutex
}

var (
	db                *Database
	ErrUserNotFound   = errors.New("user not found")
	ErrDriverNotFound = errors.New("driver not found")
	ErrRideNotFound   = errors.New("ride not found")
)

// Helper functions
func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 3959.0 // miles

	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	lon1Rad := lon1 * math.Pi / 180
	lon2Rad := lon2 * math.Pi / 180

	dlat := lat2Rad - lat1Rad
	dlon := lon2Rad - lon1Rad

	a := math.Pow(math.Sin(dlat/2), 2) + math.Cos(lat1Rad)*math.Cos(lat2Rad)*math.Pow(math.Sin(dlon/2), 2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadius * c
}

func estimatePrice(distance float64, rideType RideType) Price {
	var baseRate, perMileRate float64

	switch rideType {
	case RideTypeStandard:
		baseRate = 2.50
		perMileRate = 1.50
	case RideTypeXL:
		baseRate = 3.50
		perMileRate = 2.50
	case RideTypeLux:
		baseRate = 5.00
		perMileRate = 3.50
	}

	estimatedPrice := baseRate + (distance * perMileRate)
	// Add 20% variance for min/max
	return Price{
		MinAmount: math.Floor(estimatedPrice*0.9*100) / 100,
		MaxAmount: math.Ceil(estimatedPrice*1.1*100) / 100,
		Currency:  "USD",
	}
}

func findNearbyDrivers(location Location, rideType RideType) []Driver {
	const maxDistance = 5.0 // miles
	var nearbyDrivers []Driver

	db.mu.RLock()
	defer db.mu.RUnlock()

	for _, driver := range db.Drivers {
		if !driver.IsAvailable {
			continue
		}

		distance := calculateDistance(
			location.Latitude,
			location.Longitude,
			driver.CurrentLocation.Latitude,
			driver.CurrentLocation.Longitude,
		)

		if distance <= maxDistance {
			nearbyDrivers = append(nearbyDrivers, driver)
		}
	}

	return nearbyDrivers
}

// Handlers
func getRideEstimate(c *fiber.Ctx) error {
	pickup := Location{
		Latitude:  c.QueryFloat("pickup_latitude", 0),
		Longitude: c.QueryFloat("pickup_longitude", 0),
	}
	dropoff := Location{
		Latitude:  c.QueryFloat("dropoff_latitude", 0),
		Longitude: c.QueryFloat("dropoff_longitude", 0),
	}

	if pickup.Latitude == 0 || pickup.Longitude == 0 || dropoff.Latitude == 0 || dropoff.Longitude == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid coordinates",
		})
	}

	distance := calculateDistance(
		pickup.Latitude,
		pickup.Longitude,
		dropoff.Latitude,
		dropoff.Longitude,
	)

	// Calculate estimates for all ride types
	estimates := []RideEstimate{
		{
			RideType:          RideTypeStandard,
			EstimatedPrice:    estimatePrice(distance, RideTypeStandard),
			EstimatedDuration: int(distance * 3), // Rough estimate: 20mph average
			EstimatedDistance: math.Round(distance*100) / 100,
		},
		{
			RideType:          RideTypeXL,
			EstimatedPrice:    estimatePrice(distance, RideTypeXL),
			EstimatedDuration: int(distance * 3),
			EstimatedDistance: math.Round(distance*100) / 100,
		},
		{
			RideType:          RideTypeLux,
			EstimatedPrice:    estimatePrice(distance, RideTypeLux),
			EstimatedDuration: int(distance * 3),
			EstimatedDistance: math.Round(distance*100) / 100,
		},
	}

	return c.JSON(estimates)
}

func requestRide(c *fiber.Ctx) error {
	var req struct {
		UserEmail       string   `json:"user_email"`
		PickupLocation  Location `json:"pickup_location"`
		DropoffLocation Location `json:"dropoff_location"`
		RideType        RideType `json:"ride_type"`
		PaymentMethodID string   `json:"payment_method_id"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user and payment method
	user, exists := db.Users[req.UserEmail]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

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

	// Find nearby drivers
	nearbyDrivers := findNearbyDrivers(req.PickupLocation, req.RideType)
	if len(nearbyDrivers) == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "No available drivers nearby",
		})
	}

	// Calculate ride details
	distance := calculateDistance(
		req.PickupLocation.Latitude,
		req.PickupLocation.Longitude,
		req.DropoffLocation.Latitude,
		req.DropoffLocation.Longitude,
	)

	price := estimatePrice(distance, req.RideType)

	// Create new ride
	ride := Ride{
		ID:              uuid.New().String(),
		UserEmail:       req.UserEmail,
		PickupLocation:  req.PickupLocation,
		DropoffLocation: req.DropoffLocation,
		Status:          RideStatusRequested,
		RideType:        req.RideType,
		Price:           price.MinAmount,
		Distance:        distance,
		Duration:        int(distance * 3),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	// Save ride to database
	db.mu.Lock()
	db.Rides[ride.ID] = ride
	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(ride)
}

func getRideHistory(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email is required",
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

func getRideDetails(c *fiber.Ctx) error {
	rideID := c.Params("rideId")

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

	// Ride estimation
	api.Get("/rides/estimate", getRideEstimate)

	// Ride management
	api.Post("/rides", requestRide)
	api.Get("/rides", getRideHistory)
	api.Get("/rides/:rideId", getRideDetails)
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

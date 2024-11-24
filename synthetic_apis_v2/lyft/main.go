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

type Vehicle struct {
	Make         string `json:"make"`
	Model        string `json:"model"`
	Year         int    `json:"year"`
	Color        string `json:"color"`
	LicensePlate string `json:"license_plate"`
}

type Driver struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Vehicle  Vehicle  `json:"vehicle"`
	Rating   float64  `json:"rating"`
	Location Location `json:"location"`
	Status   string   `json:"status"` // "available", "busy", "offline"
}

type RideType string

const (
	RideTypeLyft      RideType = "lyft"
	RideTypeLyftXL    RideType = "lyft_xl"
	RideTypeLyftLux   RideType = "lyft_lux"
	RideTypeLyftBlack RideType = "lyft_black"
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
	ID        string     `json:"id"`
	UserEmail string     `json:"user_email"`
	Driver    *Driver    `json:"driver"`
	Pickup    Location   `json:"pickup"`
	Dropoff   Location   `json:"dropoff"`
	Status    RideStatus `json:"status"`
	RideType  RideType   `json:"ride_type"`
	Price     float64    `json:"price"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	PaymentID string     `json:"payment_id"`
}

type Database struct {
	Drivers map[string]Driver `json:"drivers"`
	Rides   map[string]Ride   `json:"rides"`
	Users   map[string]User   `json:"users"`
	mu      sync.RWMutex
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

var db *Database

// Error definitions
var (
	ErrUserNotFound    = errors.New("user not found")
	ErrDriverNotFound  = errors.New("driver not found")
	ErrRideNotFound    = errors.New("ride not found")
	ErrInvalidInput    = errors.New("invalid input")
	ErrNoDriversNearby = errors.New("no drivers nearby")
)

// Helper functions
func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371 // km

	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	deltaLat := (lat2 - lat1) * math.Pi / 180
	deltaLon := (lon2 - lon1) * math.Pi / 180

	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(deltaLon/2)*math.Sin(deltaLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadius * c
}

func estimatePrice(distance float64, rideType RideType, surgeMultiplier float64) float64 {
	baseRates := map[RideType]float64{
		RideTypeLyft:      2.00,
		RideTypeLyftXL:    3.50,
		RideTypeLyftLux:   5.00,
		RideTypeLyftBlack: 7.00,
	}

	perMileRates := map[RideType]float64{
		RideTypeLyft:      1.50,
		RideTypeLyftXL:    2.00,
		RideTypeLyftLux:   3.50,
		RideTypeLyftBlack: 4.50,
	}

	base := baseRates[rideType]
	perMile := perMileRates[rideType]

	return (base + (distance * perMile)) * surgeMultiplier
}

// Handlers
func getNearbyDrivers(c *fiber.Ctx) error {
	lat := c.QueryFloat("latitude", 0)
	lon := c.QueryFloat("longitude", 0)

	if lat == 0 || lon == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "latitude and longitude are required",
		})
	}

	var nearbyDrivers []Driver
	maxDistance := 5.0 // 5km radius

	db.mu.RLock()
	for _, driver := range db.Drivers {
		if driver.Status != "available" {
			continue
		}

		distance := calculateDistance(lat, lon,
			driver.Location.Latitude,
			driver.Location.Longitude)

		if distance <= maxDistance {
			nearbyDrivers = append(nearbyDrivers, driver)
		}
	}
	db.mu.RUnlock()

	return c.JSON(nearbyDrivers)
}

type RideEstimateRequest struct {
	Pickup   Location `json:"pickup"`
	Dropoff  Location `json:"dropoff"`
	RideType RideType `json:"ride_type"`
}

func getRideEstimate(c *fiber.Ctx) error {
	var req RideEstimateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	distance := calculateDistance(
		req.Pickup.Latitude,
		req.Pickup.Longitude,
		req.Dropoff.Latitude,
		req.Dropoff.Longitude,
	)

	// Calculate surge multiplier based on time of day and available drivers
	surgeMultiplier := 1.0
	hour := time.Now().Hour()
	if (hour >= 16 && hour <= 19) || (hour >= 22 || hour <= 2) {
		surgeMultiplier = 1.5
	}

	price := estimatePrice(distance, req.RideType, surgeMultiplier)
	estimatedDuration := int(distance * 3) // Rough estimate: 3 minutes per km

	return c.JSON(fiber.Map{
		"ride_type":          req.RideType,
		"estimated_price":    price,
		"estimated_duration": estimatedDuration,
		"estimated_distance": distance,
		"surge_multiplier":   surgeMultiplier,
	})
}

type RideRequest struct {
	UserEmail     string   `json:"user_email"`
	Pickup        Location `json:"pickup"`
	Dropoff       Location `json:"dropoff"`
	RideType      RideType `json:"ride_type"`
	PaymentMethod string   `json:"payment_method_id"`
}

func requestRide(c *fiber.Ctx) error {
	var req RideRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user
	user, exists := db.Users[req.UserEmail]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// Validate payment method
	validPayment := false
	for _, pm := range user.PaymentMethods {
		if pm.ID == req.PaymentMethod {

			validPayment = true
			break
		}
	}
	if !validPayment {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid payment method",
		})
	}

	// Find nearby driver
	var selectedDriver *Driver
	minDistance := math.MaxFloat64

	db.mu.RLock()
	for _, driver := range db.Drivers {
		if driver.Status != "available" {
			continue
		}

		distance := calculateDistance(
			req.Pickup.Latitude,
			req.Pickup.Longitude,
			driver.Location.Latitude,
			driver.Location.Longitude,
		)

		if distance < minDistance {
			minDistance = distance
			driverCopy := driver
			selectedDriver = &driverCopy
		}
	}
	db.mu.RUnlock()

	if selectedDriver == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "No drivers available",
		})
	}

	// Calculate price
	distance := calculateDistance(
		req.Pickup.Latitude,
		req.Pickup.Longitude,
		req.Dropoff.Latitude,
		req.Dropoff.Longitude,
	)

	surgeMultiplier := 1.0
	hour := time.Now().Hour()
	if (hour >= 16 && hour <= 19) || (hour >= 22 || hour <= 2) {
		surgeMultiplier = 1.5
	}

	price := estimatePrice(distance, req.RideType, surgeMultiplier)

	// Create ride
	ride := Ride{
		ID:        uuid.New().String(),
		UserEmail: req.UserEmail,
		Driver:    selectedDriver,
		Pickup:    req.Pickup,
		Dropoff:   req.Dropoff,
		Status:    RideStatusRequested,
		RideType:  req.RideType,
		Price:     price,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		PaymentID: req.PaymentMethod,
	}

	// Update database
	db.mu.Lock()
	db.Rides[ride.ID] = ride
	selectedDriver.Status = "busy"
	db.Drivers[selectedDriver.ID] = *selectedDriver
	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(ride)
}

func getRideStatus(c *fiber.Ctx) error {
	rideId := c.Params("rideId")
	if rideId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Ride ID is required",
		})
	}

	db.mu.RLock()
	ride, exists := db.Rides[rideId]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Ride not found",
		})
	}

	return c.JSON(ride)
}

func getUserRides(c *fiber.Ctx) error {
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

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Drivers: make(map[string]Driver),
		Rides:   make(map[string]Ride),
		Users:   make(map[string]User),
	}

	return json.Unmarshal(data, db)
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

	// Driver routes
	api.Get("/drivers/nearby", getNearbyDrivers)

	// Ride routes
	api.Post("/rides/estimate", getRideEstimate)
	api.Post("/rides", requestRide)
	api.Get("/rides", getUserRides)
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

	app.Use(logger.New())
	app.Use(recover.New())
	app.Use(cors.New())

	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

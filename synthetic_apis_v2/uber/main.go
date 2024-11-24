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

type Driver struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Phone        string    `json:"phone"`
	Rating       float64   `json:"rating"`
	CarModel     string    `json:"car_model"`
	CarColor     string    `json:"car_color"`
	LicensePlate string    `json:"license_plate"`
	Location     Location  `json:"location"`
	Available    bool      `json:"available"`
	LastUpdated  time.Time `json:"last_updated"`
}

type ServiceLevel string

const (
	UberX       ServiceLevel = "UberX"
	UberComfort ServiceLevel = "UberComfort"
	UberXL      ServiceLevel = "UberXL"
	UberBlack   ServiceLevel = "UberBlack"
)

type RideStatus string

const (
	RideStatusRequested  RideStatus = "requested"
	RideStatusAccepted   RideStatus = "accepted"
	RideStatusArriving   RideStatus = "arriving"
	RideStatusPickedUp   RideStatus = "picked_up"
	RideStatusInProgress RideStatus = "in_progress"
	RideStatusCompleted  RideStatus = "completed"
	RideStatusCancelled  RideStatus = "cancelled"
)

type Ride struct {
	ID             string       `json:"id"`
	UserEmail      string       `json:"user_email"`
	Driver         *Driver      `json:"driver,omitempty"`
	ServiceLevel   ServiceLevel `json:"service_level"`
	Status         RideStatus   `json:"status"`
	Pickup         Location     `json:"pickup"`
	Destination    Location     `json:"destination"`
	EstimatedPrice float64      `json:"estimated_price"`
	FinalPrice     float64      `json:"final_price,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
	PickupTime     *time.Time   `json:"pickup_time,omitempty"`
	DropoffTime    *time.Time   `json:"dropoff_time,omitempty"`
	PaymentMethod  string       `json:"payment_method"`
}

type RideEstimate struct {
	ServiceLevel      ServiceLevel `json:"service_level"`
	EstimatedPrice    float64      `json:"estimated_price"`
	EstimatedDuration int          `json:"estimated_duration"` // in minutes
	EstimatedDistance float64      `json:"estimated_distance"` // in miles
}

// Database represents our in-memory database
type Database struct {
	Users   map[string]User   `json:"users"`
	Drivers map[string]Driver `json:"drivers"`
	Rides   map[string]Ride   `json:"rides"`
	mu      sync.RWMutex
}

var (
	db                 *Database
	ErrUserNotFound    = errors.New("user not found")
	ErrDriverNotFound  = errors.New("driver not found")
	ErrRideNotFound    = errors.New("ride not found")
	ErrInvalidInput    = errors.New("invalid input")
	ErrNoDriversNearby = errors.New("no drivers nearby")
)

// Helper functions
func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 3959.0 // miles

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

func estimatePrice(distance float64, serviceLevel ServiceLevel) float64 {
	basePrices := map[ServiceLevel]float64{
		UberX:       2.50,
		UberComfort: 3.50,
		UberXL:      5.00,
		UberBlack:   7.00,
	}

	perMilePrices := map[ServiceLevel]float64{
		UberX:       1.50,
		UberComfort: 2.00,
		UberXL:      2.50,
		UberBlack:   3.50,
	}

	basePrice := basePrices[serviceLevel]
	perMile := perMilePrices[serviceLevel]

	return basePrice + (distance * perMile)
}

func findNearbyDriver(pickup Location, serviceLevel ServiceLevel) (*Driver, error) {
	const maxDistance = 5.0 // miles
	var nearestDriver *Driver
	minDistance := math.MaxFloat64

	db.mu.RLock()
	defer db.mu.RUnlock()

	for _, driver := range db.Drivers {
		if !driver.Available {
			continue
		}

		distance := calculateDistance(
			pickup.Latitude, pickup.Longitude,
			driver.Location.Latitude, driver.Location.Longitude,
		)

		if distance <= maxDistance && distance < minDistance {
			driverCopy := driver
			nearestDriver = &driverCopy
			minDistance = distance
		}
	}

	if nearestDriver == nil {
		return nil, ErrNoDriversNearby
	}

	return nearestDriver, nil
}

// HTTP Handlers
func getRideEstimates(c *fiber.Ctx) error {
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
		req.Pickup.Latitude, req.Pickup.Longitude,
		req.Destination.Latitude, req.Destination.Longitude,
	)

	estimates := []RideEstimate{
		{
			ServiceLevel:      UberX,
			EstimatedPrice:    estimatePrice(distance, UberX),
			EstimatedDuration: int(distance * 3), // Assuming 20mph average speed
			EstimatedDistance: distance,
		},
		{
			ServiceLevel:      UberComfort,
			EstimatedPrice:    estimatePrice(distance, UberComfort),
			EstimatedDuration: int(distance * 3),
			EstimatedDistance: distance,
		},
		{
			ServiceLevel:      UberXL,
			EstimatedPrice:    estimatePrice(distance, UberXL),
			EstimatedDuration: int(distance * 3),
			EstimatedDistance: distance,
		},
		{
			ServiceLevel:      UberBlack,
			EstimatedPrice:    estimatePrice(distance, UberBlack),
			EstimatedDuration: int(distance * 3),
			EstimatedDistance: distance,
		},
	}

	return c.JSON(estimates)
}

func requestRide(c *fiber.Ctx) error {
	var req struct {
		UserEmail     string       `json:"user_email"`
		ServiceLevel  ServiceLevel `json:"service_level"`
		Pickup        Location     `json:"pickup"`
		Destination   Location     `json:"destination"`
		PaymentMethod string       `json:"payment_method"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user and payment method
	db.mu.RLock()
	user, exists := db.Users[req.UserEmail]
	db.mu.RUnlock()
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

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
	driver, err := findNearbyDriver(req.Pickup, req.ServiceLevel)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "No drivers available nearby",
		})
	}

	// Calculate estimated price
	distance := calculateDistance(
		req.Pickup.Latitude, req.Pickup.Longitude,
		req.Destination.Latitude, req.Destination.Longitude,
	)
	estimatedPrice := estimatePrice(distance, req.ServiceLevel)

	// Create ride
	ride := Ride{
		ID:             uuid.New().String(),
		UserEmail:      req.UserEmail,
		Driver:         driver,
		ServiceLevel:   req.ServiceLevel,
		Status:         RideStatusRequested,
		Pickup:         req.Pickup,
		Destination:    req.Destination,
		EstimatedPrice: estimatedPrice,
		CreatedAt:      time.Now(),
		PaymentMethod:  req.PaymentMethod,
	}

	// Save ride
	db.mu.Lock()
	db.Rides[ride.ID] = ride
	// Update driver availability
	driver.Available = false
	db.Drivers[driver.ID] = *driver
	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(ride)
}

func getRideStatus(c *fiber.Ctx) error {
	rideId := c.Params("rideId")

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
			"error": "Email parameter is required",
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

func cancelRide(c *fiber.Ctx) error {
	rideId := c.Params("rideId")

	db.mu.Lock()
	defer db.mu.Unlock()

	ride, exists := db.Rides[rideId]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Ride not found",
		})
	}

	if ride.Status != RideStatusRequested && ride.Status != RideStatusAccepted {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot cancel ride in current status",
		})
	}

	// Update ride status
	ride.Status = RideStatusCancelled
	db.Rides[rideId] = ride

	// Make driver available again
	if ride.Driver != nil {
		driver := ride.Driver
		driver.Available = true
		db.Drivers[driver.ID] = *driver
	}

	return c.SendStatus(fiber.StatusOK)
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

	// Ride routes
	api.Post("/rides/estimate", getRideEstimates)
	api.Post("/rides", requestRide)
	api.Get("/rides", getUserRides)
	api.Get("/rides/:rideId", getRideStatus)
	api.Delete("/rides/:rideId", cancelRide)
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

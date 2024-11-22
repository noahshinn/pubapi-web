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
	"github.com/google/uuid"
)

// Domain Models
type Location struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
	City    string `json:"city"`
	State   string `json:"state"`
	ZIP     string `json:"zip"`
	Hours   string `json:"hours"`
}

type Vehicle struct {
	ID        string   `json:"id"`
	Make      string   `json:"make"`
	Model     string   `json:"model"`
	Year      int      `json:"year"`
	Category  string   `json:"category"`
	DailyRate float64  `json:"daily_rate"`
	Features  []string `json:"features"`
}

type User struct {
	Email           string    `json:"email"`
	Name            string    `json:"name"`
	Phone           string    `json:"phone"`
	DriversLicense  string    `json:"drivers_license"`
	LicenseState    string    `json:"license_state"`
	PaymentMethods  []Payment `json:"payment_methods"`
	InsurancePolicy string    `json:"insurance_policy"`
}

type Payment struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Last4    string `json:"last4"`
	ExpiryMM int    `json:"expiry_mm"`
	ExpiryYY int    `json:"expiry_yy"`
}

type ReservationStatus string

const (
	StatusPending   ReservationStatus = "pending"
	StatusConfirmed ReservationStatus = "confirmed"
	StatusActive    ReservationStatus = "active"
	StatusCompleted ReservationStatus = "completed"
	StatusCancelled ReservationStatus = "cancelled"
)

type Reservation struct {
	ID             string            `json:"id"`
	UserEmail      string            `json:"user_email"`
	Vehicle        Vehicle           `json:"vehicle"`
	PickupLocation Location          `json:"pickup_location"`
	ReturnLocation Location          `json:"return_location"`
	PickupDate     time.Time         `json:"pickup_date"`
	ReturnDate     time.Time         `json:"return_date"`
	Status         ReservationStatus `json:"status"`
	TotalCost      float64           `json:"total_cost"`
	PaymentMethod  string            `json:"payment_method"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

// Database represents our in-memory database
type Database struct {
	Users        map[string]User        `json:"users"`
	Vehicles     map[string]Vehicle     `json:"vehicles"`
	Locations    map[string]Location    `json:"locations"`
	Reservations map[string]Reservation `json:"reservations"`
	mu           sync.RWMutex
}

// Custom errors
var (
	ErrUserNotFound        = errors.New("user not found")
	ErrVehicleNotFound     = errors.New("vehicle not found")
	ErrLocationNotFound    = errors.New("location not found")
	ErrReservationNotFound = errors.New("reservation not found")
	ErrVehicleUnavailable  = errors.New("vehicle unavailable for selected dates")
)

var db *Database

// Database operations
func (d *Database) GetUser(email string) (User, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	user, exists := d.Users[email]
	if !exists {
		return User{}, ErrUserNotFound
	}
	return user, nil
}

func (d *Database) GetVehicle(id string) (Vehicle, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	vehicle, exists := d.Vehicles[id]
	if !exists {
		return Vehicle{}, ErrVehicleNotFound
	}
	return vehicle, nil
}

func (d *Database) GetLocation(id string) (Location, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	location, exists := d.Locations[id]
	if !exists {
		return Location{}, ErrLocationNotFound
	}
	return location, nil
}

func (d *Database) CreateReservation(res Reservation) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Check if vehicle is available for the requested dates
	if !d.isVehicleAvailable(res.Vehicle.ID, res.PickupDate, res.ReturnDate) {
		return ErrVehicleUnavailable
	}

	d.Reservations[res.ID] = res
	return nil
}

func (d *Database) isVehicleAvailable(vehicleID string, start, end time.Time) bool {
	for _, res := range d.Reservations {
		if res.Vehicle.ID == vehicleID &&
			res.Status != StatusCancelled &&
			res.Status != StatusCompleted {
			// Check for date overlap
			if !(end.Before(res.PickupDate) || start.After(res.ReturnDate)) {
				return false
			}
		}
	}
	return true
}

// HTTP Handlers
func getAvailableVehicles(c *fiber.Ctx) error {
	location := c.Query("location")
	pickupDate := c.Query("pickup_date")
	returnDate := c.Query("return_date")

	if location == "" || pickupDate == "" || returnDate == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "location, pickup_date, and return_date are required",
		})
	}

	start, err := time.Parse(time.RFC3339, pickupDate)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid pickup_date format",
		})
	}

	end, err := time.Parse(time.RFC3339, returnDate)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid return_date format",
		})
	}

	var availableVehicles []Vehicle

	db.mu.RLock()
	for _, vehicle := range db.Vehicles {
		if db.isVehicleAvailable(vehicle.ID, start, end) {
			availableVehicles = append(availableVehicles, vehicle)
		}
	}
	db.mu.RUnlock()

	return c.JSON(availableVehicles)
}

func getUserReservations(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	var userReservations []Reservation
	db.mu.RLock()
	for _, res := range db.Reservations {
		if res.UserEmail == email {
			userReservations = append(userReservations, res)
		}
	}
	db.mu.RUnlock()

	return c.JSON(userReservations)
}

type CreateReservationRequest struct {
	UserEmail        string    `json:"user_email"`
	VehicleID        string    `json:"vehicle_id"`
	PickupLocationID string    `json:"pickup_location_id"`
	ReturnLocationID string    `json:"return_location_id"`
	PickupDate       time.Time `json:"pickup_date"`
	ReturnDate       time.Time `json:"return_date"`
	PaymentMethod    string    `json:"payment_method"`
}

func createReservation(c *fiber.Ctx) error {
	var req CreateReservationRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user
	user, err := db.GetUser(req.UserEmail)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Validate vehicle
	vehicle, err := db.GetVehicle(req.VehicleID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Validate locations
	pickupLocation, err := db.GetLocation(req.PickupLocationID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Invalid pickup location",
		})
	}

	returnLocation, err := db.GetLocation(req.ReturnLocationID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Invalid return location",
		})
	}

	// Calculate rental duration and total cost
	days := int(req.ReturnDate.Sub(req.PickupDate).Hours() / 24)
	if days < 1 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Minimum rental period is 1 day",
		})
	}

	totalCost := vehicle.DailyRate * float64(days)

	// Create reservation
	reservation := Reservation{
		ID:             uuid.New().String(),
		UserEmail:      req.UserEmail,
		Vehicle:        vehicle,
		PickupLocation: pickupLocation,
		ReturnLocation: returnLocation,
		PickupDate:     req.PickupDate,

		ReturnDate:    req.ReturnDate,
		Status:        StatusPending,
		TotalCost:     totalCost,
		PaymentMethod: req.PaymentMethod,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
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

	// Save reservation
	if err := db.CreateReservation(reservation); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(reservation)
}

func getLocations(c *fiber.Ctx) error {
	city := c.Query("city")
	state := c.Query("state")

	var filteredLocations []Location
	db.mu.RLock()
	for _, location := range db.Locations {
		if (city == "" || location.City == city) &&
			(state == "" || location.State == state) {
			filteredLocations = append(filteredLocations, location)
		}
	}
	db.mu.RUnlock()

	return c.JSON(filteredLocations)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:        make(map[string]User),
		Vehicles:     make(map[string]Vehicle),
		Locations:    make(map[string]Location),
		Reservations: make(map[string]Reservation),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Vehicle routes
	api.Get("/vehicles", getAvailableVehicles)

	// Reservation routes
	api.Get("/reservations", getUserReservations)
	api.Post("/reservations", createReservation)

	// Location routes
	api.Get("/locations", getLocations)
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
	app.Use(cors.New())

	// Setup routes
	setupRoutes(app)

	// Start server
	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

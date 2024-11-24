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
	"github.com/google/uuid"
)

// Models
type Location struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Address string            `json:"address"`
	City    string            `json:"city"`
	State   string            `json:"state"`
	ZipCode string            `json:"zip_code"`
	Phone   string            `json:"phone"`
	Hours   map[string]string `json:"hours"`
}

type Vehicle struct {
	ID           string   `json:"id"`
	Make         string   `json:"make"`
	Model        string   `json:"model"`
	Year         int      `json:"year"`
	Class        string   `json:"class"`
	Seats        int      `json:"seats"`
	Transmission string   `json:"transmission"`
	FuelType     string   `json:"fuel_type"`
	DailyRate    float64  `json:"daily_rate"`
	Features     []string `json:"features"`
}

type Reservation struct {
	ID               string    `json:"id"`
	UserEmail        string    `json:"user_email"`
	Vehicle          Vehicle   `json:"vehicle"`
	PickupLocation   Location  `json:"pickup_location"`
	ReturnLocation   Location  `json:"return_location"`
	PickupDate       time.Time `json:"pickup_date"`
	ReturnDate       time.Time `json:"return_date"`
	Status           string    `json:"status"`
	TotalCost        float64   `json:"total_cost"`
	InsuranceOption  string    `json:"insurance_option"`
	AdditionalDriver int       `json:"additional_drivers"`
	CreatedAt        time.Time `json:"created_at"`
}

type Database struct {
	Vehicles     map[string]Vehicle     `json:"vehicles"`
	Locations    map[string]Location    `json:"locations"`
	Reservations map[string]Reservation `json:"reservations"`
	mu           sync.RWMutex
}

var db *Database

// Handlers
func searchVehicles(c *fiber.Ctx) error {
	location := c.Query("location")
	pickupDate := c.Query("pickup_date")
	returnDate := c.Query("return_date")
	vehicleClass := c.Query("vehicle_class")

	if location == "" || pickupDate == "" || returnDate == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Missing required parameters",
		})
	}

	pickup, err := time.Parse(time.RFC3339, pickupDate)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid pickup date format",
		})
	}

	returnTime, err := time.Parse(time.RFC3339, returnDate)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid return date format",
		})
	}

	if pickup.After(returnTime) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Pickup date must be before return date",
		})
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	var availableVehicles []Vehicle
	for _, vehicle := range db.Vehicles {
		if vehicleClass != "" && vehicle.Class != vehicleClass {
			continue
		}

		// Check if vehicle is already reserved for the requested dates
		isAvailable := true
		for _, reservation := range db.Reservations {
			if reservation.Vehicle.ID == vehicle.ID {
				if (pickup.After(reservation.PickupDate) && pickup.Before(reservation.ReturnDate)) ||
					(returnTime.After(reservation.PickupDate) && returnTime.Before(reservation.ReturnDate)) {
					isAvailable = false
					break
				}
			}
		}

		if isAvailable {
			availableVehicles = append(availableVehicles, vehicle)
		}
	}

	return c.JSON(availableVehicles)
}

func getLocations(c *fiber.Ctx) error {
	zipCode := c.Query("zip_code")
	city := c.Query("city")

	db.mu.RLock()
	defer db.mu.RUnlock()

	var filteredLocations []Location
	for _, location := range db.Locations {
		if (zipCode != "" && location.ZipCode == zipCode) ||
			(city != "" && location.City == city) ||
			(zipCode == "" && city == "") {
			filteredLocations = append(filteredLocations, location)
		}
	}

	return c.JSON(filteredLocations)
}

func getUserReservations(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email is required",
		})
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	var userReservations []Reservation
	for _, reservation := range db.Reservations {
		if reservation.UserEmail == email {
			userReservations = append(userReservations, reservation)
		}
	}

	return c.JSON(userReservations)
}

type NewReservationRequest struct {
	UserEmail        string    `json:"user_email"`
	VehicleID        string    `json:"vehicle_id"`
	PickupLocationID string    `json:"pickup_location_id"`
	ReturnLocationID string    `json:"return_location_id"`
	PickupDate       time.Time `json:"pickup_date"`
	ReturnDate       time.Time `json:"return_date"`
	InsuranceOption  string    `json:"insurance_option"`
	AdditionalDriver int       `json:"additional_drivers"`
}

func createReservation(c *fiber.Ctx) error {
	var req NewReservationRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	// Validate vehicle
	vehicle, exists := db.Vehicles[req.VehicleID]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Vehicle not found",
		})
	}

	// Validate locations
	pickupLocation, exists := db.Locations[req.PickupLocationID]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Pickup location not found",
		})
	}

	returnLocation, exists := db.Locations[req.ReturnLocationID]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Return location not found",
		})
	}

	// Calculate rental duration and total cost
	duration := req.ReturnDate.Sub(req.PickupDate)
	days := int(duration.Hours() / 24)
	if duration.Hours() > float64(days*24) {
		days++ // Partial day counts as full day
	}

	totalCost := vehicle.DailyRate * float64(days)
	if req.InsuranceOption != "" {
		totalCost += float64(days) * 15.0 // Basic insurance rate
	}
	if req.AdditionalDriver > 0 {
		totalCost += float64(req.AdditionalDriver) * float64(days) * 10.0
	}

	reservation := Reservation{
		ID:               uuid.New().String(),
		UserEmail:        req.UserEmail,
		Vehicle:          vehicle,
		PickupLocation:   pickupLocation,
		ReturnLocation:   returnLocation,
		PickupDate:       req.PickupDate,
		ReturnDate:       req.ReturnDate,
		Status:           "confirmed",
		TotalCost:        totalCost,
		InsuranceOption:  req.InsuranceOption,
		AdditionalDriver: req.AdditionalDriver,
		CreatedAt:        time.Now(),
	}

	db.Reservations[reservation.ID] = reservation

	return c.Status(fiber.StatusCreated).JSON(reservation)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Vehicles:     make(map[string]Vehicle),
		Locations:    make(map[string]Location),
		Reservations: make(map[string]Reservation),
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

	// Vehicle routes
	api.Get("/vehicles", searchVehicles)

	// Location routes
	api.Get("/locations", getLocations)

	// Reservation routes
	api.Get("/reservations", getUserReservations)
	api.Post("/reservations", createReservation)
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

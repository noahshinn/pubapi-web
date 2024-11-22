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
	Street    string  `json:"street"`
	City      string  `json:"city"`
	State     string  `json:"state"`
	ZipCode   string  `json:"zip_code"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type User struct {
	Email          string    `json:"email"`
	Name           string    `json:"name"`
	Phone          string    `json:"phone"`
	Address        Address   `json:"address"`
	DateOfBirth    string    `json:"date_of_birth"`
	InsuranceCard  string    `json:"insurance_card"`
	PreferredStore string    `json:"preferred_store"`
	CreatedAt      time.Time `json:"created_at"`
}

type Prescription struct {
	ID               string    `json:"id"`
	UserEmail        string    `json:"user_email"`
	Name             string    `json:"name"`
	Dosage           string    `json:"dosage"`
	Quantity         int       `json:"quantity"`
	RefillsRemaining int       `json:"refills_remaining"`
	LastFilled       time.Time `json:"last_filled"`
	Expires          time.Time `json:"expires"`
	Status           string    `json:"status"`
	Prescriber       string    `json:"prescriber"`
	PharmacyNotes    string    `json:"pharmacy_notes"`
}

type Store struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Address     Address  `json:"address"`
	Phone       string   `json:"phone"`
	Hours       string   `json:"hours"`
	HasPharmacy bool     `json:"has_pharmacy"`
	HasClinic   bool     `json:"has_clinic"`
	Services    []string `json:"services"`
}

type AppointmentType string

const (
	AppointmentVaccination AppointmentType = "vaccination"
	AppointmentClinic      AppointmentType = "clinic"
	AppointmentConsult     AppointmentType = "consultation"
)

type Appointment struct {
	ID        string          `json:"id"`
	UserEmail string          `json:"user_email"`
	Type      AppointmentType `json:"type"`
	DateTime  time.Time       `json:"datetime"`
	StoreID   string          `json:"store_id"`
	Status    string          `json:"status"`
	Notes     string          `json:"notes"`
	CreatedAt time.Time       `json:"created_at"`
}

type RefillRequest struct {
	ID             string    `json:"id"`
	PrescriptionID string    `json:"prescription_id"`
	UserEmail      string    `json:"user_email"`
	StoreID        string    `json:"pickup_store"`
	PreferredDate  time.Time `json:"preferred_date"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
}

// Database represents our in-memory database
type Database struct {
	Users          map[string]User          `json:"users"`
	Prescriptions  map[string]Prescription  `json:"prescriptions"`
	Stores         map[string]Store         `json:"stores"`
	Appointments   map[string]Appointment   `json:"appointments"`
	RefillRequests map[string]RefillRequest `json:"refill_requests"`
	mu             sync.RWMutex
}

var (
	ErrUserNotFound         = errors.New("user not found")
	ErrPrescriptionNotFound = errors.New("prescription not found")
	ErrStoreNotFound        = errors.New("store not found")
	ErrInvalidInput         = errors.New("invalid input")
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

func (d *Database) GetUserPrescriptions(email string) []Prescription {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var prescriptions []Prescription
	for _, p := range d.Prescriptions {
		if p.UserEmail == email {
			prescriptions = append(prescriptions, p)
		}
	}
	return prescriptions
}

func (d *Database) CreateRefillRequest(req RefillRequest) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.RefillRequests[req.ID] = req
	return nil
}

func (d *Database) GetNearbyStores(lat, lon float64, radius float64) []Store {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var stores []Store
	for _, store := range d.Stores {
		distance := calculateDistance(lat, lon,
			store.Address.Latitude,
			store.Address.Longitude)
		if distance <= radius {
			stores = append(stores, store)
		}
	}
	return stores
}

// HTTP Handlers
func getPrescriptions(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	// Verify user exists
	if _, err := db.GetUser(email); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	prescriptions := db.GetUserPrescriptions(email)
	return c.JSON(prescriptions)
}

func requestRefill(c *fiber.Ctx) error {
	var req struct {
		PrescriptionID string    `json:"prescription_id"`
		UserEmail      string    `json:"user_email"`
		StoreID        string    `json:"pickup_store"`
		PreferredDate  time.Time `json:"preferred_date"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user and prescription
	if _, err := db.GetUser(req.UserEmail); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	refillRequest := RefillRequest{
		ID:             uuid.New().String(),
		PrescriptionID: req.PrescriptionID,
		UserEmail:      req.UserEmail,
		StoreID:        req.StoreID,
		PreferredDate:  req.PreferredDate,
		Status:         "pending",
		CreatedAt:      time.Now(),
	}

	if err := db.CreateRefillRequest(refillRequest); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create refill request",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(refillRequest)
}

func getNearbyStores(c *fiber.Ctx) error {
	lat := c.QueryFloat("latitude", 0)
	lon := c.QueryFloat("longitude", 0)

	if lat == 0 || lon == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "latitude and longitude are required",
		})
	}

	radius := 10.0 // 10 mile radius
	stores := db.GetNearbyStores(lat, lon, radius)
	return c.JSON(stores)
}

func scheduleAppointment(c *fiber.Ctx) error {
	var req struct {
		UserEmail         string          `json:"user_email"`
		Type              AppointmentType `json:"type"`
		PreferredDateTime time.Time       `json:"preferred_datetime"`
		StoreID           string          `json:"store_id"`
		Notes             string          `json:"notes"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user
	if _, err := db.GetUser(req.UserEmail); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	appointment := Appointment{
		ID:        uuid.New().String(),
		UserEmail: req.UserEmail,
		Type:      req.Type,
		DateTime:  req.PreferredDateTime,
		StoreID:   req.StoreID,
		Status:    "scheduled",
		Notes:     req.Notes,
		CreatedAt: time.Now(),
	}

	db.mu.Lock()
	db.Appointments[appointment.ID] = appointment
	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(appointment)
}

func getUserAppointments(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	// Verify user exists
	if _, err := db.GetUser(email); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	var appointments []Appointment
	db.mu.RLock()
	for _, apt := range db.Appointments {
		if apt.UserEmail == email {
			appointments = append(appointments, apt)
		}
	}
	db.mu.RUnlock()

	return c.JSON(appointments)
}

func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	// Simplified distance calculation
	return ((lat2 - lat1) * (lat2 - lat1)) + ((lon2 - lon1) * (lon2 - lon1))
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:          make(map[string]User),
		Prescriptions:  make(map[string]Prescription),
		Stores:         make(map[string]Store),
		Appointments:   make(map[string]Appointment),
		RefillRequests: make(map[string]RefillRequest),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Prescription routes
	api.Get("/prescriptions", getPrescriptions)
	api.Post("/prescriptions/refill", requestRefill)

	// Store routes
	api.Get("/stores", getNearbyStores)

	// Appointment routes
	api.Get("/appointments", getUserAppointments)
	api.Post("/appointments", scheduleAppointment)
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
	app.Use(cors.New())

	// Setup routes
	setupRoutes(app)

	// Start server
	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

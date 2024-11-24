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
	DateOfBirth    string    `json:"date_of_birth"`
	Address        Address   `json:"address"`
	InsuranceInfo  Insurance `json:"insurance_info"`
	PreferredStore string    `json:"preferred_store"`
}

type Insurance struct {
	Provider     string `json:"provider"`
	PolicyNumber string `json:"policy_number"`
	GroupNumber  string `json:"group_number"`
	BinNumber    string `json:"bin_number"`
}

type Prescription struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Dosage           string    `json:"dosage"`
	Quantity         int       `json:"quantity"`
	RefillsRemaining int       `json:"refills_remaining"`
	LastFilled       time.Time `json:"last_filled"`
	ExpiresAt        time.Time `json:"expires_at"`
	Prescriber       string    `json:"prescriber"`
	Status           string    `json:"status"`
	UserEmail        string    `json:"user_email"`
}

type Store struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Address         Address `json:"address"`
	Phone           string  `json:"phone"`
	Hours           string  `json:"hours"`
	HasPharmacy     bool    `json:"has_pharmacy"`
	HasMinuteClinic bool    `json:"has_minute_clinic"`
}

type Appointment struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	DateTime  time.Time `json:"datetime"`
	StoreID   string    `json:"store_id"`
	UserEmail string    `json:"user_email"`
	Status    string    `json:"status"`
	Notes     string    `json:"notes"`
}

type RefillRequest struct {
	ID             string    `json:"id"`
	PrescriptionID string    `json:"prescription_id"`
	UserEmail      string    `json:"user_email"`
	Status         string    `json:"status"`
	RequestedAt    time.Time `json:"requested_at"`
	PickupStore    string    `json:"pickup_store"`
	EstimatedReady time.Time `json:"estimated_ready"`
}

type Database struct {
	Users          map[string]User          `json:"users"`
	Prescriptions  map[string]Prescription  `json:"prescriptions"`
	Stores         map[string]Store         `json:"stores"`
	Appointments   map[string]Appointment   `json:"appointments"`
	RefillRequests map[string]RefillRequest `json:"refill_requests"`
	mu             sync.RWMutex
}

var db *Database

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

func getUserPrescriptions(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	var userPrescriptions []Prescription
	for _, prescription := range db.Prescriptions {
		if prescription.UserEmail == email {
			userPrescriptions = append(userPrescriptions, prescription)
		}
	}

	return c.JSON(userPrescriptions)
}

func requestRefill(c *fiber.Ctx) error {
	prescriptionId := c.Params("prescriptionId")

	db.mu.RLock()
	prescription, exists := db.Prescriptions[prescriptionId]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Prescription not found",
		})
	}

	if prescription.RefillsRemaining <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "No refills remaining",
		})
	}

	refillRequest := RefillRequest{
		ID:             uuid.New().String(),
		PrescriptionID: prescriptionId,
		UserEmail:      prescription.UserEmail,
		Status:         "pending",
		RequestedAt:    time.Now(),
		EstimatedReady: time.Now().Add(24 * time.Hour),
	}

	db.mu.Lock()
	db.RefillRequests[refillRequest.ID] = refillRequest
	prescription.RefillsRemaining--
	db.Prescriptions[prescriptionId] = prescription
	db.mu.Unlock()

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

	db.mu.RLock()
	defer db.mu.RUnlock()

	var nearbyStores []Store
	maxDistance := 50.0 // Maximum radius in km

	for _, store := range db.Stores {
		distance := calculateDistance(lat, lon,
			store.Address.Latitude,
			store.Address.Longitude)

		if distance <= maxDistance {
			nearbyStores = append(nearbyStores, store)
		}
	}

	return c.JSON(nearbyStores)
}

func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	// Simplified distance calculation
	return ((lat2 - lat1) * (lat2 - lat1)) + ((lon2 - lon1) * (lon2 - lon1))
}

func getUserAppointments(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	var userAppointments []Appointment
	for _, appointment := range db.Appointments {
		if appointment.UserEmail == email {
			userAppointments = append(userAppointments, appointment)
		}
	}

	return c.JSON(userAppointments)
}

type NewAppointmentRequest struct {
	Type      string    `json:"type"`
	DateTime  time.Time `json:"datetime"`
	StoreID   string    `json:"store_id"`
	UserEmail string    `json:"user_email"`
	Notes     string    `json:"notes"`
}

func createAppointment(c *fiber.Ctx) error {
	var req NewAppointmentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.RLock()
	_, userExists := db.Users[req.UserEmail]
	_, storeExists := db.Stores[req.StoreID]
	db.mu.RUnlock()

	if !userExists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	if !storeExists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Store not found",
		})
	}

	appointment := Appointment{
		ID:        uuid.New().String(),
		Type:      req.Type,
		DateTime:  req.DateTime,
		StoreID:   req.StoreID,
		UserEmail: req.UserEmail,
		Status:    "scheduled",
		Notes:     req.Notes,
	}

	db.mu.Lock()
	db.Appointments[appointment.ID] = appointment
	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(appointment)
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

	// Prescription routes
	api.Get("/prescriptions", getUserPrescriptions)
	api.Post("/prescriptions/:prescriptionId/refill", requestRefill)

	// Store routes
	api.Get("/stores", getNearbyStores)

	// Appointment routes
	api.Get("/appointments", getUserAppointments)
	api.Post("/appointments", createAppointment)
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

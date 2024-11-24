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

// Data models
type Address struct {
	Street    string  `json:"street"`
	City      string  `json:"city"`
	State     string  `json:"state"`
	ZipCode   string  `json:"zip_code"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type Store struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Address      Address `json:"address"`
	Phone        string  `json:"phone"`
	Hours        string  `json:"hours"`
	HasPharmacy  bool    `json:"has_pharmacy"`
	HasDriveThru bool    `json:"has_drive_thru"`
}

type Prescription struct {
	ID               string    `json:"id"`
	UserEmail        string    `json:"user_email"`
	Name             string    `json:"name"`
	Dosage           string    `json:"dosage"`
	Prescriber       string    `json:"prescriber"`
	RemainingRefills int       `json:"remaining_refills"`
	LastFilled       time.Time `json:"last_filled"`
	NextRefillDate   time.Time `json:"next_refill_date"`
	Status           string    `json:"status"`
}

type RefillRequest struct {
	ID             string    `json:"id"`
	PrescriptionID string    `json:"prescription_id"`
	UserEmail      string    `json:"user_email"`
	Status         string    `json:"status"`
	PickupStore    string    `json:"pickup_store"`
	RequestedDate  time.Time `json:"requested_date"`
	ReadyDate      time.Time `json:"ready_date"`
}

type Product struct {
	ID                   string  `json:"id"`
	Name                 string  `json:"name"`
	Description          string  `json:"description"`
	Price                float64 `json:"price"`
	Category             string  `json:"category"`
	InStock              bool    `json:"in_stock"`
	RequiresPrescription bool    `json:"requires_prescription"`
}

type User struct {
	Email         string    `json:"email"`
	Name          string    `json:"name"`
	Phone         string    `json:"phone"`
	DateOfBirth   string    `json:"date_of_birth"`
	Address       Address   `json:"address"`
	InsuranceInfo Insurance `json:"insurance_info"`
}

type Insurance struct {
	Provider     string `json:"provider"`
	PolicyNumber string `json:"policy_number"`
	GroupNumber  string `json:"group_number"`
}

// Database struct
type Database struct {
	Users          map[string]User          `json:"users"`
	Stores         map[string]Store         `json:"stores"`
	Prescriptions  map[string]Prescription  `json:"prescriptions"`
	RefillRequests map[string]RefillRequest `json:"refill_requests"`
	Products       map[string]Product       `json:"products"`
	mu             sync.RWMutex
}

var db *Database

// Helper functions
func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	// Simplified distance calculation
	return ((lat2 - lat1) * (lat2 - lat1)) + ((lon2 - lon1) * (lon2 - lon1))
}

// Handlers
func getPrescriptions(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	var userPrescriptions []Prescription
	db.mu.RLock()
	for _, prescription := range db.Prescriptions {
		if prescription.UserEmail == email {
			userPrescriptions = append(userPrescriptions, prescription)
		}
	}
	db.mu.RUnlock()

	return c.JSON(userPrescriptions)
}

func requestRefill(c *fiber.Ctx) error {
	prescriptionId := c.Params("prescriptionId")

	var req struct {
		UserEmail   string `json:"user_email"`
		PickupStore string `json:"pickup_store"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.RLock()
	prescription, exists := db.Prescriptions[prescriptionId]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Prescription not found",
		})
	}

	if prescription.RemainingRefills <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "No refills remaining",
		})
	}

	refillRequest := RefillRequest{
		ID:             uuid.New().String(),
		PrescriptionID: prescriptionId,
		UserEmail:      req.UserEmail,
		Status:         "pending",
		PickupStore:    req.PickupStore,
		RequestedDate:  time.Now(),
		ReadyDate:      time.Now().Add(24 * time.Hour),
	}

	db.mu.Lock()
	db.RefillRequests[refillRequest.ID] = refillRequest
	prescription.RemainingRefills--
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

	var nearbyStores []Store
	maxDistance := 50.0 // Maximum radius in km

	db.mu.RLock()
	for _, store := range db.Stores {
		distance := calculateDistance(lat, lon,
			store.Address.Latitude,
			store.Address.Longitude)

		if distance <= maxDistance {
			nearbyStores = append(nearbyStores, store)
		}
	}
	db.mu.RUnlock()

	return c.JSON(nearbyStores)
}

func searchProducts(c *fiber.Ctx) error {
	query := c.Query("query")
	category := c.Query("category")

	var matchingProducts []Product
	db.mu.RLock()
	for _, product := range db.Products {
		if (query == "" || contains(product.Name, query) || contains(product.Description, query)) &&
			(category == "" || product.Category == category) {
			matchingProducts = append(matchingProducts, product)
		}
	}
	db.mu.RUnlock()

	return c.JSON(matchingProducts)
}

func contains(s, substr string) bool {
	// Case-insensitive substring search could be implemented here
	return true // Simplified for this example
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:          make(map[string]User),
		Stores:         make(map[string]Store),
		Prescriptions:  make(map[string]Prescription),
		RefillRequests: make(map[string]RefillRequest),
		Products:       make(map[string]Product),
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

	// Prescription routes
	api.Get("/prescriptions", getPrescriptions)
	api.Post("/prescriptions/:prescriptionId/refill", requestRefill)

	// Store routes
	api.Get("/stores", getNearbyStores)

	// Product routes
	api.Get("/products", searchProducts)
}

func main() {
	port := flag.String("port", "3000", "Port to run the server on")
	flag.Parse()

	if err := loadDatabase(); err != nil {
		log.Fatal(err)
	}

	app := fiber.New()

	app.Use(logger.New())
	app.Use(cors.New())

	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

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
	Email          string  `json:"email"`
	Name           string  `json:"name"`
	Phone          string  `json:"phone"`
	Address        Address `json:"address"`
	DateOfBirth    string  `json:"date_of_birth"`
	InsuranceCard  string  `json:"insurance_card"`
	InsuranceGroup string  `json:"insurance_group"`
	PreferredStore string  `json:"preferred_store"`
}

type Prescription struct {
	ID               string    `json:"id"`
	UserEmail        string    `json:"user_email"`
	Name             string    `json:"name"`
	Doctor           string    `json:"doctor"`
	Dosage           string    `json:"dosage"`
	RefillsRemaining int       `json:"refills_remaining"`
	LastFilled       time.Time `json:"last_filled"`
	NextRefillDate   time.Time `json:"next_refill_date"`
	Status           string    `json:"status"`
	Instructions     string    `json:"instructions"`
}

type Store struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Address      Address `json:"address"`
	Phone        string  `json:"phone"`
	Hours        string  `json:"hours"`
	HasPharmacy  bool    `json:"has_pharmacy"`
	HasDriveThru bool    `json:"has_drive_thru"`
	HasPhoto     bool    `json:"has_photo"`
}

type Product struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Price       float64 `json:"price"`
	Category    string  `json:"category"`
	InStock     bool    `json:"in_stock"`
}

type OrderItem struct {
	ProductID string  `json:"product_id"`
	Name      string  `json:"name"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
}

type Order struct {
	ID        string      `json:"id"`
	UserEmail string      `json:"user_email"`
	StoreID   string      `json:"store_id"`
	Items     []OrderItem `json:"items"`
	Total     float64     `json:"total"`
	Status    string      `json:"status"`
	CreatedAt time.Time   `json:"created_at"`
}

// Database represents our in-memory database
type Database struct {
	Users         map[string]User         `json:"users"`
	Prescriptions map[string]Prescription `json:"prescriptions"`
	Stores        map[string]Store        `json:"stores"`
	Products      map[string]Product      `json:"products"`
	Orders        map[string]Order        `json:"orders"`
	mu            sync.RWMutex
}

var db *Database

// Database operations
func (d *Database) GetUser(email string) (User, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	user, exists := d.Users[email]
	if !exists {
		return User{}, errors.New("user not found")
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

func (d *Database) RequestRefill(prescriptionID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	prescription, exists := d.Prescriptions[prescriptionID]
	if !exists {
		return errors.New("prescription not found")
	}

	if prescription.RefillsRemaining <= 0 {
		return errors.New("no refills remaining")
	}

	prescription.RefillsRemaining--
	prescription.Status = "processing"
	prescription.LastFilled = time.Now()
	prescription.NextRefillDate = time.Now().AddDate(0, 1, 0)

	d.Prescriptions[prescriptionID] = prescription
	return nil
}

// HTTP Handlers
func getPrescriptions(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	prescriptions := db.GetUserPrescriptions(email)
	return c.JSON(prescriptions)
}

func requestRefill(c *fiber.Ctx) error {
	var req struct {
		PrescriptionID string `json:"prescription_id"`
		UserEmail      string `json:"user_email"`
		StoreID        string `json:"store_id"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if err := db.RequestRefill(req.PrescriptionID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Refill request processed successfully",
	})
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
	maxDistance := 20.0 // Maximum radius in km

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

func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	// Simplified distance calculation
	return ((lat2 - lat1) * (lat2 - lat1)) + ((lon2 - lon1) * (lon2 - lon1))
}

func placeOrder(c *fiber.Ctx) error {
	var req struct {
		UserEmail string      `json:"user_email"`
		StoreID   string      `json:"store_id"`
		Items     []OrderItem `json:"items"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Calculate total
	var total float64
	for _, item := range req.Items {
		total += item.Price * float64(item.Quantity)
	}

	order := Order{
		ID:        uuid.New().String(),
		UserEmail: req.UserEmail,
		StoreID:   req.StoreID,
		Items:     req.Items,
		Total:     total,
		Status:    "pending",
		CreatedAt: time.Now(),
	}

	db.mu.Lock()
	db.Orders[order.ID] = order
	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(order)
}

func getUserOrders(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	var userOrders []Order
	db.mu.RLock()
	for _, order := range db.Orders {
		if order.UserEmail == email {
			userOrders = append(userOrders, order)
		}
	}
	db.mu.RUnlock()

	return c.JSON(userOrders)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:         make(map[string]User),
		Prescriptions: make(map[string]Prescription),
		Stores:        make(map[string]Store),
		Products:      make(map[string]Product),
		Orders:        make(map[string]Order),
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
	api.Get("/stores/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		store, exists := db.Stores[id]
		if !exists {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Store not found",
			})
		}
		return c.JSON(store)
	})

	// Order routes
	api.Get("/orders", getUserOrders)
	api.Post("/orders", placeOrder)

	// User routes
	api.Get("/users/:email", func(c *fiber.Ctx) error {
		email := c.Params("email")
		user, err := db.GetUser(email)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.JSON(user)
	})
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

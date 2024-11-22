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

type Hours struct {
	Monday    string `json:"monday"`
	Tuesday   string `json:"tuesday"`
	Wednesday string `json:"wednesday"`
	Thursday  string `json:"thursday"`
	Friday    string `json:"friday"`
	Saturday  string `json:"saturday"`
	Sunday    string `json:"sunday"`
}

type Location struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Address   Address  `json:"address"`
	Phone     string   `json:"phone"`
	Hours     Hours    `json:"hours"`
	Amenities []string `json:"amenities"`
}

type FitnessClass struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Instructor      string    `json:"instructor"`
	StartTime       time.Time `json:"start_time"`
	EndTime         time.Time `json:"end_time"`
	Capacity        int       `json:"capacity"`
	BookedCount     int       `json:"booked_count"`
	LocationID      string    `json:"location_id"`
	DifficultyLevel string    `json:"difficulty_level"`
}

type Booking struct {
	ID        string       `json:"id"`
	Class     FitnessClass `json:"class"`
	UserEmail string       `json:"user_email"`
	Status    string       `json:"status"`
	CreatedAt time.Time    `json:"created_at"`
}

type PaymentMethod struct {
	Type   string `json:"type"`
	Last4  string `json:"last4"`
	Expiry string `json:"expiry"`
}

type Membership struct {
	ID            string        `json:"id"`
	UserEmail     string        `json:"user_email"`
	Type          string        `json:"type"`
	Status        string        `json:"status"`
	StartDate     time.Time     `json:"start_date"`
	EndDate       time.Time     `json:"end_date"`
	HomeLocation  Location      `json:"home_location"`
	PaymentMethod PaymentMethod `json:"payment_method"`
}

type User struct {
	Email      string    `json:"email"`
	Name       string    `json:"name"`
	Phone      string    `json:"phone"`
	Address    Address   `json:"address"`
	Membership string    `json:"membership"`
	JoinDate   time.Time `json:"join_date"`
}

// Database represents our in-memory database
type Database struct {
	Users       map[string]User         `json:"users"`
	Locations   map[string]Location     `json:"locations"`
	Classes     map[string]FitnessClass `json:"classes"`
	Bookings    map[string]Booking      `json:"bookings"`
	Memberships map[string]Membership   `json:"memberships"`
	mu          sync.RWMutex
}

// Global database instance
var db *Database

// Error definitions
var (
	ErrUserNotFound      = errors.New("user not found")
	ErrLocationNotFound  = errors.New("location not found")
	ErrClassNotFound     = errors.New("class not found")
	ErrClassFull         = errors.New("class is full")
	ErrBookingNotFound   = errors.New("booking not found")
	ErrInvalidMembership = errors.New("invalid or expired membership")
)

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

func (d *Database) GetLocation(id string) (Location, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	location, exists := d.Locations[id]
	if !exists {
		return Location{}, ErrLocationNotFound
	}
	return location, nil
}

func (d *Database) GetClass(id string) (FitnessClass, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	class, exists := d.Classes[id]
	if !exists {
		return FitnessClass{}, ErrClassNotFound
	}
	return class, nil
}

func (d *Database) CreateBooking(booking Booking) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	class, exists := d.Classes[booking.Class.ID]
	if !exists {
		return ErrClassNotFound
	}

	if class.BookedCount >= class.Capacity {
		return ErrClassFull
	}

	class.BookedCount++
	d.Classes[class.ID] = class
	d.Bookings[booking.ID] = booking
	return nil
}

func (d *Database) GetMembership(email string) (Membership, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	for _, membership := range d.Memberships {
		if membership.UserEmail == email {
			return membership, nil
		}
	}
	return Membership{}, ErrInvalidMembership
}

// Handlers
func getClasses(c *fiber.Ctx) error {
	locationID := c.Query("location_id")
	if locationID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "location_id is required",
		})
	}

	dateStr := c.Query("date")
	var filterDate time.Time
	var err error
	if dateStr != "" {
		filterDate, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "invalid date format",
			})
		}
	}

	var classes []FitnessClass
	db.mu.RLock()
	for _, class := range db.Classes {
		if class.LocationID == locationID {
			if dateStr == "" || class.StartTime.Format("2006-01-02") == filterDate.Format("2006-01-02") {
				classes = append(classes, class)
			}
		}
	}
	db.mu.RUnlock()

	return c.JSON(classes)
}

func getBookings(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	var userBookings []Booking
	db.mu.RLock()
	for _, booking := range db.Bookings {
		if booking.UserEmail == email {
			userBookings = append(userBookings, booking)
		}
	}
	db.mu.RUnlock()

	return c.JSON(userBookings)
}

func createBooking(c *fiber.Ctx) error {
	var req struct {
		ClassID   string `json:"class_id"`
		UserEmail string `json:"user_email"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Verify user and membership
	membership, err := db.GetMembership(req.UserEmail)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if membership.Status != "active" || membership.EndDate.Before(time.Now()) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Membership is not active",
		})
	}

	// Get class details
	class, err := db.GetClass(req.ClassID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	booking := Booking{
		ID:        uuid.New().String(),
		Class:     class,
		UserEmail: req.UserEmail,
		Status:    "confirmed",
		CreatedAt: time.Now(),
	}

	if err := db.CreateBooking(booking); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(booking)
}

func getLocations(c *fiber.Ctx) error {
	lat := c.QueryFloat("latitude", 0)
	lon := c.QueryFloat("longitude", 0)

	if lat == 0 || lon == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "latitude and longitude are required",
		})
	}

	var nearbyLocations []Location
	maxDistance := 25.0 // Maximum radius in km

	db.mu.RLock()
	for _, location := range db.Locations {
		distance := calculateDistance(lat, lon,
			location.Address.Latitude,
			location.Address.Longitude)

		if distance <= maxDistance {
			nearbyLocations = append(nearbyLocations, location)
		}
	}
	db.mu.RUnlock()

	return c.JSON(nearbyLocations)
}

func getMembership(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	membership, err := db.GetMembership(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(membership)
}

// Helper functions
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
		Users:       make(map[string]User),
		Locations:   make(map[string]Location),
		Classes:     make(map[string]FitnessClass),
		Bookings:    make(map[string]Booking),
		Memberships: make(map[string]Membership),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	api.Get("/classes", getClasses)
	api.Get("/bookings", getBookings)
	api.Post("/bookings", createBooking)
	api.Get("/locations", getLocations)
	api.Get("/membership", getMembership)
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

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

type Hours struct {
	Weekday string `json:"weekday"`
	Weekend string `json:"weekend"`
}

type Location struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Address   string   `json:"address"`
	Phone     string   `json:"phone"`
	Hours     Hours    `json:"hours"`
	Amenities []string `json:"amenities"`
	Latitude  float64  `json:"latitude"`
	Longitude float64  `json:"longitude"`
}

type FitnessClass struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Instructor  string    `json:"instructor"`
	StartTime   time.Time `json:"startTime"`
	EndTime     time.Time `json:"endTime"`
	Capacity    int       `json:"capacity"`
	Enrolled    int       `json:"enrolled"`
	Location    string    `json:"location"`
	Description string    `json:"description"`
}

type Booking struct {
	ID        string    `json:"id"`
	ClassID   string    `json:"classId"`
	UserEmail string    `json:"userEmail"`
	Status    string    `json:"status"`
	BookedAt  time.Time `json:"bookedAt"`
}

type PaymentMethod struct {
	Type  string `json:"type"`
	Last4 string `json:"last4"`
}

type Membership struct {
	ID            string        `json:"id"`
	Type          string        `json:"type"`
	StartDate     time.Time     `json:"startDate"`
	EndDate       time.Time     `json:"endDate"`
	Status        string        `json:"status"`
	HomeGym       string        `json:"homeGym"`
	PaymentMethod PaymentMethod `json:"paymentMethod"`
}

type User struct {
	Email      string     `json:"email"`
	Name       string     `json:"name"`
	Phone      string     `json:"phone"`
	Membership Membership `json:"membership"`
}

type Database struct {
	Users     map[string]User         `json:"users"`
	Locations map[string]Location     `json:"locations"`
	Classes   map[string]FitnessClass `json:"classes"`
	Bookings  map[string]Booking      `json:"bookings"`
	mu        sync.RWMutex
}

var db *Database

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:     make(map[string]User),
		Locations: make(map[string]Location),
		Classes:   make(map[string]FitnessClass),
		Bookings:  make(map[string]Booking),
	}

	return json.Unmarshal(data, db)
}

func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	return ((lat2 - lat1) * (lat2 - lat1)) + ((lon2 - lon1) * (lon2 - lon1))
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
	maxDistance := 50.0 // Maximum radius in km

	db.mu.RLock()
	for _, location := range db.Locations {
		distance := calculateDistance(lat, lon, location.Latitude, location.Longitude)
		if distance <= maxDistance {
			nearbyLocations = append(nearbyLocations, location)
		}
	}
	db.mu.RUnlock()

	return c.JSON(nearbyLocations)
}

func getClasses(c *fiber.Ctx) error {
	locationID := c.Query("locationId")
	dateStr := c.Query("date")

	if locationID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "locationId is required",
		})
	}

	var classes []FitnessClass
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

	db.mu.RLock()
	for _, class := range db.Classes {
		if class.Location == locationID {
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
		ClassID   string `json:"classId"`
		UserEmail string `json:"userEmail"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	// Verify user exists and has active membership
	user, exists := db.Users[req.UserEmail]
	if !exists || user.Membership.Status != "active" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "User not found or inactive membership",
		})
	}

	// Verify class exists and has capacity
	class, exists := db.Classes[req.ClassID]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Class not found",
		})
	}

	if class.Enrolled >= class.Capacity {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Class is full",
		})
	}

	// Create booking
	booking := Booking{
		ID:        uuid.New().String(),
		ClassID:   req.ClassID,
		UserEmail: req.UserEmail,
		Status:    "confirmed",
		BookedAt:  time.Now(),
	}

	db.Bookings[booking.ID] = booking
	class.Enrolled++
	db.Classes[req.ClassID] = class

	return c.Status(fiber.StatusCreated).JSON(booking)
}

func getMembership(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	db.mu.RLock()
	user, exists := db.Users[email]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	return c.JSON(user.Membership)
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

	api.Get("/locations", getLocations)
	api.Get("/classes", getClasses)
	api.Get("/bookings", getBookings)
	api.Post("/bookings", createBooking)
	api.Get("/membership", getMembership)
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

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
type Location struct {
	Address   string  `json:"address"`
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
	Location       Location  `json:"location"`
	CreditsBalance int       `json:"credits_balance"`
	MembershipType string    `json:"membership_type"`
	JoinedAt       time.Time `json:"joined_at"`
}

type Studio struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Categories  []string `json:"categories"`
	Rating      float64  `json:"rating"`
	Location    Location `json:"location"`
	Description string   `json:"description"`
}

type Class struct {
	ID              string    `json:"id"`
	StudioID        string    `json:"studio_id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	Instructor      string    `json:"instructor"`
	Category        string    `json:"category"`
	StartTime       time.Time `json:"start_time"`
	Duration        int       `json:"duration"`
	Capacity        int       `json:"capacity"`
	SpotsAvailable  int       `json:"spots_available"`
	CreditsRequired int       `json:"credits_required"`
}

type BookingStatus string

const (
	BookingStatusConfirmed BookingStatus = "confirmed"
	BookingStatusCancelled BookingStatus = "cancelled"
	BookingStatusCompleted BookingStatus = "completed"
)

type Booking struct {
	ID        string        `json:"id"`
	UserEmail string        `json:"user_email"`
	ClassID   string        `json:"class_id"`
	Status    BookingStatus `json:"status"`
	CreatedAt time.Time     `json:"created_at"`
}

// Database represents our in-memory database
type Database struct {
	Users    map[string]User    `json:"users"`
	Studios  map[string]Studio  `json:"studios"`
	Classes  map[string]Class   `json:"classes"`
	Bookings map[string]Booking `json:"bookings"`
	mu       sync.RWMutex
}

// Custom errors
var (
	ErrUserNotFound        = errors.New("user not found")
	ErrStudioNotFound      = errors.New("studio not found")
	ErrClassNotFound       = errors.New("class not found")
	ErrBookingNotFound     = errors.New("booking not found")
	ErrInsufficientCredits = errors.New("insufficient credits")
	ErrClassFull           = errors.New("class is full")
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

func (d *Database) GetClass(id string) (Class, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	class, exists := d.Classes[id]
	if !exists {
		return Class{}, ErrClassNotFound
	}
	return class, nil
}

func (d *Database) CreateBooking(booking Booking) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Verify user has enough credits
	user, exists := d.Users[booking.UserEmail]
	if !exists {
		return ErrUserNotFound
	}

	class, exists := d.Classes[booking.ClassID]
	if !exists {
		return ErrClassNotFound
	}

	if user.CreditsBalance < class.CreditsRequired {
		return ErrInsufficientCredits
	}

	if class.SpotsAvailable <= 0 {
		return ErrClassFull
	}

	// Update spots available
	class.SpotsAvailable--
	d.Classes[booking.ClassID] = class

	// Deduct credits
	user.CreditsBalance -= class.CreditsRequired
	d.Users[booking.UserEmail] = user

	// Save booking
	d.Bookings[booking.ID] = booking
	return nil
}

func (d *Database) CancelBooking(id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	booking, exists := d.Bookings[id]
	if !exists {
		return ErrBookingNotFound
	}

	if booking.Status == BookingStatusCancelled {
		return errors.New("booking already cancelled")
	}

	// Refund credits
	class := d.Classes[booking.ClassID]
	user := d.Users[booking.UserEmail]
	user.CreditsBalance += class.CreditsRequired
	d.Users[booking.UserEmail] = user

	// Update spots available
	class.SpotsAvailable++
	d.Classes[booking.ClassID] = class

	// Update booking status
	booking.Status = BookingStatusCancelled
	d.Bookings[id] = booking

	return nil
}

// Business logic helpers
func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	// Simplified distance calculation
	return ((lat2 - lat1) * (lat2 - lat1)) + ((lon2 - lon1) * (lon2 - lon1))
}

// HTTP Handlers
func getStudios(c *fiber.Ctx) error {
	lat := c.QueryFloat("latitude", 0)
	lon := c.QueryFloat("longitude", 0)
	category := c.Query("category")

	if lat == 0 || lon == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "latitude and longitude are required",
		})
	}

	var nearbyStudios []Studio
	maxDistance := 10.0 // Maximum radius in km

	db.mu.RLock()
	for _, studio := range db.Studios {
		distance := calculateDistance(lat, lon,
			studio.Location.Latitude,
			studio.Location.Longitude)

		if distance <= maxDistance {
			if category == "" || contains(studio.Categories, category) {
				nearbyStudios = append(nearbyStudios, studio)
			}
		}
	}
	db.mu.RUnlock()

	return c.JSON(nearbyStudios)
}

func getClasses(c *fiber.Ctx) error {
	date := c.Query("date")
	studioID := c.Query("studio_id")

	var classes []Class
	db.mu.RLock()
	for _, class := range db.Classes {
		if studioID != "" && class.StudioID != studioID {
			continue
		}

		if date != "" {
			classDate := class.StartTime.Format("2006-01-02")
			if classDate != date {
				continue
			}
		}

		classes = append(classes, class)
	}
	db.mu.RUnlock()

	return c.JSON(classes)
}

func getUserBookings(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
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

type CreateBookingRequest struct {
	UserEmail string `json:"user_email"`
	ClassID   string `json:"class_id"`
}

func createBooking(c *fiber.Ctx) error {
	var req CreateBookingRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	booking := Booking{
		ID:        uuid.New().String(),
		UserEmail: req.UserEmail,
		ClassID:   req.ClassID,
		Status:    BookingStatusConfirmed,
		CreatedAt: time.Now(),
	}

	if err := db.CreateBooking(booking); err != nil {
		status := fiber.StatusInternalServerError
		switch err {
		case ErrUserNotFound, ErrClassNotFound:
			status = fiber.StatusNotFound
		case ErrInsufficientCredits, ErrClassFull:
			status = fiber.StatusBadRequest
		}
		return c.Status(status).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(booking)
}

func cancelBooking(c *fiber.Ctx) error {
	bookingID := c.Params("bookingId")
	if bookingID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "booking ID is required",
		})
	}

	if err := db.CancelBooking(bookingID); err != nil {
		status := fiber.StatusInternalServerError
		if errors.Is(err, ErrBookingNotFound) {
			status = fiber.StatusNotFound
		}
		return c.Status(status).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Booking cancelled successfully",
	})
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:    make(map[string]User),
		Studios:  make(map[string]Studio),
		Classes:  make(map[string]Class),
		Bookings: make(map[string]Booking),
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

	// Studio routes
	api.Get("/studios", getStudios)

	// Class routes
	api.Get("/classes", getClasses)

	// Booking routes
	api.Get("/bookings", getUserBookings)
	api.Post("/bookings", createBooking)
	api.Delete("/bookings/:bookingId", cancelBooking)
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

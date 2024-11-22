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
	Address   string  `json:"address"`
	City      string  `json:"city"`
	State     string  `json:"state"`
	ZipCode   string  `json:"zip_code"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type Studio struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Categories  []string  `json:"categories"`
	Rating      float64   `json:"rating"`
	Location    Location  `json:"location"`
	Description string    `json:"description"`
	Amenities   []string  `json:"amenities"`
	CreatedAt   time.Time `json:"created_at"`
}

type Instructor struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Bio         string   `json:"bio"`
	Specialties []string `json:"specialties"`
	ImageURL    string   `json:"image_url"`
}

type Class struct {
	ID              string     `json:"id"`
	StudioID        string     `json:"studio_id"`
	Name            string     `json:"name"`
	Description     string     `json:"description"`
	Instructor      Instructor `json:"instructor"`
	Category        string     `json:"category"`
	StartTime       time.Time  `json:"start_time"`
	Duration        int        `json:"duration"` // in minutes
	SpotsTotal      int        `json:"spots_total"`
	SpotsAvailable  int        `json:"spots_available"`
	CreditsRequired int        `json:"credits_required"`
}

type MembershipPlan string

const (
	PlanBasic     MembershipPlan = "basic"
	PlanPremium   MembershipPlan = "premium"
	PlanUnlimited MembershipPlan = "unlimited"
)

type Membership struct {
	UserEmail        string         `json:"user_email"`
	Plan             MembershipPlan `json:"plan"`
	CreditsRemaining int            `json:"credits_remaining"`
	CreditsResetDate time.Time      `json:"credits_reset_date"`
	Active           bool           `json:"active"`
	StartDate        time.Time      `json:"start_date"`
	NextBillingDate  time.Time      `json:"next_billing_date"`
}

type BookingStatus string

const (
	BookingConfirmed BookingStatus = "confirmed"
	BookingCancelled BookingStatus = "cancelled"
	BookingCompleted BookingStatus = "completed"
)

type Booking struct {
	ID          string        `json:"id"`
	UserEmail   string        `json:"user_email"`
	Class       Class         `json:"class"`
	Status      BookingStatus `json:"status"`
	CreditsUsed int           `json:"credits_used"`
	BookedAt    time.Time     `json:"booked_at"`
}

type User struct {
	Email      string     `json:"email"`
	Name       string     `json:"name"`
	Membership Membership `json:"membership"`
}

// Database represents our in-memory database
type Database struct {
	Users       map[string]User       `json:"users"`
	Studios     map[string]Studio     `json:"studios"`
	Classes     map[string]Class      `json:"classes"`
	Bookings    map[string]Booking    `json:"bookings"`
	Instructors map[string]Instructor `json:"instructors"`
	mu          sync.RWMutex
}

// Global database instance
var db *Database

// Error definitions
var (
	ErrUserNotFound        = errors.New("user not found")
	ErrStudioNotFound      = errors.New("studio not found")
	ErrClassNotFound       = errors.New("class not found")
	ErrBookingNotFound     = errors.New("booking not found")
	ErrInsufficientCredits = errors.New("insufficient credits")
	ErrClassFull           = errors.New("class is full")
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

	// Update class spots
	class := d.Classes[booking.Class.ID]
	if class.SpotsAvailable <= 0 {
		return ErrClassFull
	}

	class.SpotsAvailable--
	d.Classes[class.ID] = class

	// Update user credits
	user := d.Users[booking.UserEmail]
	user.Membership.CreditsRemaining -= booking.CreditsUsed
	d.Users[booking.UserEmail] = user

	// Save booking
	d.Bookings[booking.ID] = booking
	return nil
}

// HTTP Handlers
func getStudios(c *fiber.Ctx) error {
	lat := c.QueryFloat("latitude", 0)
	lon := c.QueryFloat("longitude", 0)
	category := c.Query("category")

	var studios []Studio
	db.mu.RLock()
	for _, studio := range db.Studios {
		// Filter by category if specified
		if category != "" {
			categoryMatch := false
			for _, cat := range studio.Categories {
				if cat == category {
					categoryMatch = true
					break
				}
			}
			if !categoryMatch {
				continue
			}
		}

		// Filter by location if coordinates provided
		if lat != 0 && lon != 0 {
			distance := calculateDistance(lat, lon,
				studio.Location.Latitude,
				studio.Location.Longitude)
			if distance > 10 { // 10km radius
				continue
			}
		}

		studios = append(studios, studio)
	}
	db.mu.RUnlock()

	return c.JSON(studios)
}

func getClasses(c *fiber.Ctx) error {
	studioID := c.Query("studio_id")
	dateStr := c.Query("date")

	var classes []Class
	db.mu.RLock()
	for _, class := range db.Classes {
		// Filter by studio if specified
		if studioID != "" && class.StudioID != studioID {
			continue
		}

		// Filter by date if specified
		if dateStr != "" {
			date, err := time.Parse("2006-01-02", dateStr)
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"error": "Invalid date format",
				})
			}

			if !isSameDay(class.StartTime, date) {
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

	var bookings []Booking
	db.mu.RLock()
	for _, booking := range db.Bookings {
		if booking.UserEmail == email {
			bookings = append(bookings, booking)
		}
	}
	db.mu.RUnlock()

	return c.JSON(bookings)
}

type BookingRequest struct {
	ClassID   string `json:"class_id"`
	UserEmail string `json:"user_email"`
}

func createBooking(c *fiber.Ctx) error {
	var req BookingRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user and membership
	user, err := db.GetUser(req.UserEmail)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if !user.Membership.Active {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
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

	// Validate credits
	if user.Membership.CreditsRemaining < class.CreditsRequired {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Insufficient credits",
		})
	}

	// Create booking
	booking := Booking{
		ID:          uuid.New().String(),
		UserEmail:   req.UserEmail,
		Class:       class,
		Status:      BookingConfirmed,
		CreditsUsed: class.CreditsRequired,
		BookedAt:    time.Now(),
	}

	// Save booking
	if err := db.CreateBooking(booking); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(booking)
}

func cancelBooking(c *fiber.Ctx) error {
	bookingID := c.Params("bookingId")
	if bookingID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Booking ID is required",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	booking, exists := db.Bookings[bookingID]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Booking not found",
		})
	}

	// Validate cancellation time (e.g., must be at least 12 hours before class)
	if time.Until(booking.Class.StartTime) < 12*time.Hour {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot cancel class less than 12 hours before start time",
		})
	}

	// Refund credits
	user := db.Users[booking.UserEmail]
	user.Membership.CreditsRemaining += booking.CreditsUsed
	db.Users[booking.UserEmail] = user

	// Update class spots
	class := db.Classes[booking.Class.ID]
	class.SpotsAvailable++
	db.Classes[class.ID] = class

	// Update booking status
	booking.Status = BookingCancelled
	db.Bookings[bookingID] = booking

	return c.JSON(booking)
}

func getMembership(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	user, err := db.GetUser(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(user.Membership)
}

// Helper functions
func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	// Simplified distance calculation
	return ((lat2 - lat1) * (lat2 - lat1)) + ((lon2 - lon1) * (lon2 - lon1))
}

func isSameDay(t1, t2 time.Time) bool {
	y1, m1, d1 := t1.Date()
	y2, m2, d2 := t2.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:       make(map[string]User),
		Studios:     make(map[string]Studio),
		Classes:     make(map[string]Class),
		Bookings:    make(map[string]Booking),
		Instructors: make(map[string]Instructor),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Studio routes
	api.Get("/studios", getStudios)

	// Class routes
	api.Get("/classes", getClasses)

	// Booking routes
	api.Get("/bookings", getUserBookings)
	api.Post("/bookings", createBooking)
	api.Post("/bookings/:bookingId/cancel", cancelBooking)

	// Membership routes
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
	app.Use(cors.New())

	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

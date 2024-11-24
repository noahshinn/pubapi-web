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
type Celebrity struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Category     string   `json:"category"`
	Price        float64  `json:"price"`
	Rating       float64  `json:"rating"`
	ResponseTime string   `json:"response_time"`
	Bio          string   `json:"bio"`
	ProfileImage string   `json:"profile_image"`
	SampleVideos []string `json:"sample_videos"`
	IsAvailable  bool     `json:"is_available"`
}

type BookingStatus string

const (
	BookingStatusPending   BookingStatus = "pending"
	BookingStatusAccepted  BookingStatus = "accepted"
	BookingStatusRejected  BookingStatus = "rejected"
	BookingStatusCompleted BookingStatus = "completed"
	BookingStatusExpired   BookingStatus = "expired"
)

type Booking struct {
	ID            string        `json:"id"`
	Celebrity     Celebrity     `json:"celebrity"`
	UserEmail     string        `json:"user_email"`
	Occasion      string        `json:"occasion"`
	RecipientName string        `json:"recipient_name"`
	Instructions  string        `json:"instructions"`
	Status        BookingStatus `json:"status"`
	VideoURL      string        `json:"video_url,omitempty"`
	CreatedAt     time.Time     `json:"created_at"`
	CompletedAt   *time.Time    `json:"completed_at,omitempty"`
}

type User struct {
	Email          string          `json:"email"`
	Name           string          `json:"name"`
	ProfilePicture string          `json:"profile_picture"`
	RegisteredAt   time.Time       `json:"registered_at"`
	FavoriteStars  []string        `json:"favorite_stars"`
	PaymentMethods []PaymentMethod `json:"payment_methods"`
}

type PaymentMethod struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Last4     string    `json:"last4"`
	ExpiryMM  int       `json:"expiry_mm"`
	ExpiryYY  int       `json:"expiry_yy"`
	CreatedAt time.Time `json:"created_at"`
}

// Database represents our in-memory database
type Database struct {
	Celebrities map[string]Celebrity `json:"celebrities"`
	Bookings    map[string]Booking   `json:"bookings"`
	Users       map[string]User      `json:"users"`
	mu          sync.RWMutex
}

// Custom errors
var (
	ErrCelebrityNotFound = errors.New("celebrity not found")
	ErrBookingNotFound   = errors.New("booking not found")
	ErrUserNotFound      = errors.New("user not found")
	ErrInvalidInput      = errors.New("invalid input")
)

// Global database instance
var db *Database

// Database operations
func (d *Database) GetCelebrity(id string) (Celebrity, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	celebrity, exists := d.Celebrities[id]
	if !exists {
		return Celebrity{}, ErrCelebrityNotFound
	}
	return celebrity, nil
}

func (d *Database) GetUser(email string) (User, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	user, exists := d.Users[email]
	if !exists {
		return User{}, ErrUserNotFound
	}
	return user, nil
}

func (d *Database) CreateBooking(booking Booking) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Bookings[booking.ID] = booking
	return nil
}

func (d *Database) GetBooking(id string) (Booking, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	booking, exists := d.Bookings[id]
	if !exists {
		return Booking{}, ErrBookingNotFound
	}
	return booking, nil
}

// HTTP Handlers
func getCelebrities(c *fiber.Ctx) error {
	category := c.Query("category")
	priceMax := c.QueryFloat("price_max", 0)

	var celebrities []Celebrity
	db.mu.RLock()
	for _, celeb := range db.Celebrities {
		if (category == "" || celeb.Category == category) &&
			(priceMax == 0 || celeb.Price <= priceMax) {
			celebrities = append(celebrities, celeb)
		}
	}
	db.mu.RUnlock()

	return c.JSON(celebrities)
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

type CreateBookingRequest struct {
	CelebrityID   string `json:"celebrity_id"`
	UserEmail     string `json:"user_email"`
	Occasion      string `json:"occasion"`
	RecipientName string `json:"recipient_name"`
	Instructions  string `json:"instructions"`
}

func createBooking(c *fiber.Ctx) error {
	var req CreateBookingRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user
	user, err := db.GetUser(req.UserEmail)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// Validate celebrity
	celebrity, err := db.GetCelebrity(req.CelebrityID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Celebrity not found",
		})
	}

	if !celebrity.IsAvailable {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Celebrity is not currently accepting bookings",
		})
	}

	booking := Booking{
		ID:            uuid.New().String(),
		Celebrity:     celebrity,
		UserEmail:     user.Email,
		Occasion:      req.Occasion,
		RecipientName: req.RecipientName,
		Instructions:  req.Instructions,
		Status:        BookingStatusPending,
		CreatedAt:     time.Now(),
	}

	if err := db.CreateBooking(booking); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create booking",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(booking)
}

func getBooking(c *fiber.Ctx) error {
	bookingId := c.Params("bookingId")
	if bookingId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Booking ID is required",
		})
	}

	booking, err := db.GetBooking(bookingId)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Booking not found",
		})
	}

	return c.JSON(booking)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Celebrities: make(map[string]Celebrity),
		Bookings:    make(map[string]Booking),
		Users:       make(map[string]User),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Celebrity routes
	api.Get("/celebrities", getCelebrities)
	api.Get("/celebrities/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		celebrity, err := db.GetCelebrity(id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Celebrity not found",
			})
		}
		return c.JSON(celebrity)
	})

	// Booking routes
	api.Get("/bookings", getUserBookings)
	api.Post("/bookings", createBooking)
	api.Get("/bookings/:bookingId", getBooking)

	// User routes
	api.Get("/users/:email", func(c *fiber.Ctx) error {
		email := c.Params("email")
		user, err := db.GetUser(email)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "User not found",
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
	app.Use(cors.New())

	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

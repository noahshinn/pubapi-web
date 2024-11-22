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
	City    string `json:"city"`
	Country string `json:"country"`
	Airport string `json:"airport,omitempty"`
}

type Flight struct {
	ID             string    `json:"id"`
	Airline        string    `json:"airline"`
	FlightNumber   string    `json:"flight_number"`
	Origin         Location  `json:"origin"`
	Destination    Location  `json:"destination"`
	DepartureTime  time.Time `json:"departure_time"`
	ArrivalTime    time.Time `json:"arrival_time"`
	Price          float64   `json:"price"`
	SeatsAvailable int       `json:"seats_available"`
	Class          string    `json:"class"`
}

type Hotel struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Location      Location `json:"location"`
	Rating        float64  `json:"rating"`
	PricePerNight float64  `json:"price_per_night"`
	Amenities     []string `json:"amenities"`
	RoomTypes     []string `json:"room_types"`
}

type BookingStatus string

const (
	BookingStatusConfirmed BookingStatus = "confirmed"
	BookingStatusPending   BookingStatus = "pending"
	BookingStatusCancelled BookingStatus = "cancelled"
)

type BookingType string

const (
	BookingTypeFlight BookingType = "flight"
	BookingTypeHotel  BookingType = "hotel"
)

type Booking struct {
	ID          string        `json:"id"`
	UserEmail   string        `json:"user_email"`
	Type        BookingType   `json:"type"`
	Status      BookingStatus `json:"status"`
	Details     interface{}   `json:"details"`
	TotalPrice  float64       `json:"total_price"`
	BookingDate time.Time     `json:"booking_date"`
}

type User struct {
	Email          string    `json:"email"`
	Name           string    `json:"name"`
	Phone          string    `json:"phone"`
	PreferredSeat  string    `json:"preferred_seat"`
	FrequentFlyer  []string  `json:"frequent_flyer"`
	PaymentMethods []Payment `json:"payment_methods"`
}

type Payment struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Last4    string `json:"last4"`
	ExpiryMM int    `json:"expiry_mm"`
	ExpiryYY int    `json:"expiry_yy"`
}

// Database represents our in-memory database
type Database struct {
	Users    map[string]User    `json:"users"`
	Flights  map[string]Flight  `json:"flights"`
	Hotels   map[string]Hotel   `json:"hotels"`
	Bookings map[string]Booking `json:"bookings"`
	mu       sync.RWMutex
}

var db *Database

// Error definitions
var (
	ErrUserNotFound    = errors.New("user not found")
	ErrFlightNotFound  = errors.New("flight not found")
	ErrHotelNotFound   = errors.New("hotel not found")
	ErrBookingNotFound = errors.New("booking not found")
	ErrInvalidBooking  = errors.New("invalid booking request")
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

func (d *Database) SearchFlights(origin, destination string, departureDate time.Time) []Flight {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var results []Flight
	for _, flight := range d.Flights {
		if flight.Origin.Airport == origin &&
			flight.Destination.Airport == destination &&
			flight.DepartureTime.Format("2006-01-02") == departureDate.Format("2006-01-02") {
			results = append(results, flight)
		}
	}
	return results
}

func (d *Database) SearchHotels(location string, checkIn, checkOut time.Time) []Hotel {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var results []Hotel
	for _, hotel := range d.Hotels {
		if hotel.Location.City == location {
			results = append(results, hotel)
		}
	}
	return results
}

func (d *Database) CreateBooking(booking Booking) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Bookings[booking.ID] = booking
	return nil
}

func (d *Database) GetUserBookings(email string) []Booking {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var bookings []Booking
	for _, booking := range d.Bookings {
		if booking.UserEmail == email {
			bookings = append(bookings, booking)
		}
	}
	return bookings
}

// HTTP Handlers
func searchFlights(c *fiber.Ctx) error {
	origin := c.Query("origin")
	destination := c.Query("destination")
	departureDate := c.Query("departure_date")

	if origin == "" || destination == "" || departureDate == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Missing required parameters",
		})
	}

	date, err := time.Parse("2006-01-02", departureDate)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid date format",
		})
	}

	flights := db.SearchFlights(origin, destination, date)
	return c.JSON(flights)
}

func searchHotels(c *fiber.Ctx) error {
	location := c.Query("location")
	checkIn := c.Query("check_in")
	checkOut := c.Query("check_out")

	if location == "" || checkIn == "" || checkOut == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Missing required parameters",
		})
	}

	checkInDate, err := time.Parse("2006-01-02", checkIn)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid check-in date format",
		})
	}

	checkOutDate, err := time.Parse("2006-01-02", checkOut)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid check-out date format",
		})
	}

	hotels := db.SearchHotels(location, checkInDate, checkOutDate)
	return c.JSON(hotels)
}

type CreateBookingRequest struct {
	UserEmail     string      `json:"user_email"`
	Type          BookingType `json:"type"`
	ItemID        string      `json:"item_id"`
	PaymentMethod string      `json:"payment_method"`
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
			"error": err.Error(),
		})
	}

	// Validate payment method
	validPayment := false
	for _, pm := range user.PaymentMethods {
		if pm.ID == req.PaymentMethod {
			validPayment = true
			break
		}
	}
	if !validPayment {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid payment method",
		})
	}

	var details interface{}
	var totalPrice float64

	switch req.Type {
	case BookingTypeFlight:
		flight, exists := db.Flights[req.ItemID]
		if !exists {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Flight not found",
			})
		}
		details = flight
		totalPrice = flight.Price

	case BookingTypeHotel:
		hotel, exists := db.Hotels[req.ItemID]
		if !exists {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Hotel not found",
			})
		}
		details = hotel
		totalPrice = hotel.PricePerNight // Note: In real implementation, multiply by number of nights

	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid booking type",
		})
	}

	booking := Booking{
		ID:          uuid.New().String(),
		UserEmail:   req.UserEmail,
		Type:        req.Type,
		Status:      BookingStatusConfirmed,
		Details:     details,
		TotalPrice:  totalPrice,
		BookingDate: time.Now(),
	}

	if err := db.CreateBooking(booking); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create booking",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(booking)
}

func getUserBookings(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email parameter is required",
		})
	}

	// Verify user exists
	if _, err := db.GetUser(email); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	bookings := db.GetUserBookings(email)
	return c.JSON(bookings)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:    make(map[string]User),
		Flights:  make(map[string]Flight),
		Hotels:   make(map[string]Hotel),
		Bookings: make(map[string]Booking),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Flight routes
	api.Get("/flights/search", searchFlights)

	// Hotel routes
	api.Get("/hotels/search", searchHotels)

	// Booking routes
	api.Get("/bookings", getUserBookings)
	api.Post("/bookings", createBooking)
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
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE",
		AllowHeaders: "Origin, Content-Type, Accept",
	}))

	// Setup routes
	setupRoutes(app)

	// Start server
	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

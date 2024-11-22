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
	Country   string  `json:"country"`
	ZipCode   string  `json:"zip_code"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type PaymentMethod struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Last4     string    `json:"last4"`
	ExpiryMM  int       `json:"expiry_mm"`
	ExpiryYY  int       `json:"expiry_yy"`
	CreatedAt time.Time `json:"created_at"`
}

type User struct {
	Email          string          `json:"email"`
	Name           string          `json:"name"`
	Phone          string          `json:"phone"`
	Address        Address         `json:"address"`
	PaymentMethods []PaymentMethod `json:"payment_methods"`
}

type Hotel struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Rating        float64  `json:"rating"`
	Address       Address  `json:"address"`
	PricePerNight float64  `json:"price_per_night"`
	Amenities     []string `json:"amenities"`
	RoomTypes     []Room   `json:"room_types"`
}

type Room struct {
	ID        string  `json:"id"`
	Type      string  `json:"type"`
	Price     float64 `json:"price"`
	Capacity  int     `json:"capacity"`
	Available bool    `json:"available"`
}

type Flight struct {
	ID             string    `json:"id"`
	Airline        string    `json:"airline"`
	FlightNumber   string    `json:"flight_number"`
	Origin         string    `json:"origin"`
	Destination    string    `json:"destination"`
	DepartureTime  time.Time `json:"departure_time"`
	ArrivalTime    time.Time `json:"arrival_time"`
	Price          float64   `json:"price"`
	SeatsAvailable int       `json:"seats_available"`
	Class          string    `json:"class"`
}

type BookingStatus string

const (
	BookingStatusPending   BookingStatus = "pending"
	BookingStatusConfirmed BookingStatus = "confirmed"
	BookingStatusCancelled BookingStatus = "cancelled"
	BookingStatusCompleted BookingStatus = "completed"
)

type BookingType string

const (
	BookingTypeHotel  BookingType = "hotel"
	BookingTypeFlight BookingType = "flight"
)

type Booking struct {
	ID            string        `json:"id"`
	Type          BookingType   `json:"type"`
	UserEmail     string        `json:"user_email"`
	Status        BookingStatus `json:"status"`
	Hotel         *Hotel        `json:"hotel,omitempty"`
	Flight        *Flight       `json:"flight,omitempty"`
	CheckIn       *time.Time    `json:"check_in,omitempty"`
	CheckOut      *time.Time    `json:"check_out,omitempty"`
	Guests        int           `json:"guests,omitempty"`
	TotalPrice    float64       `json:"total_price"`
	PaymentMethod string        `json:"payment_method"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

// Database represents our in-memory database
type Database struct {
	Users    map[string]User    `json:"users"`
	Hotels   map[string]Hotel   `json:"hotels"`
	Flights  map[string]Flight  `json:"flights"`
	Bookings map[string]Booking `json:"bookings"`
	mu       sync.RWMutex
}

// Custom errors
var (
	ErrUserNotFound    = errors.New("user not found")
	ErrHotelNotFound   = errors.New("hotel not found")
	ErrFlightNotFound  = errors.New("flight not found")
	ErrBookingNotFound = errors.New("booking not found")
	ErrInvalidInput    = errors.New("invalid input")
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

func (d *Database) SearchHotels(destination string, checkIn, checkOut time.Time, guests int) []Hotel {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var results []Hotel
	for _, hotel := range d.Hotels {
		if hotel.Address.City == destination {
			// Check room availability
			for _, room := range hotel.RoomTypes {
				if room.Available && room.Capacity >= guests {
					results = append(results, hotel)
					break
				}
			}
		}
	}
	return results
}

func (d *Database) SearchFlights(origin, destination string, departureDate time.Time) []Flight {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var results []Flight
	for _, flight := range d.Flights {
		if flight.Origin == origin &&
			flight.Destination == destination &&
			flight.DepartureTime.Format("2006-01-02") == departureDate.Format("2006-01-02") &&
			flight.SeatsAvailable > 0 {
			results = append(results, flight)
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

// HTTP Handlers
func searchHotels(c *fiber.Ctx) error {
	destination := c.Query("destination")
	checkInStr := c.Query("check_in")
	checkOutStr := c.Query("check_out")
	guests := c.QueryInt("guests", 1)

	if destination == "" || checkInStr == "" || checkOutStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Missing required parameters",
		})
	}

	checkIn, err := time.Parse("2006-01-02", checkInStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid check-in date format",
		})
	}

	checkOut, err := time.Parse("2006-01-02", checkOutStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid check-out date format",
		})
	}

	hotels := db.SearchHotels(destination, checkIn, checkOut, guests)
	return c.JSON(hotels)
}

func searchFlights(c *fiber.Ctx) error {
	origin := c.Query("origin")
	destination := c.Query("destination")
	departureDateStr := c.Query("departure_date")

	if origin == "" || destination == "" || departureDateStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Missing required parameters",
		})
	}

	departureDate, err := time.Parse("2006-01-02", departureDateStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid departure date format",
		})
	}

	flights := db.SearchFlights(origin, destination, departureDate)
	return c.JSON(flights)
}

func getUserBookings(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email parameter is required",
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
	Type          BookingType `json:"type"`
	UserEmail     string      `json:"user_email"`
	ItemID        string      `json:"item_id"`
	CheckIn       *string     `json:"check_in,omitempty"`
	CheckOut      *string     `json:"check_out,omitempty"`
	Guests        *int        `json:"guests,omitempty"`
	PaymentMethod string      `json:"payment_method"`
}

func createBooking(c *fiber.Ctx) error {
	var req CreateBookingRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	//

	// Validate user
	user, err := db.GetUser(req.UserEmail)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	var booking Booking
	booking.ID = uuid.New().String()
	booking.Type = req.Type
	booking.UserEmail = req.UserEmail
	booking.Status = BookingStatusPending
	booking.PaymentMethod = req.PaymentMethod
	booking.CreatedAt = time.Now()
	booking.UpdatedAt = time.Now()

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

	switch req.Type {
	case BookingTypeHotel:
		if req.CheckIn == nil || req.CheckOut == nil || req.Guests == nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Check-in, check-out dates and guests are required for hotel bookings",
			})
		}

		checkIn, err := time.Parse("2006-01-02", *req.CheckIn)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid check-in date format",
			})
		}

		checkOut, err := time.Parse("2006-01-02", *req.CheckOut)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid check-out date format",
			})
		}

		hotel, exists := db.Hotels[req.ItemID]
		if !exists {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Hotel not found",
			})
		}

		booking.Hotel = &hotel
		booking.CheckIn = &checkIn
		booking.CheckOut = &checkOut
		booking.Guests = *req.Guests

		nights := checkOut.Sub(checkIn).Hours() / 24
		booking.TotalPrice = hotel.PricePerNight * float64(nights)

	case BookingTypeFlight:
		flight, exists := db.Flights[req.ItemID]
		if !exists {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Flight not found",
			})
		}

		if flight.SeatsAvailable <= 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Flight is fully booked",
			})
		}

		booking.Flight = &flight
		booking.TotalPrice = flight.Price

	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid booking type",
		})
	}

	if err := db.CreateBooking(booking); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create booking",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(booking)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:    make(map[string]User),
		Hotels:   make(map[string]Hotel),
		Flights:  make(map[string]Flight),
		Bookings: make(map[string]Booking),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Hotel routes
	api.Get("/hotels/search", searchHotels)

	// Flight routes
	api.Get("/flights/search", searchFlights)

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

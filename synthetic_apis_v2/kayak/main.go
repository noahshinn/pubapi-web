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
type Location struct {
	City    string  `json:"city"`
	Country string  `json:"country"`
	Airport string  `json:"airport,omitempty"`
	Lat     float64 `json:"latitude"`
	Lon     float64 `json:"longitude"`
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
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Location       Location  `json:"location"`
	Rating         float64   `json:"rating"`
	PricePerNight  float64   `json:"price_per_night"`
	Amenities      []string  `json:"amenities"`
	RoomsAvailable int       `json:"rooms_available"`
	CheckIn        time.Time `json:"check_in"`
	CheckOut       time.Time `json:"check_out"`
}

type User struct {
	Email          string   `json:"email"`
	Name           string   `json:"name"`
	Phone          string   `json:"phone"`
	PreferredSeats []string `json:"preferred_seats"`
	Preferences    struct {
		Airlines []string `json:"preferred_airlines"`
		Hotels   []string `json:"preferred_hotels"`
	} `json:"preferences"`
	PaymentMethods []PaymentMethod `json:"payment_methods"`
}

type PaymentMethod struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Last4    string `json:"last4"`
	ExpiryMM int    `json:"expiry_mm"`
	ExpiryYY int    `json:"expiry_yy"`
}

type Booking struct {
	ID          string      `json:"id"`
	UserEmail   string      `json:"user_email"`
	Type        string      `json:"type"` // "flight" or "hotel"
	Status      string      `json:"status"`
	Details     interface{} `json:"details"` // Flight or Hotel
	TotalPrice  float64     `json:"total_price"`
	BookingDate time.Time   `json:"booking_date"`
}

// Database
type Database struct {
	Users    map[string]User    `json:"users"`
	Flights  map[string]Flight  `json:"flights"`
	Hotels   map[string]Hotel   `json:"hotels"`
	Bookings map[string]Booking `json:"bookings"`
	mu       sync.RWMutex
}

var db *Database

// Helper functions
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

func searchFlights(origin, destination string, departureDate time.Time) []Flight {
	var results []Flight
	db.mu.RLock()
	defer db.mu.RUnlock()

	for _, flight := range db.Flights {
		if flight.Origin.Airport == origin &&
			flight.Destination.Airport == destination &&
			flight.DepartureTime.Format("2006-01-02") == departureDate.Format("2006-01-02") {
			results = append(results, flight)
		}
	}
	return results
}

func searchHotels(location string, checkIn, checkOut time.Time) []Hotel {
	var results []Hotel
	db.mu.RLock()
	defer db.mu.RUnlock()

	for _, hotel := range db.Hotels {
		if hotel.Location.City == location && hotel.RoomsAvailable > 0 {
			results = append(results, hotel)
		}
	}
	return results
}

// Handlers
func handleFlightSearch(c *fiber.Ctx) error {
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

	flights := searchFlights(origin, destination, date)
	return c.JSON(flights)
}

func handleHotelSearch(c *fiber.Ctx) error {
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

	hotels := searchHotels(location, checkInDate, checkOutDate)
	return c.JSON(hotels)
}

func handleGetBookings(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email is required",
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

type NewBookingRequest struct {
	UserEmail     string `json:"user_email"`
	Type          string `json:"type"`
	ItemID        string `json:"item_id"`
	Passengers    int    `json:"passengers"`
	PaymentMethod string `json:"payment_method"`
}

func handleCreateBooking(c *fiber.Ctx) error {
	var req NewBookingRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user
	db.mu.RLock()
	user, exists := db.Users[req.UserEmail]
	db.mu.RUnlock()
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
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

	var booking Booking
	booking.ID = uuid.New().String()
	booking.UserEmail = req.UserEmail
	booking.Type = req.Type
	booking.Status = "confirmed"
	booking.BookingDate = time.Now()

	db.mu.Lock()
	defer db.mu.Unlock()

	switch req.Type {
	case "flight":
		flight, exists := db.Flights[req.ItemID]
		if !exists {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Flight not found",
			})
		}
		if flight.SeatsAvailable < req.Passengers {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Not enough seats available",
			})
		}
		booking.Details = flight

		booking.TotalPrice = flight.Price * float64(req.Passengers)
		flight.SeatsAvailable -= req.Passengers
		db.Flights[req.ItemID] = flight

	case "hotel":
		hotel, exists := db.Hotels[req.ItemID]
		if !exists {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Hotel not found",
			})
		}
		if hotel.RoomsAvailable < 1 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "No rooms available",
			})
		}
		booking.Details = hotel
		nights := hotel.CheckOut.Sub(hotel.CheckIn).Hours() / 24
		booking.TotalPrice = hotel.PricePerNight * float64(nights)
		hotel.RoomsAvailable--
		db.Hotels[req.ItemID] = hotel

	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid booking type",
		})
	}

	db.Bookings[booking.ID] = booking
	return c.Status(fiber.StatusCreated).JSON(booking)
}

func main() {
	port := flag.String("port", "3000", "Port to run the server on")
	flag.Parse()

	if err := loadDatabase(); err != nil {
		log.Fatal(err)
	}

	app := fiber.New()

	// Middleware
	app.Use(logger.New())
	app.Use(cors.New())

	// Serve OpenAPI spec
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

	// API routes
	api := app.Group("/api/v1")

	api.Get("/flights/search", handleFlightSearch)
	api.Get("/hotels/search", handleHotelSearch)
	api.Get("/bookings", handleGetBookings)
	api.Post("/bookings", handleCreateBooking)

	log.Printf("Server starting on port %s", *port)
	log.Fatal(app.Listen(":" + *port))
}

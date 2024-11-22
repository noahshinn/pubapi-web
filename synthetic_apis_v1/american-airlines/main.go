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
type Airport struct {
	Code    string  `json:"code"`
	Name    string  `json:"name"`
	City    string  `json:"city"`
	Country string  `json:"country"`
	Lat     float64 `json:"latitude"`
	Lon     float64 `json:"longitude"`
}

type Flight struct {
	FlightNumber   string    `json:"flight_number"`
	Origin         Airport   `json:"origin"`
	Destination    Airport   `json:"destination"`
	DepartureTime  time.Time `json:"departure_time"`
	ArrivalTime    time.Time `json:"arrival_time"`
	Duration       string    `json:"duration"`
	Aircraft       string    `json:"aircraft"`
	AvailableSeats int       `json:"available_seats"`
	Price          float64   `json:"price"`
}

type Passenger struct {
	Email               string `json:"email"`
	FirstName           string `json:"first_name"`
	LastName            string `json:"last_name"`
	FrequentFlyerNumber string `json:"frequent_flyer_number"`
	SeatPreference      string `json:"seat_preference"`
}

type ReservationStatus string

const (
	ReservationStatusConfirmed ReservationStatus = "confirmed"
	ReservationStatusCancelled ReservationStatus = "cancelled"
	ReservationStatusCheckedIn ReservationStatus = "checked_in"
)

type Reservation struct {
	ReservationCode string            `json:"reservation_code"`
	Passenger       Passenger         `json:"passenger"`
	Flights         []Flight          `json:"flights"`
	Status          ReservationStatus `json:"status"`
	TotalPrice      float64           `json:"total_price"`
	CreatedAt       time.Time         `json:"created_at"`
	PaymentMethodID string            `json:"payment_method_id"`
}

type BoardingPass struct {
	PassengerName string    `json:"passenger_name"`
	FlightNumber  string    `json:"flight_number"`
	Seat          string    `json:"seat"`
	Gate          string    `json:"gate"`
	BoardingTime  time.Time `json:"boarding_time"`
	QRCode        string    `json:"qr_code"`
}

// Database represents our in-memory database
type Database struct {
	Flights      map[string]Flight      `json:"flights"`
	Reservations map[string]Reservation `json:"reservations"`
	Passengers   map[string]Passenger   `json:"passengers"`
	mu           sync.RWMutex
}

// Global database instance
var db *Database

// Custom errors
var (
	ErrFlightNotFound      = errors.New("flight not found")
	ErrReservationNotFound = errors.New("reservation not found")
	ErrPassengerNotFound   = errors.New("passenger not found")
	ErrInvalidInput        = errors.New("invalid input")
)

// Database operations
func (d *Database) GetFlight(flightNumber string) (Flight, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	flight, exists := d.Flights[flightNumber]
	if !exists {
		return Flight{}, ErrFlightNotFound
	}
	return flight, nil
}

func (d *Database) GetReservation(code string) (Reservation, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	reservation, exists := d.Reservations[code]
	if !exists {
		return Reservation{}, ErrReservationNotFound
	}
	return reservation, nil
}

func (d *Database) CreateReservation(reservation Reservation) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Reservations[reservation.ReservationCode] = reservation
	return nil
}

// HTTP Handlers
func searchFlights(c *fiber.Ctx) error {
	origin := c.Query("origin")
	destination := c.Query("destination")
	departureDate := c.Query("departure_date")

	if origin == "" || destination == "" || departureDate == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "origin, destination, and departure_date are required",
		})
	}

	// Parse departure date
	date, err := time.Parse("2006-01-02", departureDate)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid date format",
		})
	}

	var availableFlights []Flight
	db.mu.RLock()
	for _, flight := range db.Flights {
		if flight.Origin.Code == origin &&
			flight.Destination.Code == destination &&
			flight.DepartureTime.Format("2006-01-02") == date.Format("2006-01-02") &&
			flight.AvailableSeats > 0 {
			availableFlights = append(availableFlights, flight)
		}
	}
	db.mu.RUnlock()

	return c.JSON(availableFlights)
}

func getUserReservations(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	var userReservations []Reservation
	db.mu.RLock()
	for _, reservation := range db.Reservations {
		if reservation.Passenger.Email == email {
			userReservations = append(userReservations, reservation)
		}
	}
	db.mu.RUnlock()

	return c.JSON(userReservations)
}

type CreateReservationRequest struct {
	FlightNumbers   []string  `json:"flight_numbers"`
	Passenger       Passenger `json:"passenger"`
	PaymentMethodID string    `json:"payment_method_id"`
}

func createReservation(c *fiber.Ctx) error {
	var req CreateReservationRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate flights exist and have available seats
	var totalPrice float64
	var flights []Flight
	for _, flightNumber := range req.FlightNumbers {
		flight, err := db.GetFlight(flightNumber)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Flight not found: " + flightNumber,
			})
		}
		if flight.AvailableSeats <= 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "No available seats on flight: " + flightNumber,
			})
		}
		totalPrice += flight.Price
		flights = append(flights, flight)
	}

	// Create reservation
	reservation := Reservation{
		ReservationCode: "RES" + uuid.New().String()[:8],
		Passenger:       req.Passenger,
		Flights:         flights,
		Status:          ReservationStatusConfirmed,
		TotalPrice:      totalPrice,
		CreatedAt:       time.Now(),
		PaymentMethodID: req.PaymentMethodID,
	}

	if err := db.CreateReservation(reservation); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create reservation",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(reservation)
}

type CheckInRequest struct {
	ReservationCode string `json:"reservation_code"`
	Email           string `json:"email"`
}

func checkIn(c *fiber.Ctx) error {
	var req CheckInRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Get reservation
	reservation, err := db.GetReservation(req.ReservationCode)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Reservation not found",
		})
	}

	// Verify passenger email
	if reservation.Passenger.Email != req.Email {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Email does not match reservation",
		})
	}

	// Check if already checked in
	if reservation.Status == ReservationStatusCheckedIn {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Already checked in",
		})
	}

	// Generate boarding pass
	boardingPass := BoardingPass{
		PassengerName: reservation.Passenger.FirstName + " " + reservation.Passenger.LastName,
		FlightNumber:  reservation.Flights[0].FlightNumber,
		Seat:          "12A", // In a real system, this would be assigned dynamically
		Gate:          "B12",
		BoardingTime:  reservation.Flights[0].DepartureTime.Add(-30 * time.Minute),
		QRCode:        generateQRCode(reservation.ReservationCode),
	}

	// Update reservation status
	db.mu.Lock()
	reservation.Status = ReservationStatusCheckedIn
	db.Reservations[reservation.ReservationCode] = reservation
	db.mu.Unlock()

	return c.JSON(boardingPass)
}

func generateQRCode(reservationCode string) string {
	// In a real system, this would generate an actual QR code
	return "data:image/png;base64,QR_CODE_DATA_FOR_" + reservationCode
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Flights:      make(map[string]Flight),
		Reservations: make(map[string]Reservation),
		Passengers:   make(map[string]Passenger),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Flight routes
	api.Get("/flights/search", searchFlights)

	// Reservation routes
	api.Get("/reservations", getUserReservations)
	api.Post("/reservations", createReservation)
	api.Get("/reservations/:code", func(c *fiber.Ctx) error {
		code := c.Params("code")
		reservation, err := db.GetReservation(code)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.JSON(reservation)
	})

	// Check-in routes
	api.Post("/check-in", checkIn)
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

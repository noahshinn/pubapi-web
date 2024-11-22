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

type Passenger struct {
	Email            string `json:"email"`
	FirstName        string `json:"first_name"`
	LastName         string `json:"last_name"`
	FrequentFlyerNum string `json:"frequent_flyer_number"`
	SeatPreference   string `json:"seat_preference"`
	PassportNumber   string `json:"passport_number,omitempty"`
	PassportExpiry   string `json:"passport_expiry,omitempty"`
	TSAPrecheck      string `json:"tsa_precheck,omitempty"`
}

type Flight struct {
	FlightNumber   string    `json:"flight_number"`
	Origin         Airport   `json:"origin"`
	Destination    Airport   `json:"destination"`
	DepartureTime  time.Time `json:"departure_time"`
	ArrivalTime    time.Time `json:"arrival_time"`
	AircraftType   string    `json:"aircraft_type"`
	AvailableSeats int       `json:"available_seats"`
	Price          float64   `json:"price"`
	Status         string    `json:"status"`
}

type Seat struct {
	Number      string `json:"number"`
	Class       string `json:"class"`
	Available   bool   `json:"available"`
	ExtraLeg    bool   `json:"extra_leg_room"`
	WindowAisle string `json:"window_aisle"`
}

type ReservationStatus string

const (
	ReservationConfirmed ReservationStatus = "confirmed"
	ReservationCancelled ReservationStatus = "cancelled"
	ReservationCheckedIn ReservationStatus = "checked_in"
)

type Reservation struct {
	ReservationNumber string            `json:"reservation_number"`
	Passenger         Passenger         `json:"passenger"`
	Flights           []Flight          `json:"flights"`
	Seats             []Seat            `json:"seats"`
	Status            ReservationStatus `json:"status"`
	TotalPrice        float64           `json:"total_price"`
	PaymentMethodID   string            `json:"payment_method_id"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

type BoardingPass struct {
	PassengerName string    `json:"passenger_name"`
	FlightNumber  string    `json:"flight_number"`
	Seat          string    `json:"seat"`
	BoardingGroup string    `json:"boarding_group"`
	Gate          string    `json:"gate"`
	BoardingTime  time.Time `json:"boarding_time"`
	QRCode        string    `json:"qr_code"`
}

// Database represents our in-memory database
type Database struct {
	Passengers     map[string]Passenger    `json:"passengers"`
	Flights        map[string]Flight       `json:"flights"`
	Reservations   map[string]Reservation  `json:"reservations"`
	BoardingPasses map[string]BoardingPass `json:"boarding_passes"`
	mu             sync.RWMutex
}

var db *Database

// Error definitions
var (
	ErrFlightNotFound      = errors.New("flight not found")
	ErrPassengerNotFound   = errors.New("passenger not found")
	ErrReservationNotFound = errors.New("reservation not found")
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

func (d *Database) GetPassenger(email string) (Passenger, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	passenger, exists := d.Passengers[email]
	if !exists {
		return Passenger{}, ErrPassengerNotFound
	}
	return passenger, nil
}

func (d *Database) CreateReservation(res Reservation) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Reservations[res.ReservationNumber] = res
	return nil
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

	// Parse departure date
	depDate, err := time.Parse("2006-01-02", departureDate)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid date format",
		})
	}

	var availableFlights []Flight
	db.mu.RLock()
	for _, flight := range db.Flights {
		if flight.Origin.Code == origin &&
			flight.Destination.Code == destination &&
			flight.DepartureTime.Format("2006-01-02") == depDate.Format("2006-01-02") &&
			flight.AvailableSeats > 0 {
			availableFlights = append(availableFlights, flight)
		}
	}
	db.mu.RUnlock()

	return c.JSON(availableFlights)
}

func getReservations(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email is required",
		})
	}

	var userReservations []Reservation
	db.mu.RLock()
	for _, res := range db.Reservations {
		if res.Passenger.Email == email {
			userReservations = append(userReservations, res)
		}
	}
	db.mu.RUnlock()

	return c.JSON(userReservations)
}

type NewReservationRequest struct {
	FlightNumbers   []string `json:"flight_numbers"`
	PassengerEmail  string   `json:"passenger_email"`
	PaymentMethodID string   `json:"payment_method_id"`
	SeatPreferences []string `json:"seat_preferences"`
}

func createReservation(c *fiber.Ctx) error {
	var req NewReservationRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate passenger
	passenger, err := db.GetPassenger(req.PassengerEmail)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Passenger not found",
		})
	}

	// Validate and collect flights
	var flights []Flight
	var totalPrice float64

	for _, flightNum := range req.FlightNumbers {
		flight, err := db.GetFlight(flightNum)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Flight not found: " + flightNum,
			})
		}

		if flight.AvailableSeats <= 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "No available seats on flight: " + flightNum,
			})
		}

		flights = append(flights, flight)
		totalPrice += flight.Price
	}

	// Create reservation
	reservation := Reservation{
		ReservationNumber: "RES-" + uuid.New().String()[:8],
		Passenger:         passenger,
		Flights:           flights,
		Status:            ReservationConfirmed,
		TotalPrice:        totalPrice,
		PaymentMethodID:   req.PaymentMethodID,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if err := db.CreateReservation(reservation); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create reservation",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(reservation)
}

type CheckInRequest struct {
	ReservationNumber string `json:"reservation_number"`
	Email             string `json:"email"`
}

func checkIn(c *fiber.Ctx) error {
	var req CheckInRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	reservation, exists := db.Reservations[req.ReservationNumber]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Reservation not found",
		})
	}

	if reservation.Passenger.Email != req.Email {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Unauthorized access to reservation",
		})
	}

	if reservation.Status == ReservationCancelled {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot check in for cancelled reservation",
		})
	}

	if reservation.Status == ReservationCheckedIn {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Already checked in",
		})
	}

	// Generate boarding pass
	boardingPass := BoardingPass{
		PassengerName: reservation.Passenger.FirstName + " " + reservation.Passenger.LastName,
		FlightNumber:  reservation.Flights[0].FlightNumber,
		Seat:          "Auto-assigned", // In a real system, this would use seat allocation logic
		BoardingGroup: "B",
		Gate:          "A12",
		BoardingTime:  reservation.Flights[0].DepartureTime.Add(-30 * time.Minute),
		QRCode:        generateQRCode(reservation.ReservationNumber),
	}

	// Update reservation status
	reservation.Status = ReservationCheckedIn
	reservation.UpdatedAt = time.Now()
	db.Reservations[reservation.ReservationNumber] = reservation

	// Store boarding pass
	db.BoardingPasses[reservation.ReservationNumber] = boardingPass

	return c.JSON(boardingPass)
}

func generateQRCode(reservationNumber string) string {
	// In a real system, this would generate an actual QR code
	return "QR_" + reservationNumber
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Passengers:     make(map[string]Passenger),
		Flights:        make(map[string]Flight),
		Reservations:   make(map[string]Reservation),
		BoardingPasses: make(map[string]BoardingPass),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Flight routes
	api.Get("/flights/search", searchFlights)

	// Reservation routes
	api.Get("/reservations", getReservations)
	api.Post("/reservations", createReservation)

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

	app.Use(logger.New())
	app.Use(recover.New())
	app.Use(cors.New())

	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

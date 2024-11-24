package main

import (
	"encoding/json"
	"flag"
	"fmt"
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
	Origin         string    `json:"origin"`
	Destination    string    `json:"destination"`
	DepartureTime  time.Time `json:"departure_time"`
	ArrivalTime    time.Time `json:"arrival_time"`
	AircraftType   string    `json:"aircraft_type"`
	AvailableSeats int       `json:"available_seats"`
	Price          float64   `json:"price"`
	CabinClasses   []string  `json:"cabin_classes"`
}

type Passenger struct {
	Email               string    `json:"email"`
	FirstName           string    `json:"first_name"`
	LastName            string    `json:"last_name"`
	DateOfBirth         time.Time `json:"date_of_birth"`
	FrequentFlyerNumber string    `json:"frequent_flyer_number"`
	PassportNumber      string    `json:"passport_number"`
}

type Reservation struct {
	ReservationNumber string    `json:"reservation_number"`
	Passenger         Passenger `json:"passenger"`
	Flights           []Flight  `json:"flights"`
	Status            string    `json:"status"`
	TotalPrice        float64   `json:"total_price"`
	CabinClass        string    `json:"cabin_class"`
	CreatedAt         time.Time `json:"created_at"`
	CheckedIn         bool      `json:"checked_in"`
	SeatAssignments   []string  `json:"seat_assignments"`
}

type BoardingPass struct {
	PassengerName string    `json:"passenger_name"`
	FlightNumber  string    `json:"flight_number"`
	Seat          string    `json:"seat"`
	Gate          string    `json:"gate"`
	BoardingTime  time.Time `json:"boarding_time"`
	QRCode        string    `json:"qr_code"`
}

type Database struct {
	Flights      map[string]Flight      `json:"flights"`
	Reservations map[string]Reservation `json:"reservations"`
	Passengers   map[string]Passenger   `json:"passengers"`
	mu           sync.RWMutex
}

var db *Database

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

func searchFlights(c *fiber.Ctx) error {
	origin := c.Query("origin")
	destination := c.Query("destination")
	departureDate := c.Query("departure_date")
	passengers := c.QueryInt("passengers", 1)

	if origin == "" || destination == "" || departureDate == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Missing required parameters",
		})
	}

	depDate, err := time.Parse("2006-01-02", departureDate)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid date format",
		})
	}

	var availableFlights []Flight
	db.mu.RLock()
	for _, flight := range db.Flights {
		if flight.Origin == origin &&
			flight.Destination == destination &&
			flight.DepartureTime.Format("2006-01-02") == depDate.Format("2006-01-02") &&
			flight.AvailableSeats >= passengers {
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
			"error": "Email is required",
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

type NewReservationRequest struct {
	FlightNumbers   []string  `json:"flight_numbers"`
	Passenger       Passenger `json:"passenger"`
	CabinClass      string    `json:"cabin_class"`
	PaymentMethodID string    `json:"payment_method_id"`
}

func createReservation(c *fiber.Ctx) error {
	var req NewReservationRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate flights exist and have availability
	var totalPrice float64
	var bookedFlights []Flight

	db.mu.RLock()
	for _, flightNum := range req.FlightNumbers {
		flight, exists := db.Flights[flightNum]
		if !exists {
			db.mu.RUnlock()
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": fmt.Sprintf("Flight %s not found", flightNum),
			})
		}
		if flight.AvailableSeats < 1 {
			db.mu.RUnlock()
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": fmt.Sprintf("Flight %s is fully booked", flightNum),
			})
		}
		totalPrice += flight.Price
		bookedFlights = append(bookedFlights, flight)
	}
	db.mu.RUnlock()

	// Create reservation
	reservation := Reservation{
		ReservationNumber: fmt.Sprintf("RES%s", uuid.New().String()[:8]),
		Passenger:         req.Passenger,
		Flights:           bookedFlights,
		Status:            "confirmed",
		TotalPrice:        totalPrice,
		CabinClass:        req.CabinClass,
		CreatedAt:         time.Now(),
		CheckedIn:         false,
	}

	// Update database
	db.mu.Lock()
	db.Reservations[reservation.ReservationNumber] = reservation
	// Update available seats
	for _, flight := range bookedFlights {
		f := db.Flights[flight.FlightNumber]
		f.AvailableSeats--
		db.Flights[flight.FlightNumber] = f
	}
	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(reservation)
}

type CheckInRequest struct {
	ReservationNumber string `json:"reservation_number"`
	LastName          string `json:"last_name"`
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

	if reservation.Passenger.LastName != req.LastName {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Last name does not match reservation",
		})
	}

	if reservation.CheckedIn {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Already checked in",
		})
	}

	// Generate boarding pass
	boardingPass := BoardingPass{
		PassengerName: fmt.Sprintf("%s %s", reservation.Passenger.FirstName, reservation.Passenger.LastName),
		FlightNumber:  reservation.Flights[0].FlightNumber,
		Seat:          "12A", // In a real system, this would be assigned dynamically
		Gate:          "B12",
		BoardingTime:  reservation.Flights[0].DepartureTime.Add(-30 * time.Minute),
		QRCode:        fmt.Sprintf("BP-%s-%s", reservation.ReservationNumber, reservation.Flights[0].FlightNumber),
	}

	// Update reservation status
	reservation.CheckedIn = true
	db.Reservations[req.ReservationNumber] = reservation

	return c.JSON(boardingPass)
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

	// Flight routes
	api.Get("/flights/search", searchFlights)

	// Reservation routes
	api.Get("/reservations", getUserReservations)
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

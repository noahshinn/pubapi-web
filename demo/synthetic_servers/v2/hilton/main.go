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

type Address struct {
	Street     string  `json:"street"`
	City       string  `json:"city"`
	State      string  `json:"state"`
	Country    string  `json:"country"`
	PostalCode string  `json:"postal_code"`
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
}

type RoomType struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	Capacity      int     `json:"capacity"`
	PricePerNight float64 `json:"price_per_night"`
	Available     bool    `json:"available"`
}

type Hotel struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Address   Address    `json:"address"`
	Rating    float64    `json:"rating"`
	Amenities []string   `json:"amenities"`
	RoomTypes []RoomType `json:"room_types"`
	Images    []string   `json:"images"`
}

type Reservation struct {
	ID         string    `json:"id"`
	UserEmail  string    `json:"user_email"`
	Hotel      Hotel     `json:"hotel"`
	RoomType   RoomType  `json:"room_type"`
	CheckIn    string    `json:"check_in"`
	CheckOut   string    `json:"check_out"`
	Guests     int       `json:"guests"`
	TotalPrice float64   `json:"total_price"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

type Rewards struct {
	UserEmail        string    `json:"user_email"`
	Tier             string    `json:"tier"`
	Points           int       `json:"points"`
	NightsStayed     int       `json:"nights_stayed"`
	NightsToNextTier int       `json:"nights_to_next_tier"`
	MemberSince      time.Time `json:"member_since"`
}

type User struct {
	Email   string  `json:"email"`
	Name    string  `json:"name"`
	Phone   string  `json:"phone"`
	Address Address `json:"address"`
	Rewards Rewards `json:"rewards"`
}

type Database struct {
	Users        map[string]User        `json:"users"`
	Hotels       map[string]Hotel       `json:"hotels"`
	Reservations map[string]Reservation `json:"reservations"`
	mu           sync.RWMutex
}

var db *Database

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:        make(map[string]User),
		Hotels:       make(map[string]Hotel),
		Reservations: make(map[string]Reservation),
	}

	return json.Unmarshal(data, db)
}

func searchHotels(c *fiber.Ctx) error {
	location := c.Query("location")
	checkIn := c.Query("check_in")
	checkOut := c.Query("check_out")
	guests := c.QueryInt("guests", 1)

	if location == "" || checkIn == "" || checkOut == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Missing required parameters",
		})
	}

	var availableHotels []Hotel
	db.mu.RLock()
	for _, hotel := range db.Hotels {
		// Simple search by city or state
		if hotel.Address.City == location || hotel.Address.State == location {
			// Check if hotel has available rooms for the given criteria
			for _, room := range hotel.RoomTypes {
				if room.Available && room.Capacity >= guests {
					availableHotels = append(availableHotels, hotel)
					break
				}
			}
		}
	}
	db.mu.RUnlock()

	return c.JSON(availableHotels)
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
		if reservation.UserEmail == email {
			userReservations = append(userReservations, reservation)
		}
	}
	db.mu.RUnlock()

	return c.JSON(userReservations)
}

func createReservation(c *fiber.Ctx) error {
	var newReservation struct {
		HotelID         string `json:"hotel_id"`
		RoomTypeID      string `json:"room_type_id"`
		UserEmail       string `json:"user_email"`
		CheckIn         string `json:"check_in"`
		CheckOut        string `json:"check_out"`
		Guests          int    `json:"guests"`
		SpecialRequests string `json:"special_requests"`
	}

	if err := c.BodyParser(&newReservation); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.RLock()
	hotel, exists := db.Hotels[newReservation.HotelID]
	if !exists {
		db.mu.RUnlock()
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Hotel not found",
		})
	}

	var selectedRoom RoomType
	roomFound := false
	for _, room := range hotel.RoomTypes {
		if room.ID == newReservation.RoomTypeID && room.Available {
			selectedRoom = room
			roomFound = true
			break
		}
	}
	db.mu.RUnlock()

	if !roomFound {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Room type not available",
		})
	}

	checkIn, _ := time.Parse("2006-01-02", newReservation.CheckIn)
	checkOut, _ := time.Parse("2006-01-02", newReservation.CheckOut)
	nights := checkOut.Sub(checkIn).Hours() / 24

	reservation := Reservation{
		ID:         uuid.New().String(),
		UserEmail:  newReservation.UserEmail,
		Hotel:      hotel,
		RoomType:   selectedRoom,
		CheckIn:    newReservation.CheckIn,
		CheckOut:   newReservation.CheckOut,
		Guests:     newReservation.Guests,
		TotalPrice: selectedRoom.PricePerNight * float64(nights),
		Status:     "confirmed",
		CreatedAt:  time.Now(),
	}

	db.mu.Lock()
	db.Reservations[reservation.ID] = reservation
	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(reservation)
}

func getRewards(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email is required",
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

	return c.JSON(user.Rewards)
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

	// Hotel routes
	api.Get("/hotels", searchHotels)

	// Reservation routes
	api.Get("/reservations", getUserReservations)
	api.Post("/reservations", createReservation)

	// Rewards routes
	api.Get("/rewards", getRewards)
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

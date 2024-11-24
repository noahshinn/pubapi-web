package main

import (
	"encoding/json"
	"errors"
	"flag"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/google/uuid"
)

// Domain Models
type Address struct {
	Street    string  `json:"street"`
	City      string  `json:"city"`
	State     string  `json:"state"`
	ZipCode   string  `json:"zip_code"`
	Country   string  `json:"country"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type Room struct {
	Type         string   `json:"type"`
	Price        float64  `json:"price"`
	Beds         string   `json:"beds"`
	MaxOccupancy int      `json:"max_occupancy"`
	Description  string   `json:"description"`
	Amenities    []string `json:"amenities"`
}

type Hotel struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Address     Address  `json:"address"`
	Rating      float64  `json:"rating"`
	Amenities   []string `json:"amenities"`
	Rooms       []Room   `json:"rooms"`
	Description string   `json:"description"`
	Images      []string `json:"images"`
	PhoneNumber string   `json:"phone_number"`
}

type BookingStatus string

const (
	BookingStatusConfirmed BookingStatus = "confirmed"
	BookingStatusCancelled BookingStatus = "cancelled"
	BookingStatusCompleted BookingStatus = "completed"
)

type Booking struct {
	ID         string        `json:"id"`
	UserEmail  string        `json:"user_email"`
	Hotel      Hotel         `json:"hotel"`
	RoomType   string        `json:"room_type"`
	CheckIn    time.Time     `json:"check_in"`
	CheckOut   time.Time     `json:"check_out"`
	Guests     int           `json:"guests"`
	Status     BookingStatus `json:"status"`
	TotalPrice float64       `json:"total_price"`
	CreatedAt  time.Time     `json:"created_at"`
	UpdatedAt  time.Time     `json:"updated_at"`
}

type RewardsTier string

const (
	TierBlue    RewardsTier = "blue"
	TierSilver  RewardsTier = "silver"
	TierGold    RewardsTier = "gold"
	TierDiamond RewardsTier = "diamond"
)

type Rewards struct {
	UserEmail      string      `json:"user_email"`
	Tier           RewardsTier `json:"tier"`
	Points         int         `json:"points"`
	NightsThisYear int         `json:"nights_this_year"`
	Benefits       []string    `json:"benefits"`
	JoinDate       time.Time   `json:"join_date"`
}

type User struct {
	Email   string  `json:"email"`
	Name    string  `json:"name"`
	Phone   string  `json:"phone"`
	Address Address `json:"address"`
	Rewards Rewards `json:"rewards"`
}

// Database represents our in-memory database
type Database struct {
	Users    map[string]User    `json:"users"`
	Hotels   map[string]Hotel   `json:"hotels"`
	Bookings map[string]Booking `json:"bookings"`
	mu       sync.RWMutex
}

// Global database instance
var db *Database

// Error definitions
var (
	ErrUserNotFound    = errors.New("user not found")
	ErrHotelNotFound   = errors.New("hotel not found")
	ErrBookingNotFound = errors.New("booking not found")
	ErrInvalidInput    = errors.New("invalid input")
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

func (d *Database) GetHotel(id string) (Hotel, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	hotel, exists := d.Hotels[id]
	if !exists {
		return Hotel{}, ErrHotelNotFound
	}
	return hotel, nil
}

func (d *Database) CreateBooking(booking Booking) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Bookings[booking.ID] = booking
	return nil
}

// HTTP Handlers
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

	if checkOutDate.Before(checkInDate) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Check-out date must be after check-in date",
		})
	}

	var availableHotels []Hotel
	db.mu.RLock()
	for _, hotel := range db.Hotels {
		// Simple location-based filtering (in real implementation, use proper geo-search)
		if containsIgnoreCase(hotel.Address.City, location) ||
			containsIgnoreCase(hotel.Address.State, location) ||
			containsIgnoreCase(hotel.Address.Country, location) {
			// Check if hotel has rooms that can accommodate the guests
			for _, room := range hotel.Rooms {
				if room.MaxOccupancy >= guests {
					availableHotels = append(availableHotels, hotel)
					break
				}
			}
		}
	}
	db.mu.RUnlock()

	return c.JSON(availableHotels)
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

func createBooking(c *fiber.Ctx) error {
	var req struct {
		HotelID   string `json:"hotel_id"`
		RoomType  string `json:"room_type"`
		CheckIn   string `json:"check_in"`
		CheckOut  string `json:"check_out"`
		Guests    int    `json:"guests"`
		UserEmail string `json:"user_email"`
	}

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

	// Validate hotel
	hotel, err := db.GetHotel(req.HotelID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Hotel not found",
		})
	}

	// Validate dates
	checkIn, err := time.Parse("2006-01-02", req.CheckIn)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid check-in date format",
		})
	}

	checkOut, err := time.Parse("2006-01-02", req.CheckOut)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid check-out date format",
		})
	}

	// Find requested room
	var selectedRoom *Room
	for _, room := range hotel.Rooms {
		if room.Type == req.RoomType && room.MaxOccupancy >= req.Guests {
			selectedRoom = &room
			break
		}
	}

	if selectedRoom == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "No suitable room available",
		})
	}

	// Calculate total price
	nights := checkOut.Sub(checkIn).Hours() / 24
	totalPrice := selectedRoom.Price * float64(nights)

	// Create booking
	booking := Booking{
		ID:         uuid.New().String(),
		UserEmail:  req.UserEmail,
		Hotel:      hotel,
		RoomType:   req.RoomType,
		CheckIn:    checkIn,
		CheckOut:   checkOut,
		Guests:     req.Guests,
		Status:     BookingStatusConfirmed,
		TotalPrice: totalPrice,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	// Save booking
	if err := db.CreateBooking(booking); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create booking",
		})
	}

	// Update user's rewards
	user.Rewards.Points += int(totalPrice)
	user.Rewards.NightsThisYear += int(nights)
	db.Users[user.Email] = user

	return c.Status(fiber.StatusCreated).JSON(booking)
}

func getRewardsStatus(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email parameter is required",
		})
	}

	user, err := db.GetUser(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	return c.JSON(user.Rewards)
}

func containsIgnoreCase(s, substr string) bool {
	s, substr = strings.ToLower(s), strings.ToLower(substr)
	return strings.Contains(s, substr)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:    make(map[string]User),
		Hotels:   make(map[string]Hotel),
		Bookings: make(map[string]Booking),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Hotel routes
	api.Get("/hotels", searchHotels)
	api.Get("/hotels/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		hotel, err := db.GetHotel(id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.JSON(hotel)
	})

	// Booking routes
	api.Get("/bookings", getUserBookings)
	api.Post("/bookings", createBooking)

	// Rewards routes
	api.Get("/rewards", getRewardsStatus)
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

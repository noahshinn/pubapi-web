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
type Theater struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Address   string   `json:"address"`
	City      string   `json:"city"`
	State     string   `json:"state"`
	ZIP       string   `json:"zip"`
	Latitude  float64  `json:"latitude"`
	Longitude float64  `json:"longitude"`
	Amenities []string `json:"amenities"`
}

type Movie struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Rating      string    `json:"rating"`
	Runtime     int       `json:"runtime"`
	Genre       string    `json:"genre"`
	Synopsis    string    `json:"synopsis"`
	PosterURL   string    `json:"poster_url"`
	TrailerURL  string    `json:"trailer_url"`
	ReleaseDate time.Time `json:"release_date"`
}

type Showtime struct {
	ID             string    `json:"id"`
	MovieID        string    `json:"movie_id"`
	TheaterID      string    `json:"theater_id"`
	StartTime      time.Time `json:"start_time"`
	EndTime        time.Time `json:"end_time"`
	Screen         string    `json:"screen"`
	Format         string    `json:"format"`
	Price          float64   `json:"price"`
	AvailableSeats int       `json:"available_seats"`
}

type Ticket struct {
	ID           string    `json:"id"`
	Showtime     Showtime  `json:"showtime"`
	Movie        Movie     `json:"movie"`
	Theater      Theater   `json:"theater"`
	UserEmail    string    `json:"user_email"`
	SeatCount    int       `json:"seat_count"`
	TotalPrice   float64   `json:"total_price"`
	PurchaseDate time.Time `json:"purchase_date"`
	QRCode       string    `json:"qr_code"`
}

type User struct {
	Email          string    `json:"email"`
	Name           string    `json:"name"`
	PaymentMethods []Payment `json:"payment_methods"`
}

type Payment struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Last4 string `json:"last4"`
}

// Database represents our in-memory database
type Database struct {
	Users     map[string]User     `json:"users"`
	Theaters  map[string]Theater  `json:"theaters"`
	Movies    map[string]Movie    `json:"movies"`
	Showtimes map[string]Showtime `json:"showtimes"`
	Tickets   map[string]Ticket   `json:"tickets"`
	mu        sync.RWMutex
}

// Global database instance
var db *Database

// Error definitions
var (
	ErrUserNotFound     = errors.New("user not found")
	ErrTheaterNotFound  = errors.New("theater not found")
	ErrMovieNotFound    = errors.New("movie not found")
	ErrShowtimeNotFound = errors.New("showtime not found")
	ErrInvalidInput     = errors.New("invalid input")
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

func (d *Database) GetTheater(id string) (Theater, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	theater, exists := d.Theaters[id]
	if !exists {
		return Theater{}, ErrTheaterNotFound
	}
	return theater, nil
}

func (d *Database) GetMovie(id string) (Movie, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	movie, exists := d.Movies[id]
	if !exists {
		return Movie{}, ErrMovieNotFound
	}
	return movie, nil
}

func (d *Database) GetShowtime(id string) (Showtime, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	showtime, exists := d.Showtimes[id]
	if !exists {
		return Showtime{}, ErrShowtimeNotFound
	}
	return showtime, nil
}

func (d *Database) CreateTicket(ticket Ticket) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Tickets[ticket.ID] = ticket
	return nil
}

// Handlers
func getTheaters(c *fiber.Ctx) error {
	lat := c.QueryFloat("latitude", 0)
	lon := c.QueryFloat("longitude", 0)

	if lat == 0 || lon == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "latitude and longitude are required",
		})
	}

	var nearbyTheaters []Theater
	maxDistance := 50.0 // Maximum radius in km

	db.mu.RLock()
	for _, theater := range db.Theaters {
		distance := calculateDistance(lat, lon, theater.Latitude, theater.Longitude)
		if distance <= maxDistance {
			nearbyTheaters = append(nearbyTheaters, theater)
		}
	}
	db.mu.RUnlock()

	return c.JSON(nearbyTheaters)
}

func getMovies(c *fiber.Ctx) error {
	theaterID := c.Query("theater_id")

	db.mu.RLock()
	defer db.mu.RUnlock()

	var movies []Movie
	if theaterID != "" {
		// Get movies showing at specific theater
		movieIDs := make(map[string]bool)
		for _, showtime := range db.Showtimes {
			if showtime.TheaterID == theaterID {
				movieIDs[showtime.MovieID] = true
			}
		}

		for movieID := range movieIDs {
			if movie, exists := db.Movies[movieID]; exists {
				movies = append(movies, movie)
			}
		}
	} else {
		// Get all current movies
		for _, movie := range db.Movies {
			movies = append(movies, movie)
		}
	}

	return c.JSON(movies)
}

func getShowtimes(c *fiber.Ctx) error {
	movieID := c.Query("movie_id")
	theaterID := c.Query("theater_id")
	dateStr := c.Query("date")

	if movieID == "" || theaterID == "" || dateStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "movie_id, theater_id, and date are required",
		})
	}

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid date format",
		})
	}

	var showtimes []Showtime
	db.mu.RLock()
	for _, showtime := range db.Showtimes {
		if showtime.MovieID == movieID &&
			showtime.TheaterID == theaterID &&
			showtime.StartTime.Format("2006-01-02") == date.Format("2006-01-02") {
			showtimes = append(showtimes, showtime)
		}
	}
	db.mu.RUnlock()

	return c.JSON(showtimes)
}

type PurchaseTicketRequest struct {
	ShowtimeID      string `json:"showtime_id"`
	UserEmail       string `json:"user_email"`
	SeatCount       int    `json:"seat_count"`
	PaymentMethodID string `json:"payment_method_id"`
}

func purchaseTickets(c *fiber.Ctx) error {
	var req PurchaseTicketRequest
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
		if pm.ID == req.PaymentMethodID {
			validPayment = true
			break
		}
	}
	if !validPayment {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid payment method",
		})
	}

	// Get showtime
	showtime, err := db.GetShowtime(req.ShowtimeID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Validate seat availability
	if showtime.AvailableSeats < req.SeatCount {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Not enough seats available",
		})
	}

	// Get movie and theater info
	movie, err := db.GetMovie(showtime.MovieID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to get movie information",
		})
	}

	theater, err := db.GetTheater(showtime.TheaterID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to get theater information",
		})
	}

	// Create ticket
	ticket := Ticket{
		ID:           uuid.New().String(),
		Showtime:     showtime,
		Movie:        movie,
		Theater:      theater,
		UserEmail:    req.UserEmail,
		SeatCount:    req.SeatCount,
		TotalPrice:   showtime.Price * float64(req.SeatCount),
		PurchaseDate: time.Now(),
		QRCode:       generateQRCode(),
	}

	if err := db.CreateTicket(ticket); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create ticket",
		})
	}

	// Update available seats
	db.mu.Lock()
	showtime.AvailableSeats -= req.SeatCount
	db.Showtimes[showtime.ID] = showtime
	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(ticket)
}

func getTicketHistory(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	// Verify user exists
	if _, err := db.GetUser(email); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	var userTickets []Ticket
	db.mu.RLock()
	for _, ticket := range db.Tickets {
		if ticket.UserEmail == email {
			userTickets = append(userTickets, ticket)
		}
	}
	db.mu.RUnlock()

	return c.JSON(userTickets)
}

// Helper functions
func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	// Simplified distance calculation
	return ((lat2 - lat1) * (lat2 - lat1)) + ((lon2 - lon1) * (lon2 - lon1))
}

func generateQRCode() string {
	return uuid.New().String() // Simplified QR code generation
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:     make(map[string]User),
		Theaters:  make(map[string]Theater),
		Movies:    make(map[string]Movie),
		Showtimes: make(map[string]Showtime),
		Tickets:   make(map[string]Ticket),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	api.Get("/theaters", getTheaters)
	api.Get("/movies", getMovies)
	api.Get("/showtimes", getShowtimes)
	api.Post("/tickets", purchaseTickets)
	api.Get("/tickets/history", getTicketHistory)
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

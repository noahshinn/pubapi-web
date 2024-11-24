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
type Movie struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Rating      string    `json:"rating"`
	Duration    int       `json:"duration"`
	Genre       string    `json:"genre"`
	Synopsis    string    `json:"synopsis"`
	PosterURL   string    `json:"posterUrl"`
	TrailerURL  string    `json:"trailerUrl"`
	ReleaseDate time.Time `json:"releaseDate"`
}

type Theater struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Address   string   `json:"address"`
	City      string   `json:"city"`
	State     string   `json:"state"`
	ZipCode   string   `json:"zipCode"`
	Amenities []string `json:"amenities"`
}

type Showtime struct {
	ID             string    `json:"id"`
	MovieID        string    `json:"movieId"`
	TheaterID      string    `json:"theaterId"`
	DateTime       time.Time `json:"datetime"`
	ScreenNumber   int       `json:"screenNumber"`
	AvailableSeats int       `json:"availableSeats"`
	Price          float64   `json:"price"`
}

type Ticket struct {
	ID           string    `json:"id"`
	Movie        Movie     `json:"movie"`
	Theater      Theater   `json:"theater"`
	Showtime     Showtime  `json:"showtime"`
	SeatNumbers  []string  `json:"seatNumbers"`
	UserEmail    string    `json:"userEmail"`
	PurchaseDate time.Time `json:"purchaseDate"`
	TotalPrice   float64   `json:"totalPrice"`
	QRCode       string    `json:"qrCode"`
}

type Database struct {
	Movies    map[string]Movie    `json:"movies"`
	Theaters  map[string]Theater  `json:"theaters"`
	Showtimes map[string]Showtime `json:"showtimes"`
	Tickets   map[string]Ticket   `json:"tickets"`
	mu        sync.RWMutex
}

var (
	ErrMovieNotFound    = errors.New("movie not found")
	ErrTheaterNotFound  = errors.New("theater not found")
	ErrShowtimeNotFound = errors.New("showtime not found")
	ErrInvalidInput     = errors.New("invalid input")
)

var db *Database

// Database operations
func (d *Database) GetMovie(id string) (Movie, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	movie, exists := d.Movies[id]
	if !exists {
		return Movie{}, ErrMovieNotFound
	}
	return movie, nil
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

	// Update available seats
	showtime := d.Showtimes[ticket.Showtime.ID]
	showtime.AvailableSeats -= len(ticket.SeatNumbers)
	d.Showtimes[ticket.Showtime.ID] = showtime

	return nil
}

// HTTP Handlers
func getMovies(c *fiber.Ctx) error {
	zipCode := c.Query("zipCode")
	if zipCode == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "zipCode is required",
		})
	}

	dateStr := c.Query("date")
	var filterDate time.Time
	var err error
	if dateStr != "" {
		filterDate, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "invalid date format",
			})
		}
	}

	var movies []Movie
	db.mu.RLock()
	for _, movie := range db.Movies {
		if dateStr != "" {
			if movie.ReleaseDate.After(filterDate) {
				continue
			}
		}
		movies = append(movies, movie)
	}
	db.mu.RUnlock()

	return c.JSON(movies)
}

func getTheaters(c *fiber.Ctx) error {
	zipCode := c.Query("zipCode")
	if zipCode == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "zipCode is required",
		})
	}

	var theaters []Theater
	db.mu.RLock()
	for _, theater := range db.Theaters {
		if theater.ZipCode == zipCode {
			theaters = append(theaters, theater)
		}
	}
	db.mu.RUnlock()

	return c.JSON(theaters)
}

func getShowtimes(c *fiber.Ctx) error {
	movieId := c.Query("movieId")
	theaterId := c.Query("theaterId")
	dateStr := c.Query("date")

	if movieId == "" || theaterId == "" || dateStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "movieId, theaterId, and date are required",
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
		if showtime.MovieID == movieId &&
			showtime.TheaterID == theaterId &&
			showtime.DateTime.Format("2006-01-02") == date.Format("2006-01-02") {
			showtimes = append(showtimes, showtime)
		}
	}
	db.mu.RUnlock()

	return c.JSON(showtimes)
}

type PurchaseTicketRequest struct {
	ShowtimeID    string   `json:"showtimeId"`
	Email         string   `json:"email"`
	Quantity      int      `json:"quantity"`
	SeatNumbers   []string `json:"seatNumbers"`
	PaymentMethod string   `json:"paymentMethod"`
}

func purchaseTickets(c *fiber.Ctx) error {
	var req PurchaseTicketRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate request
	if req.ShowtimeID == "" || req.Email == "" || req.Quantity <= 0 ||
		len(req.SeatNumbers) != req.Quantity {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request parameters",
		})
	}

	// Get showtime
	showtime, err := db.GetShowtime(req.ShowtimeID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Check seat availability
	if showtime.AvailableSeats < req.Quantity {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Not enough seats available",
		})
	}

	// Get movie and theater info
	movie, err := db.GetMovie(showtime.MovieID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	theater, err := db.GetTheater(showtime.TheaterID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Create ticket
	ticket := Ticket{
		ID:           uuid.New().String(),
		Movie:        movie,
		Theater:      theater,
		Showtime:     showtime,
		SeatNumbers:  req.SeatNumbers,
		UserEmail:    req.Email,
		PurchaseDate: time.Now(),
		TotalPrice:   float64(req.Quantity) * showtime.Price,
		QRCode:       generateQRCode(),
	}

	// Save ticket
	if err := db.CreateTicket(ticket); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create ticket",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(ticket)
}

func getUserTickets(c *fiber.Ctx) error {
	email := c.Params("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	var tickets []Ticket
	db.mu.RLock()
	for _, ticket := range db.Tickets {
		if ticket.UserEmail == email {
			tickets = append(tickets, ticket)
		}
	}
	db.mu.RUnlock()

	return c.JSON(tickets)
}

func generateQRCode() string {
	// In a real implementation, this would generate an actual QR code
	return uuid.New().String()
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Movies:    make(map[string]Movie),
		Theaters:  make(map[string]Theater),
		Showtimes: make(map[string]Showtime),
		Tickets:   make(map[string]Ticket),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	api.Get("/movies", getMovies)
	api.Get("/theaters", getTheaters)
	api.Get("/showtimes", getShowtimes)
	api.Post("/tickets", purchaseTickets)
	api.Get("/tickets/:email", getUserTickets)
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

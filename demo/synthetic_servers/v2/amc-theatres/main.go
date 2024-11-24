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

// Models
type Location struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type Theater struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Address   string   `json:"address"`
	City      string   `json:"city"`
	State     string   `json:"state"`
	ZipCode   string   `json:"zipCode"`
	Location  Location `json:"location"`
	Screens   int      `json:"screens"`
	Amenities []string `json:"amenities"`
}

type Movie struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Rating      string    `json:"rating"`
	Runtime     int       `json:"runtime"`
	Genre       string    `json:"genre"`
	Synopsis    string    `json:"synopsis"`
	PosterURL   string    `json:"posterUrl"`
	TrailerURL  string    `json:"trailerUrl"`
	ReleaseDate time.Time `json:"releaseDate"`
}

type Showtime struct {
	ID             string    `json:"id"`
	MovieID        string    `json:"movieId"`
	TheaterID      string    `json:"theaterId"`
	ScreenNumber   int       `json:"screenNumber"`
	StartTime      time.Time `json:"startTime"`
	EndTime        time.Time `json:"endTime"`
	Format         string    `json:"format"`
	Price          float64   `json:"price"`
	AvailableSeats int       `json:"availableSeats"`
}

type Seat struct {
	Row    string `json:"row"`
	Number int    `json:"number"`
}

type Reservation struct {
	ID            string    `json:"id"`
	UserEmail     string    `json:"userEmail"`
	ShowtimeID    string    `json:"showtimeId"`
	Seats         []Seat    `json:"seats"`
	TotalPrice    float64   `json:"totalPrice"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"createdAt"`
	PaymentMethod string    `json:"paymentMethod"`
}

// Database
type Database struct {
	Theaters     map[string]Theater     `json:"theaters"`
	Movies       map[string]Movie       `json:"movies"`
	Showtimes    map[string]Showtime    `json:"showtimes"`
	Reservations map[string]Reservation `json:"reservations"`
	mu           sync.RWMutex
}

var db *Database

// Handlers
func getTheaters(c *fiber.Ctx) error {
	lat := c.QueryFloat("latitude", 0)
	lon := c.QueryFloat("longitude", 0)

	if lat == 0 || lon == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "latitude and longitude are required",
		})
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	var nearbyTheaters []Theater
	maxDistance := 50.0 // Maximum radius in km

	for _, theater := range db.Theaters {
		distance := calculateDistance(lat, lon,
			theater.Location.Latitude,
			theater.Location.Longitude)

		if distance <= maxDistance {
			nearbyTheaters = append(nearbyTheaters, theater)
		}
	}

	return c.JSON(nearbyTheaters)
}

func getMovies(c *fiber.Ctx) error {
	theaterId := c.Query("theaterId")

	db.mu.RLock()
	defer db.mu.RUnlock()

	if theaterId == "" {
		// Return all movies
		movies := make([]Movie, 0, len(db.Movies))
		for _, movie := range db.Movies {
			movies = append(movies, movie)
		}
		return c.JSON(movies)
	}

	// Filter movies by theater
	var theaterMovies []Movie
	movieIDs := make(map[string]bool)

	for _, showtime := range db.Showtimes {
		if showtime.TheaterID == theaterId {
			if movie, exists := db.Movies[showtime.MovieID]; exists {
				if !movieIDs[movie.ID] {
					theaterMovies = append(theaterMovies, movie)
					movieIDs[movie.ID] = true
				}
			}
		}
	}

	return c.JSON(theaterMovies)
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

	db.mu.RLock()
	defer db.mu.RUnlock()

	var filteredShowtimes []Showtime
	for _, showtime := range db.Showtimes {
		if showtime.MovieID == movieId &&
			showtime.TheaterID == theaterId &&
			isSameDay(showtime.StartTime, date) {
			filteredShowtimes = append(filteredShowtimes, showtime)
		}
	}

	return c.JSON(filteredShowtimes)
}

func createReservation(c *fiber.Ctx) error {
	var request struct {
		ShowtimeID    string `json:"showtimeId"`
		UserEmail     string `json:"userEmail"`
		Seats         []Seat `json:"seats"`
		PaymentMethod string `json:"paymentMethod"`
	}

	if err := c.BodyParser(&request); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	// Validate showtime exists
	showtime, exists := db.Showtimes[request.ShowtimeID]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Showtime not found",
		})
	}

	// Check seat availability
	if len(request.Seats) > showtime.AvailableSeats {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Not enough available seats",
		})
	}

	// Create reservation
	reservation := Reservation{
		ID:            uuid.New().String(),
		UserEmail:     request.UserEmail,
		ShowtimeID:    request.ShowtimeID,
		Seats:         request.Seats,
		TotalPrice:    float64(len(request.Seats)) * showtime.Price,
		Status:        "confirmed",
		CreatedAt:     time.Now(),
		PaymentMethod: request.PaymentMethod,
	}

	// Update available seats
	showtime.AvailableSeats -= len(request.Seats)
	db.Showtimes[showtime.ID] = showtime

	// Save reservation
	db.Reservations[reservation.ID] = reservation

	return c.Status(fiber.StatusCreated).JSON(reservation)
}

func getUserReservations(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	var userReservations []Reservation
	for _, reservation := range db.Reservations {
		if reservation.UserEmail == email {
			userReservations = append(userReservations, reservation)
		}
	}

	return c.JSON(userReservations)
}

// Helper functions
func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	// Simplified distance calculation
	return ((lat2 - lat1) * (lat2 - lat1)) + ((lon2 - lon1) * (lon2 - lon1))
}

func isSameDay(t1, t2 time.Time) bool {
	y1, m1, d1 := t1.Date()
	y2, m2, d2 := t2.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Theaters:     make(map[string]Theater),
		Movies:       make(map[string]Movie),
		Showtimes:    make(map[string]Showtime),
		Reservations: make(map[string]Reservation),
	}

	return json.Unmarshal(data, db)
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

	// Theater routes
	api.Get("/theaters", getTheaters)

	// Movie routes
	api.Get("/movies", getMovies)

	// Showtime routes
	api.Get("/showtimes", getShowtimes)

	// Reservation routes
	api.Post("/reservations", createReservation)
	api.Get("/reservations", getUserReservations)
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

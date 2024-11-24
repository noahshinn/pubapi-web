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
	"github.com/google/uuid"
)

type Movie struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	DurationMins int      `json:"duration_mins"`
	Rating       string   `json:"rating"`
	Genre        string   `json:"genre"`
	ReleaseDate  string   `json:"release_date"`
	PosterURL    string   `json:"poster_url"`
	TrailerURL   string   `json:"trailer_url"`
	Cast         []string `json:"cast"`
}

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

type Showtime struct {
	ID             string    `json:"id"`
	MovieID        string    `json:"movie_id"`
	TheaterID      string    `json:"theater_id"`
	StartTime      time.Time `json:"start_time"`
	EndTime        time.Time `json:"end_time"`
	Screen         string    `json:"screen"`
	Price          float64   `json:"price"`
	AvailableSeats int       `json:"available_seats"`
}

type Ticket struct {
	ID           string    `json:"id"`
	Showtime     Showtime  `json:"showtime"`
	UserEmail    string    `json:"user_email"`
	SeatCount    int       `json:"seat_count"`
	TotalPrice   float64   `json:"total_price"`
	PurchaseDate time.Time `json:"purchase_date"`
	QRCode       string    `json:"qr_code"`
}

type Database struct {
	Movies    map[string]Movie    `json:"movies"`
	Theaters  map[string]Theater  `json:"theaters"`
	Showtimes map[string]Showtime `json:"showtimes"`
	Tickets   map[string]Ticket   `json:"tickets"`
	mu        sync.RWMutex
}

var db *Database

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

func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	// Simplified distance calculation
	return ((lat2 - lat1) * (lat2 - lat1)) + ((lon2 - lon1) * (lon2 - lon1))
}

func getMovies(c *fiber.Ctx) error {
	status := c.Query("status", "now_playing")

	db.mu.RLock()
	defer db.mu.RUnlock()

	var movies []Movie
	currentDate := time.Now()

	for _, movie := range db.Movies {
		releaseDate, _ := time.Parse("2006-01-02", movie.ReleaseDate)

		if status == "now_playing" && releaseDate.Before(currentDate) {
			movies = append(movies, movie)
		} else if status == "coming_soon" && releaseDate.After(currentDate) {
			movies = append(movies, movie)
		}
	}

	return c.JSON(movies)
}

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
		distance := calculateDistance(lat, lon, theater.Latitude, theater.Longitude)
		if distance <= maxDistance {
			nearbyTheaters = append(nearbyTheaters, theater)
		}
	}

	return c.JSON(nearbyTheaters)
}

func getShowtimes(c *fiber.Ctx) error {
	movieID := c.Query("movie_id")
	theaterID := c.Query("theater_id")
	dateStr := c.Query("date")

	if movieID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "movie_id is required",
		})
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	var filteredShowtimes []Showtime
	for _, showtime := range db.Showtimes {
		if showtime.MovieID != movieID {
			continue
		}

		if theaterID != "" && showtime.TheaterID != theaterID {
			continue
		}

		if dateStr != "" {
			date, err := time.Parse("2006-01-02", dateStr)
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"error": "invalid date format",
				})
			}

			if !showtime.StartTime.Truncate(24 * time.Hour).Equal(date) {
				continue
			}
		}

		filteredShowtimes = append(filteredShowtimes, showtime)
	}

	return c.JSON(filteredShowtimes)
}

func purchaseTickets(c *fiber.Ctx) error {
	var request struct {
		ShowtimeID      string `json:"showtime_id"`
		UserEmail       string `json:"user_email"`
		SeatCount       int    `json:"seat_count"`
		PaymentMethodID string `json:"payment_method_id"`
	}

	if err := c.BodyParser(&request); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	showtime, exists := db.Showtimes[request.ShowtimeID]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Showtime not found",
		})
	}

	if showtime.AvailableSeats < request.SeatCount {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Not enough available seats",
		})
	}

	// Create ticket
	ticket := Ticket{
		ID:           uuid.New().String(),
		Showtime:     showtime,
		UserEmail:    request.UserEmail,
		SeatCount:    request.SeatCount,
		TotalPrice:   showtime.Price * float64(request.SeatCount),
		PurchaseDate: time.Now(),
		QRCode:       fmt.Sprintf("qr_%s", uuid.New().String()),
	}

	// Update available seats
	showtime.AvailableSeats -= request.SeatCount
	db.Showtimes[request.ShowtimeID] = showtime

	// Save ticket
	db.Tickets[ticket.ID] = ticket

	return c.Status(fiber.StatusCreated).JSON(ticket)
}

func getUserTickets(c *fiber.Ctx) error {
	email := c.Params("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	var userTickets []Ticket
	for _, ticket := range db.Tickets {
		if ticket.UserEmail == email {
			userTickets = append(userTickets, ticket)
		}
	}

	return c.JSON(userTickets)
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

	// Movie routes
	api.Get("/movies", getMovies)

	// Theater routes
	api.Get("/theaters", getTheaters)

	// Showtime routes
	api.Get("/showtimes", getShowtimes)

	// Ticket routes
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
	app.Use(cors.New())

	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

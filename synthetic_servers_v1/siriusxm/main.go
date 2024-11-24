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
)

// Domain Models
type Station struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Category     string    `json:"category"`
	Channel      string    `json:"channel"`
	Description  string    `json:"description"`
	StreamingURL string    `json:"streaming_url"`
	CreatedAt    time.Time `json:"created_at"`
}

type Episode struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Duration    int       `json:"duration"`
	AirDate     time.Time `json:"air_date"`
	Description string    `json:"description"`
	AudioURL    string    `json:"audio_url"`
}

type Show struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Host        string    `json:"host"`
	Category    string    `json:"category"`
	Episodes    []Episode `json:"episodes"`
}

type NowPlaying struct {
	StationID string    `json:"station_id"`
	Track     string    `json:"track"`
	Artist    string    `json:"artist"`
	Show      string    `json:"show"`
	StartedAt time.Time `json:"started_at"`
	EndsAt    time.Time `json:"ends_at"`
}

type User struct {
	Email            string    `json:"email"`
	Name             string    `json:"name"`
	SubscriptionPlan string    `json:"subscription_plan"`
	FavoriteStations []string  `json:"favorite_stations"`
	FavoriteShows    []string  `json:"favorite_shows"`
	LastLogin        time.Time `json:"last_login"`
}

// Database represents our in-memory database
type Database struct {
	Users      map[string]User       `json:"users"`
	Stations   map[string]Station    `json:"stations"`
	Shows      map[string]Show       `json:"shows"`
	NowPlaying map[string]NowPlaying `json:"now_playing"`
	mu         sync.RWMutex
}

// Custom errors
var (
	ErrUserNotFound    = errors.New("user not found")
	ErrStationNotFound = errors.New("station not found")
	ErrShowNotFound    = errors.New("show not found")
	ErrInvalidInput    = errors.New("invalid input")
	ErrUnauthorized    = errors.New("unauthorized")
)

// Global database instance
var db *Database

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

func (d *Database) GetStation(id string) (Station, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	station, exists := d.Stations[id]
	if !exists {
		return Station{}, ErrStationNotFound
	}
	return station, nil
}

func (d *Database) GetShow(id string) (Show, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	show, exists := d.Shows[id]
	if !exists {
		return Show{}, ErrShowNotFound
	}
	return show, nil
}

func (d *Database) AddToFavorites(email string, contentID string, contentType string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	user, exists := d.Users[email]
	if !exists {
		return ErrUserNotFound
	}

	switch contentType {
	case "station":
		if _, exists := d.Stations[contentID]; !exists {
			return ErrStationNotFound
		}
		user.FavoriteStations = append(user.FavoriteStations, contentID)
	case "show":
		if _, exists := d.Shows[contentID]; !exists {
			return ErrShowNotFound
		}
		user.FavoriteShows = append(user.FavoriteShows, contentID)
	default:
		return ErrInvalidInput
	}

	d.Users[email] = user
	return nil
}

// HTTP Handlers
func getStations(c *fiber.Ctx) error {
	category := c.Query("category")

	var stations []Station
	db.mu.RLock()
	for _, station := range db.Stations {
		if category == "" || station.Category == category {
			stations = append(stations, station)
		}
	}
	db.mu.RUnlock()

	return c.JSON(stations)
}

func getNowPlaying(c *fiber.Ctx) error {
	stationId := c.Params("stationId")
	if stationId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Station ID is required",
		})
	}

	db.mu.RLock()
	nowPlaying, exists := db.NowPlaying[stationId]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Station not found",
		})
	}

	return c.JSON(nowPlaying)
}

func getFavorites(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email is required",
		})
	}

	user, err := db.GetUser(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	var favoriteStations []Station
	var favoriteShows []Show

	db.mu.RLock()
	for _, stationID := range user.FavoriteStations {
		if station, exists := db.Stations[stationID]; exists {
			favoriteStations = append(favoriteStations, station)
		}
	}

	for _, showID := range user.FavoriteShows {
		if show, exists := db.Shows[showID]; exists {
			favoriteShows = append(favoriteShows, show)
		}
	}
	db.mu.RUnlock()

	return c.JSON(fiber.Map{
		"stations": favoriteStations,
		"shows":    favoriteShows,
	})
}

type AddFavoriteRequest struct {
	UserEmail   string `json:"user_email"`
	ContentID   string `json:"content_id"`
	ContentType string `json:"content_type"`
}

func addToFavorites(c *fiber.Ctx) error {
	var req AddFavoriteRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if err := db.AddToFavorites(req.UserEmail, req.ContentID, req.ContentType); err != nil {
		status := fiber.StatusInternalServerError
		if errors.Is(err, ErrUserNotFound) || errors.Is(err, ErrStationNotFound) || errors.Is(err, ErrShowNotFound) {
			status = fiber.StatusNotFound
		} else if errors.Is(err, ErrInvalidInput) {
			status = fiber.StatusBadRequest
		}

		return c.Status(status).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Added to favorites successfully",
	})
}

func getShows(c *fiber.Ctx) error {
	category := c.Query("category")

	var shows []Show
	db.mu.RLock()
	for _, show := range db.Shows {
		if category == "" || show.Category == category {
			shows = append(shows, show)
		}
	}
	db.mu.RUnlock()

	return c.JSON(shows)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:      make(map[string]User),
		Stations:   make(map[string]Station),
		Shows:      make(map[string]Show),
		NowPlaying: make(map[string]NowPlaying),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Station routes
	api.Get("/stations", getStations)
	api.Get("/stations/:stationId", func(c *fiber.Ctx) error {
		id := c.Params("stationId")
		station, err := db.GetStation(id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.JSON(station)
	})
	api.Get("/stations/:stationId/now-playing", getNowPlaying)

	// Show routes
	api.Get("/shows", getShows)
	api.Get("/shows/:showId", func(c *fiber.Ctx) error {
		id := c.Params("showId")
		show, err := db.GetShow(id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.JSON(show)
	})

	// Favorites routes
	api.Get("/favorites", getFavorites)
	api.Post("/favorites", addToFavorites)

	// User routes
	api.Get("/users/:email", func(c *fiber.Ctx) error {
		email := c.Params("email")
		user, err := db.GetUser(email)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.JSON(user)
	})
}

func main() {
	// Command line flags
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

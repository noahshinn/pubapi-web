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
type Profile struct {
	Email        string      `json:"email"`
	Name         string      `json:"name"`
	Subscription string      `json:"subscription_type"`
	Preferences  Preferences `json:"preferences"`
	CreatedAt    time.Time   `json:"created_at"`
}

type Preferences struct {
	AudioQuality          string `json:"audio_quality"`
	ExplicitContentFilter bool   `json:"explicit_content_filter"`
	DiscoveryLevel        string `json:"discovery_level"`
}

type Station struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	SeedArtist string    `json:"seed_artist"`
	UserEmail  string    `json:"user_email"`
	CreatedAt  time.Time `json:"created_at"`
	LastPlayed time.Time `json:"last_played"`
}

type Track struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Album    string `json:"album"`
	Duration int    `json:"duration"`
	CoverArt string `json:"cover_art"`
	Explicit bool   `json:"explicit"`
}

type Feedback struct {
	TrackID   string    `json:"track_id"`
	UserEmail string    `json:"user_email"`
	Type      string    `json:"type"` // thumbs_up, thumbs_down, skip
	CreatedAt time.Time `json:"created_at"`
}

// Database represents our in-memory database
type Database struct {
	Profiles      map[string]Profile    `json:"profiles"`
	Stations      map[string]Station    `json:"stations"`
	Tracks        map[string]Track      `json:"tracks"`
	Feedback      map[string][]Feedback `json:"feedback"`       // key: track_id
	StationTracks map[string][]string   `json:"station_tracks"` // key: station_id, value: track_ids
	mu            sync.RWMutex
}

// Custom errors
var (
	ErrProfileNotFound = errors.New("profile not found")
	ErrStationNotFound = errors.New("station not found")
	ErrTrackNotFound   = errors.New("track not found")
)

// Global database instance
var db *Database

// Database operations
func (d *Database) GetProfile(email string) (Profile, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	profile, exists := d.Profiles[email]
	if !exists {
		return Profile{}, ErrProfileNotFound
	}
	return profile, nil
}

func (d *Database) GetUserStations(email string) []Station {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var stations []Station
	for _, station := range d.Stations {
		if station.UserEmail == email {
			stations = append(stations, station)
		}
	}
	return stations
}

func (d *Database) CreateStation(station Station) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Stations[station.ID] = station
	return nil
}

func (d *Database) GetStationTracks(stationId string, limit int) ([]Track, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	trackIds, exists := d.StationTracks[stationId]
	if !exists {
		return nil, ErrStationNotFound
	}

	var tracks []Track
	for i := 0; i < limit && i < len(trackIds); i++ {
		if track, exists := d.Tracks[trackIds[i]]; exists {
			tracks = append(tracks, track)
		}
	}

	return tracks, nil
}

func (d *Database) AddFeedback(feedback Feedback) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Feedback[feedback.TrackID] = append(d.Feedback[feedback.TrackID], feedback)
	return nil
}

// HTTP Handlers
func getProfile(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	profile, err := db.GetProfile(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(profile)
}

func getUserStations(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	stations := db.GetUserStations(email)
	return c.JSON(stations)
}

func createStation(c *fiber.Ctx) error {
	var req struct {
		Name       string `json:"name"`
		SeedArtist string `json:"seed_artist"`
		UserEmail  string `json:"user_email"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user exists
	if _, err := db.GetProfile(req.UserEmail); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	station := Station{
		ID:         uuid.New().String(),
		Name:       req.Name,
		SeedArtist: req.SeedArtist,
		UserEmail:  req.UserEmail,
		CreatedAt:  time.Now(),
		LastPlayed: time.Now(),
	}

	if err := db.CreateStation(station); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create station",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(station)
}

func getStationTracks(c *fiber.Ctx) error {
	stationId := c.Params("stationId")
	if stationId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "stationId parameter is required",
		})
	}

	tracks, err := db.GetStationTracks(stationId, 10) // Get next 10 tracks
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(tracks)
}

func submitFeedback(c *fiber.Ctx) error {
	trackId := c.Params("trackId")
	if trackId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "trackId parameter is required",
		})
	}

	var req struct {
		Type      string `json:"type"`
		UserEmail string `json:"user_email"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate feedback type
	if req.Type != "thumbs_up" && req.Type != "thumbs_down" && req.Type != "skip" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid feedback type",
		})
	}

	feedback := Feedback{
		TrackID:   trackId,
		UserEmail: req.UserEmail,
		Type:      req.Type,
		CreatedAt: time.Now(),
	}

	if err := db.AddFeedback(feedback); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to save feedback",
		})
	}

	return c.SendStatus(fiber.StatusOK)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Profiles:      make(map[string]Profile),
		Stations:      make(map[string]Station),
		Tracks:        make(map[string]Track),
		Feedback:      make(map[string][]Feedback),
		StationTracks: make(map[string][]string),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Profile routes
	api.Get("/profile", getProfile)

	// Station routes
	api.Get("/stations", getUserStations)
	api.Post("/stations", createStation)
	api.Get("/stations/:stationId/tracks", getStationTracks)

	// Feedback routes
	api.Post("/tracks/:trackId/feedback", submitFeedback)
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

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
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/google/uuid"
)

// Domain Models
type Track struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Artist     string    `json:"artist"`
	Album      string    `json:"album"`
	DurationMs int       `json:"duration_ms"`
	PreviewURL string    `json:"preview_url"`
	CreatedAt  time.Time `json:"created_at"`
}

type Playlist struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	OwnerEmail  string    `json:"owner_email"`
	Tracks      []Track   `json:"tracks"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type User struct {
	Email          string        `json:"email"`
	Name           string        `json:"name"`
	Premium        bool          `json:"premium"`
	JoinDate       time.Time     `json:"join_date"`
	RecentlyPlayed []PlayHistory `json:"recently_played"`
	SavedTracks    []Track       `json:"saved_tracks"`
}

type PlayHistory struct {
	Track    Track     `json:"track"`
	PlayedAt time.Time `json:"played_at"`
}

// Database represents our in-memory database
type Database struct {
	Users     map[string]User     `json:"users"`
	Tracks    map[string]Track    `json:"tracks"`
	Playlists map[string]Playlist `json:"playlists"`
	mu        sync.RWMutex
}

// Custom errors
var (
	ErrUserNotFound     = errors.New("user not found")
	ErrTrackNotFound    = errors.New("track not found")
	ErrPlaylistNotFound = errors.New("playlist not found")
	ErrUnauthorized     = errors.New("unauthorized")
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

func (d *Database) GetTrack(id string) (Track, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	track, exists := d.Tracks[id]
	if !exists {
		return Track{}, ErrTrackNotFound
	}
	return track, nil
}

func (d *Database) GetPlaylist(id string) (Playlist, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	playlist, exists := d.Playlists[id]
	if !exists {
		return Playlist{}, ErrPlaylistNotFound
	}
	return playlist, nil
}

func (d *Database) CreatePlaylist(playlist Playlist) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Playlists[playlist.ID] = playlist
	return nil
}

func (d *Database) AddTrackToPlaylist(playlistId string, track Track) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	playlist, exists := d.Playlists[playlistId]
	if !exists {
		return ErrPlaylistNotFound
	}

	playlist.Tracks = append(playlist.Tracks, track)
	playlist.UpdatedAt = time.Now()
	d.Playlists[playlistId] = playlist
	return nil
}

func (d *Database) RemoveTrackFromPlaylist(playlistId string, trackId string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	playlist, exists := d.Playlists[playlistId]
	if !exists {
		return ErrPlaylistNotFound
	}

	var newTracks []Track
	for _, track := range playlist.Tracks {
		if track.ID != trackId {
			newTracks = append(newTracks, track)
		}
	}

	playlist.Tracks = newTracks
	playlist.UpdatedAt = time.Now()
	d.Playlists[playlistId] = playlist
	return nil
}

func (d *Database) AddToRecentlyPlayed(email string, track Track) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	user, exists := d.Users[email]
	if !exists {
		return ErrUserNotFound
	}

	playHistory := PlayHistory{
		Track:    track,
		PlayedAt: time.Now(),
	}

	// Keep only last 50 recently played tracks
	if len(user.RecentlyPlayed) >= 50 {
		user.RecentlyPlayed = user.RecentlyPlayed[1:]
	}

	user.RecentlyPlayed = append(user.RecentlyPlayed, playHistory)
	d.Users[email] = user
	return nil
}

// HTTP Handlers
func getUserPlaylists(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	var userPlaylists []Playlist
	db.mu.RLock()
	for _, playlist := range db.Playlists {
		if playlist.OwnerEmail == email {
			userPlaylists = append(userPlaylists, playlist)
		}
	}
	db.mu.RUnlock()

	return c.JSON(userPlaylists)
}

func createPlaylist(c *fiber.Ctx) error {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		OwnerEmail  string `json:"owner_email"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user exists
	if _, err := db.GetUser(req.OwnerEmail); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	playlist := Playlist{
		ID:          uuid.New().String(),
		Name:        req.Name,
		Description: req.Description,
		OwnerEmail:  req.OwnerEmail,
		Tracks:      []Track{},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := db.CreatePlaylist(playlist); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create playlist",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(playlist)
}

func addTrackToPlaylist(c *fiber.Ctx) error {
	playlistId := c.Params("playlistId")
	var req struct {
		TrackID string `json:"track_id"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	track, err := db.GetTrack(req.TrackID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if err := db.AddTrackToPlaylist(playlistId, track); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to add track to playlist",
		})
	}

	return c.SendStatus(fiber.StatusOK)
}

func removeTrackFromPlaylist(c *fiber.Ctx) error {
	playlistId := c.Params("playlistId")
	var req struct {
		TrackID string `json:"track_id"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if err := db.RemoveTrackFromPlaylist(playlistId, req.TrackID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to remove track from playlist",
		})
	}

	return c.SendStatus(fiber.StatusOK)
}

func searchTracks(c *fiber.Ctx) error {
	query := c.Query("query")
	if query == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "query parameter is required",
		})
	}

	var matchingTracks []Track
	db.mu.RLock()
	for _, track := range db.Tracks {
		// Simple case-insensitive search in title, artist, or album
		if strings.Contains(strings.ToLower(track.Title), strings.ToLower(query)) ||
			strings.Contains(strings.ToLower(track.Artist), strings.ToLower(query)) ||
			strings.Contains(strings.ToLower(track.Album), strings.ToLower(query)) {
			matchingTracks = append(matchingTracks, track)
		}
	}
	db.mu.RUnlock()

	return c.JSON(matchingTracks)
}

func getRecentlyPlayed(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	user, err := db.GetUser(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(user.RecentlyPlayed)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:     make(map[string]User),
		Tracks:    make(map[string]Track),
		Playlists: make(map[string]Playlist),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Playlist routes
	api.Get("/playlists", getUserPlaylists)
	api.Post("/playlists", createPlaylist)
	api.Post("/playlists/:playlistId/tracks", addTrackToPlaylist)
	api.Delete("/playlists/:playlistId/tracks", removeTrackFromPlaylist)

	// Track routes
	api.Get("/tracks/search", searchTracks)

	// User routes
	api.Get("/me/recently-played", getRecentlyPlayed)
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

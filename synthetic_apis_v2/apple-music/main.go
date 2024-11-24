package main

import (
	"encoding/json"
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

// Data models
type Artist struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Genres []string `json:"genres"`
}

type Album struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Artist      Artist    `json:"artist"`
	ReleaseDate time.Time `json:"releaseDate"`
	Genre       string    `json:"genre"`
	TrackCount  int       `json:"trackCount"`
}

type Song struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Artist      Artist    `json:"artist"`
	Album       Album     `json:"album"`
	Duration    int       `json:"duration"` // in seconds
	ReleaseDate time.Time `json:"releaseDate"`
	Genre       string    `json:"genre"`
}

type Playlist struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Creator     string    `json:"creator"` // user email
	Songs       []Song    `json:"songs"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type User struct {
	Email     string     `json:"email"`
	Name      string     `json:"name"`
	Playlists []Playlist `json:"playlists"`
	Library   struct {
		Songs []Song `json:"songs"`
	} `json:"library"`
}

// Database represents our in-memory database
type Database struct {
	Users     map[string]User     `json:"users"`
	Songs     map[string]Song     `json:"songs"`
	Albums    map[string]Album    `json:"albums"`
	Artists   map[string]Artist   `json:"artists"`
	Playlists map[string]Playlist `json:"playlists"`
	mu        sync.RWMutex
}

var db *Database

// Database operations
func (d *Database) GetUser(email string) (User, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if user, exists := d.Users[email]; exists {
		return user, nil
	}
	return User{}, fiber.NewError(fiber.StatusNotFound, "User not found")
}

func (d *Database) CreatePlaylist(playlist Playlist) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Playlists[playlist.ID] = playlist

	// Update user's playlists
	if user, exists := d.Users[playlist.Creator]; exists {
		user.Playlists = append(user.Playlists, playlist)
		d.Users[playlist.Creator] = user
	}

	return nil
}

func (d *Database) AddSongToPlaylist(playlistID, songID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	playlist, exists := d.Playlists[playlistID]
	if !exists {
		return fiber.NewError(fiber.StatusNotFound, "Playlist not found")
	}

	song, exists := d.Songs[songID]
	if !exists {
		return fiber.NewError(fiber.StatusNotFound, "Song not found")
	}

	playlist.Songs = append(playlist.Songs, song)
	playlist.UpdatedAt = time.Now()
	d.Playlists[playlistID] = playlist

	return nil
}

// HTTP Handlers
func getUserLibrary(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Email is required")
	}

	user, err := db.GetUser(email)
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{
		"playlists": user.Playlists,
		"songs":     user.Library.Songs,
	})
}

func getPlaylists(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Email is required")
	}

	user, err := db.GetUser(email)
	if err != nil {
		return err
	}

	return c.JSON(user.Playlists)
}

func createPlaylist(c *fiber.Ctx) error {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		UserEmail   string `json:"userEmail"`
	}

	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.Name == "" || req.UserEmail == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Name and user email are required")
	}

	// Verify user exists
	if _, err := db.GetUser(req.UserEmail); err != nil {
		return err
	}

	playlist := Playlist{
		ID:          uuid.New().String(),
		Name:        req.Name,
		Description: req.Description,
		Creator:     req.UserEmail,
		Songs:       []Song{},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := db.CreatePlaylist(playlist); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create playlist")
	}

	return c.Status(fiber.StatusCreated).JSON(playlist)
}

func addSongToPlaylist(c *fiber.Ctx) error {
	playlistID := c.Params("playlistId")
	var req struct {
		SongID string `json:"songId"`
	}

	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if err := db.AddSongToPlaylist(playlistID, req.SongID); err != nil {
		return err
	}

	return c.SendStatus(fiber.StatusOK)
}

func removeSongFromPlaylist(c *fiber.Ctx) error {
	playlistID := c.Params("playlistId")
	songID := c.Query("songId")

	db.mu.Lock()
	defer db.mu.Unlock()

	playlist, exists := db.Playlists[playlistID]
	if !exists {
		return fiber.NewError(fiber.StatusNotFound, "Playlist not found")
	}

	var newSongs []Song
	for _, song := range playlist.Songs {
		if song.ID != songID {
			newSongs = append(newSongs, song)
		}
	}

	playlist.Songs = newSongs
	playlist.UpdatedAt = time.Now()
	db.Playlists[playlistID] = playlist

	return c.SendStatus(fiber.StatusOK)
}

func search(c *fiber.Ctx) error {
	query := c.Query("query")
	searchType := c.Query("type")

	if query == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Search query is required")
	}

	results := fiber.Map{}

	db.mu.RLock()
	defer db.mu.RUnlock()

	switch searchType {
	case "song":
		var songs []Song
		for _, song := range db.Songs {
			if contains(song.Title, query) {
				songs = append(songs, song)
			}
		}
		results["songs"] = songs

	case "album":
		var albums []Album
		for _, album := range db.Albums {
			if contains(album.Title, query) {
				albums = append(albums, album)
			}
		}
		results["albums"] = albums

	case "artist":
		var artists []Artist
		for _, artist := range db.Artists {
			if contains(artist.Name, query) {
				artists = append(artists, artist)
			}
		}
		results["artists"] = artists

	default:
		return fiber.NewError(fiber.StatusBadRequest, "Invalid search type")
	}

	return c.JSON(results)
}

// Helper function for case-insensitive substring search
func contains(s, substr string) bool {
	s, substr = strings.ToLower(s), strings.ToLower(substr)
	return strings.Contains(s, substr)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:     make(map[string]User),
		Songs:     make(map[string]Song),
		Albums:    make(map[string]Album),
		Artists:   make(map[string]Artist),
		Playlists: make(map[string]Playlist),
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

	// Library routes
	api.Get("/library", getUserLibrary)

	// Playlist routes
	api.Get("/playlists", getPlaylists)
	api.Post("/playlists", createPlaylist)
	api.Post("/playlists/:playlistId/songs", addSongToPlaylist)
	api.Delete("/playlists/:playlistId/songs", removeSongFromPlaylist)

	// Search route
	api.Get("/search", search)
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

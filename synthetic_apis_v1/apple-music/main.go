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
type Song struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Artist      string    `json:"artist"`
	Album       string    `json:"album"`
	Duration    int       `json:"duration"`
	Genre       string    `json:"genre"`
	ReleaseDate time.Time `json:"release_date"`
}

type Playlist struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Owner       string    `json:"owner"`
	Songs       []Song    `json:"songs"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Artist struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Genres []string `json:"genres"`
}

type Album struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Artist      string    `json:"artist"`
	ReleaseDate time.Time `json:"release_date"`
	Tracks      []Song    `json:"tracks"`
}

type User struct {
	Email            string     `json:"email"`
	Name             string     `json:"name"`
	Playlists        []Playlist `json:"playlists"`
	LikedSongs       []Song     `json:"liked_songs"`
	SubscriptionType string     `json:"subscription_type"`
	JoinDate         time.Time  `json:"join_date"`
}

// Database represents our in-memory database
type Database struct {
	Users     map[string]User     `json:"users"`
	Songs     map[string]Song     `json:"songs"`
	Artists   map[string]Artist   `json:"artists"`
	Albums    map[string]Album    `json:"albums"`
	Playlists map[string]Playlist `json:"playlists"`
	mu        sync.RWMutex
}

// Custom errors
var (
	ErrUserNotFound     = errors.New("user not found")
	ErrSongNotFound     = errors.New("song not found")
	ErrPlaylistNotFound = errors.New("playlist not found")
	ErrInvalidInput     = errors.New("invalid input")
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

func (d *Database) AddSongToPlaylist(playlistId string, songId string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	playlist, exists := d.Playlists[playlistId]
	if !exists {
		return ErrPlaylistNotFound
	}

	song, exists := d.Songs[songId]
	if !exists {
		return ErrSongNotFound
	}

	playlist.Songs = append(playlist.Songs, song)
	playlist.UpdatedAt = time.Now()
	d.Playlists[playlistId] = playlist

	return nil
}

// HTTP Handlers
func getUserLibrary(c *fiber.Ctx) error {
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

	return c.JSON(fiber.Map{
		"playlists":   user.Playlists,
		"liked_songs": user.LikedSongs,
	})
}

func getUserPlaylists(c *fiber.Ctx) error {
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

	return c.JSON(user.Playlists)
}

type CreatePlaylistRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	UserEmail   string `json:"user_email"`
}

func createPlaylist(c *fiber.Ctx) error {
	var req CreatePlaylistRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.Name == "" || req.UserEmail == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Name and user_email are required",
		})
	}

	user, err := db.GetUser(req.UserEmail)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	playlist := Playlist{
		ID:          uuid.New().String(),
		Name:        req.Name,
		Description: req.Description,
		Owner:       req.UserEmail,
		Songs:       []Song{},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := db.CreatePlaylist(playlist); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create playlist",
		})
	}

	user.Playlists = append(user.Playlists, playlist)
	db.Users[req.UserEmail] = user

	return c.Status(fiber.StatusCreated).JSON(playlist)
}

func addSongToPlaylist(c *fiber.Ctx) error {
	playlistId := c.Params("playlistId")
	var req struct {
		SongId string `json:"song_id"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if err := db.AddSongToPlaylist(playlistId, req.SongId); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	playlist, _ := db.GetPlaylist(playlistId)
	return c.JSON(playlist)
}

func searchMusic(c *fiber.Ctx) error {
	query := c.Query("query")
	searchType := c.Query("type")

	if query == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "query parameter is required",
		})
	}

	var results interface{}
	db.mu.RLock()
	defer db.mu.RUnlock()

	switch searchType {
	case "song":
		var songs []Song
		for _, song := range db.Songs {
			if contains(song.Title, query) || contains(song.Artist, query) {
				songs = append(songs, song)
			}
		}
		results = songs
	case "artist":
		var artists []Artist
		for _, artist := range db.Artists {
			if contains(artist.Name, query) {
				artists = append(artists, artist)
			}
		}
		results = artists
	case "album":
		var albums []Album
		for _, album := range db.Albums {
			if contains(album.Title, query) || contains(album.Artist, query) {
				albums = append(albums, album)
			}
		}
		results = albums
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid search type",
		})
	}

	return c.JSON(fiber.Map{
		"results": results,
	})
}

// Helper functions
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:     make(map[string]User),
		Songs:     make(map[string]Song),
		Artists:   make(map[string]Artist),
		Albums:    make(map[string]Album),
		Playlists: make(map[string]Playlist),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Library routes
	api.Get("/library", getUserLibrary)

	// Playlist routes
	api.Get("/playlists", getUserPlaylists)
	api.Post("/playlists", createPlaylist)
	api.Post("/playlists/:playlistId/songs", addSongToPlaylist)

	// Search routes
	api.Get("/search", searchMusic)
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

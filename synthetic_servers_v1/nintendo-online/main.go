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
	"github.com/gofiber/fiber/v2/middleware/recover"
)

// Data models
type Profile struct {
	Email            string    `json:"email"`
	Nickname         string    `json:"nickname"`
	Avatar           string    `json:"avatar"`
	FriendCode       string    `json:"friend_code"`
	MembershipStatus string    `json:"membership_status"`
	MembershipExpiry time.Time `json:"membership_expiry"`
}

type Friend struct {
	FriendCode   string `json:"friend_code"`
	Nickname     string `json:"nickname"`
	Avatar       string `json:"avatar"`
	OnlineStatus string `json:"online_status"`
	CurrentGame  string `json:"current_game,omitempty"`
}

type Game struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	LastPlayed  time.Time `json:"last_played"`
	PlayTime    int       `json:"play_time"` // in minutes
	HasSaveData bool      `json:"has_save_data"`
}

type SaveData struct {
	GameID     string    `json:"game_id"`
	LastSaved  time.Time `json:"last_saved"`
	CloudSync  bool      `json:"cloud_sync"`
	SaveSize   int       `json:"save_size"` // in bytes
	SaveBlocks []byte    `json:"-"`         // actual save data
}

type OnlineStatus struct {
	Email       string `json:"email"`
	Status      string `json:"status"` // "online", "offline", "playing"
	CurrentGame string `json:"current_game,omitempty"`
}

// Database structure
type Database struct {
	Profiles map[string]Profile             `json:"profiles"`
	Friends  map[string][]Friend            `json:"friends"`
	Games    map[string][]Game              `json:"games"`
	SaveData map[string]map[string]SaveData `json:"save_data"` // email -> gameId -> SaveData
	Status   map[string]OnlineStatus        `json:"status"`
	mu       sync.RWMutex
}

var db *Database

// Handlers
func getProfile(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	db.mu.RLock()
	profile, exists := db.Profiles[email]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "profile not found",
		})
	}

	return c.JSON(profile)
}

func getFriends(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	db.mu.RLock()
	friends, exists := db.Friends[email]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "friends list not found",
		})
	}

	// Update online status for each friend
	for i := range friends {
		db.mu.RLock()
		if status, exists := db.Status[friends[i].FriendCode]; exists {
			friends[i].OnlineStatus = status.Status
			friends[i].CurrentGame = status.CurrentGame
		}
		db.mu.RUnlock()
	}

	return c.JSON(friends)
}

func getGames(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	db.mu.RLock()
	games, exists := db.Games[email]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "games library not found",
		})
	}

	return c.JSON(games)
}

func getSaveData(c *fiber.Ctx) error {
	email := c.Query("email")
	gameId := c.Params("gameId")

	if email == "" || gameId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email and gameId are required",
		})
	}

	db.mu.RLock()
	userSaves, exists := db.SaveData[email]
	if !exists {
		db.mu.RUnlock()
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "no save data found for user",
		})
	}

	saveData, exists := userSaves[gameId]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "no save data found for game",
		})
	}

	return c.JSON(saveData)
}

func updateOnlineStatus(c *fiber.Ctx) error {
	var status OnlineStatus
	if err := c.BodyParser(&status); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	if status.Email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	db.mu.Lock()
	db.Status[status.Email] = status
	db.mu.Unlock()

	return c.JSON(fiber.Map{
		"message": "status updated successfully",
	})
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Profiles: make(map[string]Profile),
		Friends:  make(map[string][]Friend),
		Games:    make(map[string][]Game),
		SaveData: make(map[string]map[string]SaveData),
		Status:   make(map[string]OnlineStatus),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	api.Get("/profile", getProfile)
	api.Get("/friends", getFriends)
	api.Get("/games", getGames)
	api.Get("/games/:gameId/save-data", getSaveData)
	api.Post("/online-status", updateOnlineStatus)
}

func main() {
	// Command line flags
	port := flag.String("port", "3000", "Port to run the server on")
	flag.Parse()

	// Load database
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
	app.Use(cors.New())

	// Setup routes
	setupRoutes(app)

	// Start server
	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

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

// Domain Models
type Profile struct {
	PSNID       string    `json:"psn_id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	AvatarURL   string    `json:"avatar_url"`
	Level       int       `json:"level"`
	TrophyLevel int       `json:"trophy_level"`
	PlusMember  bool      `json:"plus_member"`
	Country     string    `json:"country"`
	CreatedAt   time.Time `json:"created_at"`
}

type Game struct {
	ID             string    `json:"id"`
	Title          string    `json:"title"`
	CoverURL       string    `json:"cover_url"`
	Platform       string    `json:"platform"`
	PurchaseDate   time.Time `json:"purchase_date"`
	LastPlayed     time.Time `json:"last_played"`
	TotalPlayTime  int       `json:"total_play_time"` // in minutes
	CompletionRate float64   `json:"completion_rate"`
}

type TrophyType string

const (
	TrophyBronze   TrophyType = "bronze"
	TrophySilver   TrophyType = "silver"
	TrophyGold     TrophyType = "gold"
	TrophyPlatinum TrophyType = "platinum"
)

type Trophy struct {
	ID          string     `json:"id"`
	GameID      string     `json:"game_id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Type        TrophyType `json:"type"`
	Rarity      string     `json:"rarity"`
	Earned      bool       `json:"earned"`
	EarnedAt    *time.Time `json:"earned_at,omitempty"`
}

type OnlineStatus string

const (
	StatusOnline  OnlineStatus = "online"
	StatusOffline OnlineStatus = "offline"
	StatusAway    OnlineStatus = "away"
	StatusBusy    OnlineStatus = "busy"
)

type Friend struct {
	PSNID        string       `json:"psn_id"`
	DisplayName  string       `json:"display_name"`
	AvatarURL    string       `json:"avatar_url"`
	OnlineStatus OnlineStatus `json:"online_status"`
	CurrentGame  *Game        `json:"current_game,omitempty"`
}

// Database represents our in-memory database
type Database struct {
	Profiles map[string]Profile  `json:"profiles"`
	Games    map[string][]Game   `json:"games"`    // email -> games
	Trophies map[string][]Trophy `json:"trophies"` // email -> trophies
	Friends  map[string][]Friend `json:"friends"`  // email -> friends
	mu       sync.RWMutex
}

var db *Database

// Database operations
func (d *Database) GetProfile(email string) (Profile, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	profile, exists := d.Profiles[email]
	if !exists {
		return Profile{}, fiber.NewError(fiber.StatusNotFound, "Profile not found")
	}
	return profile, nil
}

func (d *Database) GetGames(email string) ([]Game, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	games, exists := d.Games[email]
	if !exists {
		return []Game{}, nil
	}
	return games, nil
}

func (d *Database) GetTrophies(email string, gameID string) ([]Trophy, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	trophies, exists := d.Trophies[email]
	if !exists {
		return []Trophy{}, nil
	}

	if gameID != "" {
		filtered := make([]Trophy, 0)
		for _, trophy := range trophies {
			if trophy.GameID == gameID {
				filtered = append(filtered, trophy)
			}
		}
		return filtered, nil
	}

	return trophies, nil
}

func (d *Database) GetFriends(email string) ([]Friend, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	friends, exists := d.Friends[email]
	if !exists {
		return []Friend{}, nil
	}
	return friends, nil
}

// HTTP Handlers
func getProfile(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Email is required")
	}

	profile, err := db.GetProfile(email)
	if err != nil {
		return err
	}

	return c.JSON(profile)
}

func getGames(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Email is required")
	}

	games, err := db.GetGames(email)
	if err != nil {
		return err
	}

	return c.JSON(games)
}

func getTrophies(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Email is required")
	}

	gameID := c.Query("game_id")
	trophies, err := db.GetTrophies(email, gameID)
	if err != nil {
		return err
	}

	return c.JSON(trophies)
}

func getFriends(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Email is required")
	}

	friends, err := db.GetFriends(email)
	if err != nil {
		return err
	}

	return c.JSON(friends)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Profiles: make(map[string]Profile),
		Games:    make(map[string][]Game),
		Trophies: make(map[string][]Trophy),
		Friends:  make(map[string][]Friend),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	api.Get("/profile", getProfile)
	api.Get("/games/library", getGames)
	api.Get("/trophies", getTrophies)
	api.Get("/friends", getFriends)
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

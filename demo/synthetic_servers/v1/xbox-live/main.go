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
	Email         string `json:"email"`
	Gamertag      string `json:"gamertag"`
	Name          string `json:"name"`
	AvatarURL     string `json:"avatar_url"`
	Gamerscore    int    `json:"gamerscore"`
	Reputation    string `json:"reputation"`
	AccountTier   string `json:"account_tier"`
	YearsWithGold int    `json:"years_with_gold"`
}

type Friend struct {
	Gamertag        string `json:"gamertag"`
	Name            string `json:"name"`
	AvatarURL       string `json:"avatar_url"`
	OnlineStatus    string `json:"online_status"`
	CurrentActivity string `json:"current_activity"`
	Gamerscore      int    `json:"gamerscore"`
}

type AchievementProgress struct {
	Earned int `json:"earned"`
	Total  int `json:"total"`
}

type Game struct {
	ID                  string              `json:"id"`
	Title               string              `json:"title"`
	Description         string              `json:"description"`
	CoverURL            string              `json:"cover_url"`
	LastPlayed          time.Time           `json:"last_played"`
	AchievementProgress AchievementProgress `json:"achievement_progress"`
	InstallStatus       string              `json:"install_status"`
}

type Achievement struct {
	ID               string    `json:"id"`
	GameID           string    `json:"game_id"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	Gamerscore       int       `json:"gamerscore"`
	ImageURL         string    `json:"image_url"`
	Unlocked         bool      `json:"unlocked"`
	UnlockTime       time.Time `json:"unlock_time,omitempty"`
	RarityPercentage float64   `json:"rarity_percentage"`
}

// Database structure
type Database struct {
	Profiles     map[string]Profile                  `json:"profiles"`
	Friends      map[string][]Friend                 `json:"friends"`
	Games        map[string][]Game                   `json:"games"`
	Achievements map[string]map[string][]Achievement `json:"achievements"`
	mu           sync.RWMutex
}

var db *Database

// Handler functions
func getProfile(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
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
			"error": "email parameter is required",
		})
	}

	db.mu.RLock()
	friends, exists := db.Friends[email]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "user not found",
		})
	}

	return c.JSON(friends)
}

func getGames(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	db.mu.RLock()
	games, exists := db.Games[email]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "user not found",
		})
	}

	return c.JSON(games)
}

func getAchievements(c *fiber.Ctx) error {
	email := c.Query("email")
	gameID := c.Query("game_id")

	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	db.mu.RLock()
	userAchievements, exists := db.Achievements[email]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "user not found",
		})
	}

	if gameID != "" {
		// Return achievements for specific game
		achievements, exists := userAchievements[gameID]
		if !exists {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "game not found",
			})
		}
		return c.JSON(achievements)
	}

	// Return all achievements
	var allAchievements []Achievement
	for _, gameAchievements := range userAchievements {
		allAchievements = append(allAchievements, gameAchievements...)
	}

	return c.JSON(allAchievements)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Profiles:     make(map[string]Profile),
		Friends:      make(map[string][]Friend),
		Games:        make(map[string][]Game),
		Achievements: make(map[string]map[string][]Achievement),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	api.Get("/profile", getProfile)
	api.Get("/friends", getFriends)
	api.Get("/games", getGames)
	api.Get("/achievements", getAchievements)
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

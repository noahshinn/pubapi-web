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

type Profile struct {
	Email            string    `json:"email"`
	Nickname         string    `json:"nickname"`
	Avatar           string    `json:"avatar"`
	FriendCode       string    `json:"friend_code"`
	OnlineStatus     string    `json:"online_status"`
	MembershipStatus string    `json:"membership_status"`
	MembershipExpiry time.Time `json:"membership_expiry"`
}

type Game struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	CoverImage    string    `json:"cover_image"`
	TotalPlaytime int       `json:"total_playtime"`
	LastPlayed    time.Time `json:"last_played"`
	Achievements  struct {
		Earned int `json:"earned"`
		Total  int `json:"total"`
	} `json:"achievements"`
}

type Friend struct {
	FriendCode       string `json:"friend_code"`
	Nickname         string `json:"nickname"`
	Avatar           string `json:"avatar"`
	OnlineStatus     string `json:"online_status"`
	CurrentlyPlaying struct {
		Game      string    `json:"game"`
		StartedAt time.Time `json:"started_at"`
	} `json:"currently_playing"`
}

type PlaytimeStats struct {
	TotalPlaytime int `json:"total_playtime"`
	GamesOwned    int `json:"games_owned"`
	MostPlayed    []struct {
		Game     string `json:"game"`
		Playtime int    `json:"playtime"`
	} `json:"most_played"`
}

type Database struct {
	Profiles map[string]Profile       `json:"profiles"`
	Games    map[string][]Game        `json:"games"`
	Friends  map[string][]Friend      `json:"friends"`
	Stats    map[string]PlaytimeStats `json:"stats"`
	mu       sync.RWMutex
}

var db *Database

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Profiles: make(map[string]Profile),
		Games:    make(map[string][]Game),
		Friends:  make(map[string][]Friend),
		Stats:    make(map[string]PlaytimeStats),
	}

	return json.Unmarshal(data, db)
}

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

func addFriend(c *fiber.Ctx) error {
	var req struct {
		UserEmail  string `json:"user_email"`
		FriendCode string `json:"friend_code"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	// Verify user exists
	if _, exists := db.Profiles[req.UserEmail]; !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "user not found",
		})
	}

	// Find friend profile by friend code
	var friendProfile Profile
	var foundFriend bool
	for _, profile := range db.Profiles {
		if profile.FriendCode == req.FriendCode {
			friendProfile = profile
			foundFriend = true
			break
		}
	}

	if !foundFriend {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "friend code not found",
		})
	}

	// Add friend to user's friend list
	newFriend := Friend{
		FriendCode:   friendProfile.FriendCode,
		Nickname:     friendProfile.Nickname,
		Avatar:       friendProfile.Avatar,
		OnlineStatus: friendProfile.OnlineStatus,
	}

	db.Friends[req.UserEmail] = append(db.Friends[req.UserEmail], newFriend)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "friend added successfully",
	})
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

func getPlaytimeStats(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	db.mu.RLock()
	stats, exists := db.Stats[email]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "user not found",
		})
	}

	return c.JSON(stats)
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

	api.Get("/profile", getProfile)
	api.Get("/friends", getFriends)
	api.Post("/friends", addFriend)
	api.Get("/games", getGames)
	api.Get("/playtime", getPlaytimeStats)
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

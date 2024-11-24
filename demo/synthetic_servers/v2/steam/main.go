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
	"github.com/google/uuid"
)

type Game struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Playtime   int       `json:"playtime"`
	LastPlayed time.Time `json:"last_played"`
	Installed  bool      `json:"installed"`
}

type StoreGame struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Price       float64   `json:"price"`
	Discount    float64   `json:"discount"`
	Rating      float64   `json:"rating"`
	Tags        []string  `json:"tags"`
	ReleaseDate time.Time `json:"release_date"`
}

type Friend struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	Status      string    `json:"status"`
	CurrentGame string    `json:"current_game"`
	LastOnline  time.Time `json:"last_online"`
}

type Achievement struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Unlocked    bool      `json:"unlocked"`
	UnlockDate  time.Time `json:"unlock_date,omitempty"`
}

type User struct {
	Email          string                   `json:"email"`
	Username       string                   `json:"username"`
	Library        []Game                   `json:"library"`
	Friends        []Friend                 `json:"friends"`
	Achievements   map[string][]Achievement `json:"achievements"`
	PaymentMethods []PaymentMethod          `json:"payment_methods"`
}

type PaymentMethod struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Last4    string `json:"last4"`
	ExpiryMM int    `json:"expiry_mm"`
	ExpiryYY int    `json:"expiry_yy"`
}

type Purchase struct {
	ID           string    `json:"id"`
	Game         Game      `json:"game"`
	Price        float64   `json:"price"`
	PurchaseDate time.Time `json:"purchase_date"`
}

type Database struct {
	Users      map[string]User      `json:"users"`
	StoreGames map[string]StoreGame `json:"store_games"`
	mu         sync.RWMutex
}

var db *Database

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:      make(map[string]User),
		StoreGames: make(map[string]StoreGame),
	}

	return json.Unmarshal(data, db)
}

func getUserLibrary(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	db.mu.RLock()
	user, exists := db.Users[email]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "user not found",
		})
	}

	return c.JSON(user.Library)
}

func getStoreGames(c *fiber.Ctx) error {
	category := c.Query("category")
	tag := c.Query("tag")

	var games []StoreGame
	db.mu.RLock()
	for _, game := range db.StoreGames {
		if category != "" {
			// Filter by category if specified
			hasCategory := false
			for _, t := range game.Tags {
				if t == category {
					hasCategory = true
					break
				}
			}
			if !hasCategory {
				continue
			}
		}

		if tag != "" {
			// Filter by tag if specified
			hasTag := false
			for _, t := range game.Tags {
				if t == tag {
					hasTag = true
					break
				}
			}
			if !hasTag {
				continue
			}
		}

		games = append(games, game)
	}
	db.mu.RUnlock()

	return c.JSON(games)
}

func getFriends(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	db.mu.RLock()
	user, exists := db.Users[email]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "user not found",
		})
	}

	return c.JSON(user.Friends)
}

func getAchievements(c *fiber.Ctx) error {
	email := c.Query("email")
	gameId := c.Params("gameId")

	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	db.mu.RLock()
	user, exists := db.Users[email]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "user not found",
		})
	}

	achievements, exists := user.Achievements[gameId]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "no achievements found for this game",
		})
	}

	return c.JSON(achievements)
}

type PurchaseRequest struct {
	GameID        string `json:"game_id"`
	UserEmail     string `json:"user_email"`
	PaymentMethod string `json:"payment_method"`
}

func purchaseGame(c *fiber.Ctx) error {
	var req PurchaseRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	// Verify user exists
	user, exists := db.Users[req.UserEmail]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "user not found",
		})
	}

	// Verify game exists
	game, exists := db.StoreGames[req.GameID]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "game not found",
		})
	}

	// Check if user already owns the game
	for _, ownedGame := range user.Library {
		if ownedGame.ID == req.GameID {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "user already owns this game",
			})
		}
	}

	// Verify payment method
	validPayment := false
	for _, pm := range user.PaymentMethods {
		if pm.ID == req.PaymentMethod {
			validPayment = true
			break
		}
	}
	if !validPayment {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid payment method",
		})
	}

	// Create new game entry in user's library
	newGame := Game{
		ID:         game.ID,
		Title:      game.Title,
		Playtime:   0,
		LastPlayed: time.Time{},
		Installed:  false,
	}

	user.Library = append(user.Library, newGame)
	db.Users[req.UserEmail] = user

	// Create purchase record
	purchase := Purchase{
		ID:           uuid.New().String(),
		Game:         newGame,
		Price:        game.Price * (1 - game.Discount),
		PurchaseDate: time.Now(),
	}

	return c.Status(fiber.StatusCreated).JSON(purchase)
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

	api.Get("/library", getUserLibrary)
	api.Get("/store", getStoreGames)
	api.Get("/friends", getFriends)
	api.Get("/achievements/:gameId", getAchievements)
	api.Post("/purchases", purchaseGame)
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
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE",
		AllowHeaders: "Origin, Content-Type, Accept",
	}))

	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

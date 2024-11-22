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
type SystemRequirements struct {
	Minimum     string `json:"minimum"`
	Recommended string `json:"recommended"`
}

type Game struct {
	ID          string             `json:"id"`
	Title       string             `json:"title"`
	Description string             `json:"description"`
	Price       float64            `json:"price"`
	Publisher   string             `json:"publisher"`
	Developer   string             `json:"developer"`
	ReleaseDate time.Time          `json:"release_date"`
	Categories  []string           `json:"categories"`
	Tags        []string           `json:"tags"`
	Rating      float64            `json:"rating"`
	ReviewCount int                `json:"reviews_count"`
	SysReq      SystemRequirements `json:"system_requirements"`
}

type UserGame struct {
	Game         Game      `json:"game"`
	PurchaseDate time.Time `json:"purchase_date"`
	PlayTime     float64   `json:"play_time"` // in hours
	LastPlayed   time.Time `json:"last_played"`
}

type Friend struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	Status      string    `json:"status"` // online, offline, in-game
	CurrentGame string    `json:"current_game"`
	LastOnline  time.Time `json:"last_online"`
}

type User struct {
	Email          string          `json:"email"`
	Username       string          `json:"username"`
	Library        []UserGame      `json:"library"`
	Friends        []Friend        `json:"friends"`
	PaymentMethods []PaymentMethod `json:"payment_methods"`
	WishList       []Game          `json:"wish_list"`
}

type PaymentMethod struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Last4     string    `json:"last4"`
	ExpiryMM  int       `json:"expiry_mm"`
	ExpiryYY  int       `json:"expiry_yy"`
	CreatedAt time.Time `json:"created_at"`
}

type Purchase struct {
	ID              string    `json:"id"`
	UserEmail       string    `json:"user_email"`
	GameID          string    `json:"game_id"`
	Price           float64   `json:"price"`
	PaymentMethodID string    `json:"payment_method_id"`
	PurchaseDate    time.Time `json:"purchase_date"`
}

// Database represents our in-memory database
type Database struct {
	Users     map[string]User     `json:"users"`
	Games     map[string]Game     `json:"games"`
	Purchases map[string]Purchase `json:"purchases"`
	mu        sync.RWMutex
}

var (
	db                *Database
	ErrUserNotFound   = errors.New("user not found")
	ErrGameNotFound   = errors.New("game not found")
	ErrInvalidPayment = errors.New("invalid payment method")
)

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

func (d *Database) GetGame(id string) (Game, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	game, exists := d.Games[id]
	if !exists {
		return Game{}, ErrGameNotFound
	}
	return game, nil
}

func (d *Database) AddPurchase(purchase Purchase) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Add purchase record
	d.Purchases[purchase.ID] = purchase

	// Update user's library
	user := d.Users[purchase.UserEmail]
	game := d.Games[purchase.GameID]

	userGame := UserGame{
		Game:         game,
		PurchaseDate: purchase.PurchaseDate,
		PlayTime:     0,
		LastPlayed:   time.Time{},
	}

	user.Library = append(user.Library, userGame)
	d.Users[purchase.UserEmail] = user

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

	return c.JSON(user.Library)
}

func getStoreGames(c *fiber.Ctx) error {
	category := c.Query("category")
	search := c.Query("search")

	var games []Game
	db.mu.RLock()
	for _, game := range db.Games {
		if category != "" {
			categoryMatch := false
			for _, cat := range game.Categories {
				if cat == category {
					categoryMatch = true
					break
				}
			}
			if !categoryMatch {
				continue
			}
		}

		if search != "" {
			// Simple case-insensitive search
			if !strings.Contains(strings.ToLower(game.Title), strings.ToLower(search)) {
				continue
			}
		}

		games = append(games, game)
	}
	db.mu.RUnlock()

	return c.JSON(games)
}

func getGameDetails(c *fiber.Ctx) error {
	gameId := c.Params("gameId")

	game, err := db.GetGame(gameId)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(game)
}

func getFriends(c *fiber.Ctx) error {
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

	return c.JSON(user.Friends)
}

type PurchaseRequest struct {
	GameID          string `json:"game_id"`
	UserEmail       string `json:"user_email"`
	PaymentMethodID string `json:"payment_method_id"`
}

func purchaseGame(c *fiber.Ctx) error {
	var req PurchaseRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user and payment method
	user, err := db.GetUser(req.UserEmail)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	validPayment := false
	for _, pm := range user.PaymentMethods {
		if pm.ID == req.PaymentMethodID {
			validPayment = true
			break
		}
	}
	if !validPayment {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": ErrInvalidPayment.Error(),
		})
	}

	// Validate game
	game, err := db.GetGame(req.GameID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Check if user already owns the game
	for _, userGame := range user.Library {
		if userGame.Game.ID == req.GameID {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "User already owns this game",
			})
		}
	}

	// Create purchase record
	purchase := Purchase{
		ID:              uuid.New().String(),
		UserEmail:       req.UserEmail,
		GameID:          req.GameID,
		Price:           game.Price,
		PaymentMethodID: req.PaymentMethodID,
		PurchaseDate:    time.Now(),
	}

	if err := db.AddPurchase(purchase); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to process purchase",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(purchase)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:     make(map[string]User),
		Games:     make(map[string]Game),
		Purchases: make(map[string]Purchase),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Library routes
	api.Get("/library", getUserLibrary)

	// Store routes
	api.Get("/store/games", getStoreGames)
	api.Get("/store/games/:gameId", getGameDetails)

	// Friends routes
	api.Get("/friends", getFriends)

	// Purchase routes
	api.Post("/purchases", purchaseGame)
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

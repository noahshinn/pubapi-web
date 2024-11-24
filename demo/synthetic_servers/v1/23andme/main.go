package main

import (
	"encoding/json"
	"errors"
	"flag"
	"log"
	"os"
	"sync"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

// Domain Models
type GeneticProfile struct {
	ID                    string  `json:"id"`
	Haplogroup            string  `json:"haplogroup"`
	NeanderthalPercentage float64 `json:"neanderthal_percentage"`
	SampleID              string  `json:"sample_id"`
	GenotypingChip        string  `json:"genotyping_chip"`
}

type Population struct {
	Population string  `json:"population"`
	Percentage float64 `json:"percentage"`
	Confidence float64 `json:"confidence"`
}

type AncestryComposition struct {
	Populations []Population `json:"populations"`
}

type Relative struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Relationship string  `json:"relationship"`
	SharedDNA    float64 `json:"shared_dna"`
	Segments     int     `json:"segments"`
	Ancestry     string  `json:"ancestry"`
}

type HealthReport struct {
	Trait           string   `json:"trait"`
	Status          string   `json:"status"`
	RiskFactor      float64  `json:"risk_factor"`
	Description     string   `json:"description"`
	Recommendations []string `json:"recommendations"`
}

type User struct {
	Email             string              `json:"email"`
	Name              string              `json:"name"`
	GeneticProfile    GeneticProfile      `json:"genetic_profile"`
	Ancestry          AncestryComposition `json:"ancestry"`
	Relatives         []Relative          `json:"relatives"`
	HealthReports     []HealthReport      `json:"health_reports"`
	ConsentedToHealth bool                `json:"consented_to_health"`
}

// Database represents our in-memory database
type Database struct {
	Users map[string]User `json:"users"`
	mu    sync.RWMutex
}

// Custom errors
var (
	ErrUserNotFound    = errors.New("user not found")
	ErrUnauthorized    = errors.New("unauthorized")
	ErrNoHealthConsent = errors.New("user has not consented to health reports")
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

// HTTP Handlers
func getGeneticProfile(c *fiber.Ctx) error {
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

	return c.JSON(user.GeneticProfile)
}

func getAncestryComposition(c *fiber.Ctx) error {
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

	return c.JSON(user.Ancestry)
}

func getRelatives(c *fiber.Ctx) error {
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

	return c.JSON(user.Relatives)
}

func getHealthReports(c *fiber.Ctx) error {
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

	if !user.ConsentedToHealth {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": ErrNoHealthConsent.Error(),
		})
	}

	return c.JSON(user.HealthReports)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users: make(map[string]User),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Genetic profile routes
	api.Get("/profile", getGeneticProfile)

	// Ancestry routes
	api.Get("/ancestry", getAncestryComposition)

	// Relatives routes
	api.Get("/relatives", getRelatives)

	// Health reports routes
	api.Get("/health-reports", getHealthReports)
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

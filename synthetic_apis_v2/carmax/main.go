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
	"github.com/google/uuid"
)

// Data models
type Car struct {
	ID              string   `json:"id"`
	Make            string   `json:"make"`
	Model           string   `json:"model"`
	Year            int      `json:"year"`
	Price           float64  `json:"price"`
	Mileage         int      `json:"mileage"`
	ExteriorColor   string   `json:"exterior_color"`
	InteriorColor   string   `json:"interior_color"`
	FuelType        string   `json:"fuel_type"`
	Transmission    string   `json:"transmission"`
	Location        string   `json:"location"`
	VIN             string   `json:"vin"`
	Features        []string `json:"features"`
	AccidentHistory string   `json:"accident_history"`
	OwnersCount     int      `json:"owners_count"`
	Images          []string `json:"images"`
}

type User struct {
	Email     string   `json:"email"`
	Name      string   `json:"name"`
	Phone     string   `json:"phone"`
	SavedCars []string `json:"saved_cars"`
}

type Appraisal struct {
	ID          string    `json:"id"`
	OfferAmount float64   `json:"offer_amount"`
	ValidUntil  time.Time `json:"valid_until"`
	Location    string    `json:"location"`
}

type Database struct {
	Cars       map[string]Car       `json:"cars"`
	Users      map[string]User      `json:"users"`
	Appraisals map[string]Appraisal `json:"appraisals"`
	mu         sync.RWMutex
}

var db *Database

// Handler functions
func searchInventory(c *fiber.Ctx) error {
	make := c.Query("make")
	model := c.Query("model")
	yearMin := c.QueryInt("year_min", 0)
	yearMax := c.QueryInt("year_max", 9999)
	priceMin := c.QueryFloat("price_min", 0)
	priceMax := c.QueryFloat("price_max", 999999)

	var results []Car
	db.mu.RLock()
	for _, car := range db.Cars {
		if (make == "" || car.Make == make) &&
			(model == "" || car.Model == model) &&
			car.Year >= yearMin &&
			car.Year <= yearMax &&
			car.Price >= priceMin &&
			car.Price <= priceMax {
			results = append(results, car)
		}
	}
	db.mu.RUnlock()

	return c.JSON(results)
}

func getCarDetails(c *fiber.Ctx) error {
	carID := c.Params("carId")

	db.mu.RLock()
	car, exists := db.Cars[carID]
	db.mu.RUnlock()

	if !exists {
		return c.Status(404).JSON(fiber.Map{
			"error": "Car not found",
		})
	}

	return c.JSON(car)
}

type AppraisalRequest struct {
	Make      string `json:"make"`
	Model     string `json:"model"`
	Year      int    `json:"year"`
	Mileage   int    `json:"mileage"`
	Condition string `json:"condition"`
	ZipCode   string `json:"zip_code"`
}

func createAppraisal(c *fiber.Ctx) error {
	var req AppraisalRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Simple appraisal calculation logic
	baseValue := float64(30000)
	yearFactor := float64(req.Year-2000) * 500
	mileageFactor := float64(req.Mileage) * -0.05

	offerAmount := baseValue + yearFactor + mileageFactor
	if offerAmount < 0 {
		offerAmount = 500 // Minimum offer
	}

	appraisal := Appraisal{
		ID:          uuid.New().String(),
		OfferAmount: offerAmount,
		ValidUntil:  time.Now().Add(7 * 24 * time.Hour),
		Location:    "CarMax " + req.ZipCode,
	}

	db.mu.Lock()
	db.Appraisals[appraisal.ID] = appraisal
	db.mu.Unlock()

	return c.JSON(appraisal)
}

func getSavedCars(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Email is required",
		})
	}

	db.mu.RLock()
	user, exists := db.Users[email]
	if !exists {
		db.mu.RUnlock()
		return c.Status(404).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	var savedCars []Car
	for _, carID := range user.SavedCars {
		if car, exists := db.Cars[carID]; exists {
			savedCars = append(savedCars, car)
		}
	}
	db.mu.RUnlock()

	return c.JSON(savedCars)
}

type SaveCarRequest struct {
	Email string `json:"email"`
	CarID string `json:"carId"`
}

func saveCar(c *fiber.Ctx) error {
	var req SaveCarRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	user, exists := db.Users[req.Email]
	if !exists {
		return c.Status(404).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	if _, exists := db.Cars[req.CarID]; !exists {
		return c.Status(404).JSON(fiber.Map{
			"error": "Car not found",
		})
	}

	// Check if car is already saved
	for _, carID := range user.SavedCars {
		if carID == req.CarID {
			return c.Status(400).JSON(fiber.Map{
				"error": "Car already saved",
			})
		}
	}

	user.SavedCars = append(user.SavedCars, req.CarID)
	db.Users[req.Email] = user

	return c.Status(201).JSON(fiber.Map{
		"message": "Car saved successfully",
	})
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Cars:       make(map[string]Car),
		Users:      make(map[string]User),
		Appraisals: make(map[string]Appraisal),
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

	// Inventory routes
	api.Get("/inventory", searchInventory)
	api.Get("/inventory/:carId", getCarDetails)

	// Appraisal routes
	api.Post("/appraisals", createAppraisal)

	// Saved cars routes
	api.Get("/saved-cars", getSavedCars)
	api.Post("/saved-cars", saveCar)
}

func main() {
	port := flag.String("port", "3000", "Port to run the server on")
	flag.Parse()

	if err := loadDatabase(); err != nil {
		log.Fatal(err)
	}

	app := fiber.New()

	app.Use(logger.New())
	app.Use(cors.New())

	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

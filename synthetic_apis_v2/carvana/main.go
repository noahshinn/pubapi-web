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

// Domain Models
type Vehicle struct {
	ID            string   `json:"id"`
	Make          string   `json:"make"`
	Model         string   `json:"model"`
	Year          int      `json:"year"`
	Price         float64  `json:"price"`
	Mileage       int      `json:"mileage"`
	VIN           string   `json:"vin"`
	ExteriorColor string   `json:"exterior_color"`
	InteriorColor string   `json:"interior_color"`
	Transmission  string   `json:"transmission"`
	FuelType      string   `json:"fuel_type"`
	BodyStyle     string   `json:"body_style"`
	Features      []string `json:"features"`
	Images        []string `json:"images"`
	CarfaxLink    string   `json:"carfax_link"`
	Available     bool     `json:"available"`
}

type FinancingDetails struct {
	TermMonths     int     `json:"term_months"`
	DownPayment    float64 `json:"down_payment"`
	MonthlyPayment float64 `json:"monthly_payment"`
	APR            float64 `json:"apr"`
}

type TradeInRequest struct {
	Make      string `json:"make"`
	Model     string `json:"model"`
	Year      int    `json:"year"`
	Mileage   int    `json:"mileage"`
	Condition string `json:"condition"`
	VIN       string `json:"vin"`
}

type TradeInEstimate struct {
	EstimatedValue       float64 `json:"estimated_value"`
	ConditionAdjustments float64 `json:"condition_adjustments"`
	MarketAdjustments    float64 `json:"market_adjustments"`
	FinalOffer           float64 `json:"final_offer"`
}

type TradeInDetails struct {
	Vehicle  TradeInRequest  `json:"vehicle"`
	Estimate TradeInEstimate `json:"estimate"`
}

type Order struct {
	ID               string            `json:"id"`
	UserEmail        string            `json:"user_email"`
	Vehicle          Vehicle           `json:"vehicle"`
	Status           string            `json:"status"`
	DeliveryDate     time.Time         `json:"delivery_date"`
	DeliveryAddress  string            `json:"delivery_address"`
	PaymentMethod    string            `json:"payment_method"`
	FinancingDetails *FinancingDetails `json:"financing_details,omitempty"`
	TradeIn          *TradeInDetails   `json:"trade_in,omitempty"`
	TotalPrice       float64           `json:"total_price"`
	CreatedAt        time.Time         `json:"created_at"`
}

type Database struct {
	Vehicles map[string]Vehicle `json:"vehicles"`
	Orders   map[string]Order   `json:"orders"`
	mu       sync.RWMutex
}

var db *Database

func searchVehicles(c *fiber.Ctx) error {
	make := c.Query("make")
	model := c.Query("model")
	yearMin := c.QueryInt("year_min", 0)
	yearMax := c.QueryInt("year_max", 9999)
	priceMin := c.QueryFloat("price_min", 0)
	priceMax := c.QueryFloat("price_max", 999999999)

	var results []Vehicle
	db.mu.RLock()
	for _, v := range db.Vehicles {
		if (make == "" || v.Make == make) &&
			(model == "" || v.Model == model) &&
			v.Year >= yearMin &&
			v.Year <= yearMax &&
			v.Price >= priceMin &&
			v.Price <= priceMax &&
			v.Available {
			results = append(results, v)
		}
	}
	db.mu.RUnlock()

	return c.JSON(results)
}

func getVehicle(c *fiber.Ctx) error {
	id := c.Params("vehicleId")

	db.mu.RLock()
	vehicle, exists := db.Vehicles[id]
	db.mu.RUnlock()

	if !exists {
		return c.Status(404).JSON(fiber.Map{
			"error": "Vehicle not found",
		})
	}

	return c.JSON(vehicle)
}

func getUserOrders(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Email is required",
		})
	}

	var userOrders []Order
	db.mu.RLock()
	for _, order := range db.Orders {
		if order.UserEmail == email {
			userOrders = append(userOrders, order)
		}
	}
	db.mu.RUnlock()

	return c.JSON(userOrders)
}

type NewOrderRequest struct {
	VehicleID        string            `json:"vehicle_id"`
	UserEmail        string            `json:"user_email"`
	DeliveryAddress  string            `json:"delivery_address"`
	PaymentMethod    string            `json:"payment_method"`
	FinancingDetails *FinancingDetails `json:"financing_details"`
	TradeIn          *TradeInDetails   `json:"trade_in"`
}

func createOrder(c *fiber.Ctx) error {
	var req NewOrderRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.RLock()
	vehicle, exists := db.Vehicles[req.VehicleID]
	db.mu.RUnlock()

	if !exists {
		return c.Status(404).JSON(fiber.Map{
			"error": "Vehicle not found",
		})
	}

	if !vehicle.Available {
		return c.Status(400).JSON(fiber.Map{
			"error": "Vehicle is no longer available",
		})
	}

	totalPrice := vehicle.Price
	if req.TradeIn != nil {
		totalPrice -= req.TradeIn.Estimate.FinalOffer
	}

	order := Order{
		ID:               uuid.New().String(),
		UserEmail:        req.UserEmail,
		Vehicle:          vehicle,
		Status:           "pending",
		DeliveryAddress:  req.DeliveryAddress,
		PaymentMethod:    req.PaymentMethod,
		FinancingDetails: req.FinancingDetails,
		TradeIn:          req.TradeIn,
		TotalPrice:       totalPrice,
		DeliveryDate:     time.Now().AddDate(0, 0, 7), // Default delivery in 7 days
		CreatedAt:        time.Now(),
	}

	db.mu.Lock()
	db.Orders[order.ID] = order
	vehicle.Available = false
	db.Vehicles[vehicle.ID] = vehicle
	db.mu.Unlock()

	return c.Status(201).JSON(order)
}

func getTradeInEstimate(c *fiber.Ctx) error {
	var req TradeInRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Simple trade-in value calculation (in reality, this would be much more complex)
	baseValue := float64(30000 - (2023-req.Year)*1500 - req.Mileage/100)
	conditionAdjustment := 0.0
	switch req.Condition {
	case "excellent":
		conditionAdjustment = 2000
	case "good":
		conditionAdjustment = 1000
	case "fair":
		conditionAdjustment = 0
	case "poor":
		conditionAdjustment = -1000
	}

	marketAdjustment := 500.0 // Market demand adjustment

	estimate := TradeInEstimate{
		EstimatedValue:       baseValue,
		ConditionAdjustments: conditionAdjustment,
		MarketAdjustments:    marketAdjustment,
		FinalOffer:           baseValue + conditionAdjustment + marketAdjustment,
	}

	return c.JSON(estimate)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Vehicles: make(map[string]Vehicle),
		Orders:   make(map[string]Order),
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

	// Vehicle routes
	api.Get("/vehicles", searchVehicles)
	api.Get("/vehicles/:vehicleId", getVehicle)

	// Order routes
	api.Get("/orders", getUserOrders)
	api.Post("/orders", createOrder)

	// Trade-in routes
	api.Post("/trade-in/estimate", getTradeInEstimate)
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

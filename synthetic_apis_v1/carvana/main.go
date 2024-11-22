package main

import (
	"encoding/json"
	"errors"
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
	ID         string   `json:"id"`
	Make       string   `json:"make"`
	Model      string   `json:"model"`
	Year       int      `json:"year"`
	Price      float64  `json:"price"`
	Mileage    int      `json:"mileage"`
	Color      string   `json:"color"`
	VIN        string   `json:"vin"`
	Features   []string `json:"features"`
	Images     []string `json:"images"`
	Condition  string   `json:"condition"`
	CarfaxLink string   `json:"carfax_link"`
	Available  bool     `json:"available"`
}

type FinancingDetails struct {
	TermMonths     int     `json:"term_months"`
	APR            float64 `json:"apr"`
	MonthlyPayment float64 `json:"monthly_payment"`
	DownPayment    float64 `json:"down_payment"`
}

type TradeInDetails struct {
	Make  string  `json:"make"`
	Model string  `json:"model"`
	Year  int     `json:"year"`
	Value float64 `json:"value"`
}

type OrderStatus string

const (
	OrderStatusPending    OrderStatus = "pending"
	OrderStatusApproved   OrderStatus = "approved"
	OrderStatusDelivering OrderStatus = "delivering"
	OrderStatusCompleted  OrderStatus = "completed"
	OrderStatusCancelled  OrderStatus = "cancelled"
)

type Order struct {
	ID               string            `json:"id"`
	VehicleID        string            `json:"vehicle_id"`
	UserEmail        string            `json:"user_email"`
	Status           OrderStatus       `json:"status"`
	DeliveryDate     time.Time         `json:"delivery_date"`
	PaymentMethod    string            `json:"payment_method"`
	FinancingDetails *FinancingDetails `json:"financing_details,omitempty"`
	TradeIn          *TradeInDetails   `json:"trade_in,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

type User struct {
	Email          string          `json:"email"`
	Name           string          `json:"name"`
	Phone          string          `json:"phone"`
	Address        string          `json:"address"`
	LicenseNumber  string          `json:"license_number"`
	LicenseState   string          `json:"license_state"`
	PaymentMethods []PaymentMethod `json:"payment_methods"`
}

type PaymentMethod struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Last4    string `json:"last4"`
	ExpiryMM int    `json:"expiry_mm"`
	ExpiryYY int    `json:"expiry_yy"`
}

// Database represents our in-memory database
type Database struct {
	Users    map[string]User    `json:"users"`
	Vehicles map[string]Vehicle `json:"vehicles"`
	Orders   map[string]Order   `json:"orders"`
	mu       sync.RWMutex
}

var db *Database

// Database operations
func (d *Database) GetVehicle(id string) (Vehicle, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	vehicle, exists := d.Vehicles[id]
	if !exists {
		return Vehicle{}, errors.New("vehicle not found")
	}
	return vehicle, nil
}

func (d *Database) GetUser(email string) (User, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	user, exists := d.Users[email]
	if !exists {
		return User{}, errors.New("user not found")
	}
	return user, nil
}

func (d *Database) CreateOrder(order Order) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Orders[order.ID] = order
	return nil
}

// Handlers
func searchVehicles(c *fiber.Ctx) error {
	make := c.Query("make")
	model := c.Query("model")
	yearMin := c.QueryInt("year_min", 0)
	yearMax := c.QueryInt("year_max", 0)
	priceMin := c.QueryFloat("price_min", 0)
	priceMax := c.QueryFloat("price_max", 0)

	var results []Vehicle

	db.mu.RLock()
	for _, vehicle := range db.Vehicles {
		if !vehicle.Available {
			continue
		}

		if make != "" && vehicle.Make != make {
			continue
		}
		if model != "" && vehicle.Model != model {
			continue
		}
		if yearMin != 0 && vehicle.Year < yearMin {
			continue
		}
		if yearMax != 0 && vehicle.Year > yearMax {
			continue
		}
		if priceMin != 0 && vehicle.Price < priceMin {
			continue
		}
		if priceMax != 0 && vehicle.Price > priceMax {
			continue
		}

		results = append(results, vehicle)
	}
	db.mu.RUnlock()

	return c.JSON(results)
}

func getVehicleDetails(c *fiber.Ctx) error {
	id := c.Params("vehicleId")

	vehicle, err := db.GetVehicle(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Vehicle not found",
		})
	}

	return c.JSON(vehicle)
}

func getUserOrders(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
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
	VehicleID          string `json:"vehicle_id"`
	UserEmail          string `json:"user_email"`
	PaymentMethod      string `json:"payment_method"`
	FinancingRequested bool   `json:"financing_requested"`
	TradeInID          string `json:"trade_in_id"`
}

func createOrder(c *fiber.Ctx) error {
	var req NewOrderRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user
	user, err := db.GetUser(req.UserEmail)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// Validate vehicle
	vehicle, err := db.GetVehicle(req.VehicleID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Vehicle not found",
		})
	}

	if !vehicle.Available {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Vehicle is no longer available",
		})
	}

	// Validate payment method
	validPayment := false
	for _, pm := range user.PaymentMethods {
		if pm.ID == req.PaymentMethod {
			validPayment = true
			break
		}
	}
	if !validPayment {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid payment method",
		})
	}

	// Create new order
	order := Order{
		ID:            uuid.New().String(),
		VehicleID:     req.VehicleID,
		UserEmail:     req.UserEmail,
		Status:        OrderStatusPending,
		DeliveryDate:  time.Now().AddDate(0, 0, 7), // Default delivery in 7 days
		PaymentMethod: req.PaymentMethod,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	// If financing is requested, calculate details
	if req.FinancingRequested {
		order.FinancingDetails = &FinancingDetails{
			TermMonths:     72,
			APR:            4.99,
			MonthlyPayment: calculateMonthlyPayment(vehicle.Price, 72, 4.99),
			DownPayment:    vehicle.Price * 0.1, // 10% down payment
		}
	}

	// Save order
	if err := db.CreateOrder(order); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create order",
		})
	}

	// Mark vehicle as unavailable
	db.mu.Lock()
	vehicle.Available = false
	db.Vehicles[vehicle.ID] = vehicle
	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(order)
}

type TradeInRequest struct {
	Make      string `json:"make"`
	Model     string `json:"model"`
	Year      int    `json:"year"`
	Mileage   int    `json:"mileage"`
	Condition string `json:"condition"`
	ZipCode   string `json:"zip_code"`
}

func getTradeInEstimate(c *fiber.Ctx) error {
	var req TradeInRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Simple trade-in value calculation
	baseValue := 5000.0
	yearFactor := float64(req.Year-2000) * 500
	mileageFactor := float64(req.Mileage) * -0.02

	value := baseValue + yearFactor + mileageFactor
	if req.Condition == "excellent" {
		value *= 1.2
	} else if req.Condition == "poor" {
		value *= 0.8
	}

	estimate := struct {
		ID         string    `json:"id"`
		Value      float64   `json:"value"`
		ValidUntil time.Time `json:"valid_until"`
	}{
		ID:         uuid.New().String(),
		Value:      value,
		ValidUntil: time.Now().AddDate(0, 0, 7),
	}

	return c.JSON(estimate)
}

func calculateMonthlyPayment(principal float64, termMonths int, apr float64) float64 {
	monthlyRate := apr / 12 / 100
	payment := principal * monthlyRate * float64(pow(1+monthlyRate, termMonths)) /
		float64(pow(1+monthlyRate, termMonths)-1)
	return payment
}

func pow(base float64, exp int) float64 {
	result := 1.0
	for i := 0; i < exp; i++ {
		result *= base
	}
	return result
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:    make(map[string]User),
		Vehicles: make(map[string]Vehicle),
		Orders:   make(map[string]Order),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Vehicle routes
	api.Get("/vehicles", searchVehicles)
	api.Get("/vehicles/:vehicleId", getVehicleDetails)

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

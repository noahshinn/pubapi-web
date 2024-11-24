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
)

// Domain Models
type Vehicle struct {
	ID           string    `json:"id"`
	VIN          string    `json:"vin"`
	Model        string    `json:"model"`
	Name         string    `json:"name"`
	Color        string    `json:"color"`
	PurchaseDate time.Time `json:"purchase_date"`
	UserEmail    string    `json:"user_email"`
}

type ClimateState struct {
	InsideTemp  float64 `json:"inside_temp"`
	IsClimateOn bool    `json:"is_climate_on"`
	SetTemp     float64 `json:"set_temp"`
}

type DriveState struct {
	Speed     float64 `json:"speed"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Heading   float64 `json:"heading"`
}

type SoftwareUpdate struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

type VehicleState struct {
	BatteryLevel   int            `json:"battery_level"`
	ChargingState  string         `json:"charging_state"`
	ClimateState   ClimateState   `json:"climate_state"`
	DriveState     DriveState     `json:"drive_state"`
	Locked         bool           `json:"locked"`
	SentryMode     bool           `json:"sentry_mode"`
	SoftwareUpdate SoftwareUpdate `json:"software_update"`
}

type ChargingScheduleDay struct {
	Day         int    `json:"day"`
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	ChargeLimit int    `json:"charge_limit"`
}

type ChargingSchedule struct {
	Enabled        bool                  `json:"enabled"`
	WeeklySchedule []ChargingScheduleDay `json:"weekly_schedule"`
}

type User struct {
	Email      string   `json:"email"`
	Name       string   `json:"name"`
	VehicleIds []string `json:"vehicle_ids"`
}

// Database represents our in-memory database
type Database struct {
	Users             map[string]User             `json:"users"`
	Vehicles          map[string]Vehicle          `json:"vehicles"`
	VehicleStates     map[string]VehicleState     `json:"vehicle_states"`
	ChargingSchedules map[string]ChargingSchedule `json:"charging_schedules"`
	mu                sync.RWMutex
}

// Custom errors
var (
	ErrUserNotFound    = errors.New("user not found")
	ErrVehicleNotFound = errors.New("vehicle not found")
	ErrUnauthorized    = errors.New("unauthorized")
	ErrInvalidCommand  = errors.New("invalid command")
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

func (d *Database) GetVehicle(id string) (Vehicle, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	vehicle, exists := d.Vehicles[id]
	if !exists {
		return Vehicle{}, ErrVehicleNotFound
	}
	return vehicle, nil
}

func (d *Database) GetVehicleState(id string) (VehicleState, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	state, exists := d.VehicleStates[id]
	if !exists {
		return VehicleState{}, ErrVehicleNotFound
	}
	return state, nil
}

func (d *Database) UpdateVehicleState(id string, state VehicleState) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.Vehicles[id]; !exists {
		return ErrVehicleNotFound
	}

	d.VehicleStates[id] = state
	return nil
}

// HTTP Handlers
func getUserVehicles(c *fiber.Ctx) error {
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

	var vehicles []Vehicle
	for _, vehicleId := range user.VehicleIds {
		if vehicle, err := db.GetVehicle(vehicleId); err == nil {
			vehicles = append(vehicles, vehicle)
		}
	}

	return c.JSON(vehicles)
}

func getVehicleState(c *fiber.Ctx) error {
	vehicleId := c.Params("vehicleId")
	if vehicleId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "vehicle ID is required",
		})
	}

	state, err := db.GetVehicleState(vehicleId)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(state)
}

type CommandRequest struct {
	Command    string                 `json:"command"`
	Parameters map[string]interface{} `json:"parameters"`
}

func sendCommand(c *fiber.Ctx) error {
	vehicleId := c.Params("vehicleId")
	if vehicleId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "vehicle ID is required",
		})
	}

	var cmd CommandRequest
	if err := c.BodyParser(&cmd); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Get current vehicle state
	state, err := db.GetVehicleState(vehicleId)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Process command
	result := processCommand(&state, cmd)

	// Update vehicle state
	if err := db.UpdateVehicleState(vehicleId, state); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update vehicle state",
		})
	}

	return c.JSON(result)
}

func processCommand(state *VehicleState, cmd CommandRequest) map[string]interface{} {
	result := map[string]interface{}{
		"result": true,
		"reason": "Command executed successfully",
	}

	switch cmd.Command {
	case "door_lock":
		state.Locked = true
	case "door_unlock":
		state.Locked = false
	case "climate_on":
		state.ClimateState.IsClimateOn = true
		if temp, ok := cmd.Parameters["temperature"].(float64); ok {
			state.ClimateState.SetTemp = temp
		}
	case "climate_off":
		state.ClimateState.IsClimateOn = false
	case "charge_start":
		state.ChargingState = "Charging"
	case "charge_stop":
		state.ChargingState = "Stopped"
	default:
		result["result"] = false
		result["reason"] = "Unknown command"
	}

	return result
}

func getChargingSchedule(c *fiber.Ctx) error {
	vehicleId := c.Params("vehicleId")
	if vehicleId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "vehicle ID is required",
		})
	}

	db.mu.RLock()
	schedule, exists := db.ChargingSchedules[vehicleId]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Charging schedule not found",
		})
	}

	return c.JSON(schedule)
}

func updateChargingSchedule(c *fiber.Ctx) error {
	vehicleId := c.Params("vehicleId")
	if vehicleId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "vehicle ID is required",
		})
	}

	var schedule ChargingSchedule
	if err := c.BodyParser(&schedule); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate schedule
	for _, day := range schedule.WeeklySchedule {
		if day.Day < 0 || day.Day > 6 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid day in schedule",
			})
		}
		if day.ChargeLimit < 0 || day.ChargeLimit > 100 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid charge limit",
			})
		}
	}

	db.mu.Lock()
	db.ChargingSchedules[vehicleId] = schedule
	db.mu.Unlock()

	return c.JSON(schedule)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:             make(map[string]User),
		Vehicles:          make(map[string]Vehicle),
		VehicleStates:     make(map[string]VehicleState),
		ChargingSchedules: make(map[string]ChargingSchedule),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Vehicle routes
	api.Get("/vehicles", getUserVehicles)
	api.Get("/vehicles/:vehicleId/state", getVehicleState)
	api.Post("/vehicles/:vehicleId/command", sendCommand)

	// Charging schedule routes
	api.Get("/vehicles/:vehicleId/charging/schedule", getChargingSchedule)
	api.Put("/vehicles/:vehicleId/charging/schedule", updateChargingSchedule)
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

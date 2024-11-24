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
type Vehicle struct {
	ID           string    `json:"id"`
	VIN          string    `json:"vin"`
	Model        string    `json:"model"`
	Name         string    `json:"name"`
	Color        string    `json:"color"`
	PurchaseDate time.Time `json:"purchase_date"`
}

type ClimateState struct {
	InsideTemp  float64 `json:"inside_temp"`
	OutsideTemp float64 `json:"outside_temp"`
	IsClimateOn bool    `json:"is_climate_on"`
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
	Day         string `json:"day"`
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	ChargeLimit int    `json:"charge_limit"`
}

type ChargingSchedule struct {
	Enabled        bool                  `json:"enabled"`
	WeeklySchedule []ChargingScheduleDay `json:"weekly_schedule"`
}

type Command struct {
	Command    string                 `json:"command"`
	Parameters map[string]interface{} `json:"parameters"`
}

type CommandResult struct {
	Result bool   `json:"result"`
	Reason string `json:"reason"`
}

// Database structure
type Database struct {
	Users             map[string]User             `json:"users"`
	Vehicles          map[string]Vehicle          `json:"vehicles"`
	VehicleStates     map[string]VehicleState     `json:"vehicle_states"`
	ChargingSchedules map[string]ChargingSchedule `json:"charging_schedules"`
	mu                sync.RWMutex
}

type User struct {
	Email      string   `json:"email"`
	Name       string   `json:"name"`
	VehicleIds []string `json:"vehicle_ids"`
}

var db *Database

// Database operations
func (d *Database) GetUserVehicles(email string) ([]Vehicle, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	user, exists := d.Users[email]
	if !exists {
		return nil, fiber.NewError(fiber.StatusNotFound, "User not found")
	}

	var vehicles []Vehicle
	for _, vehicleId := range user.VehicleIds {
		if vehicle, exists := d.Vehicles[vehicleId]; exists {
			vehicles = append(vehicles, vehicle)
		}
	}

	return vehicles, nil
}

func (d *Database) GetVehicleState(vehicleId string) (VehicleState, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	state, exists := d.VehicleStates[vehicleId]
	if !exists {
		return VehicleState{}, fiber.NewError(fiber.StatusNotFound, "Vehicle state not found")
	}

	return state, nil
}

func (d *Database) UpdateVehicleState(vehicleId string, updates map[string]interface{}) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	state, exists := d.VehicleStates[vehicleId]
	if !exists {
		return fiber.NewError(fiber.StatusNotFound, "Vehicle state not found")
	}

	// Update relevant fields based on the command
	// This is a simplified version - in reality, you'd want more sophisticated state management
	if val, ok := updates["battery_level"].(int); ok {
		state.BatteryLevel = val
	}
	if val, ok := updates["charging_state"].(string); ok {
		state.ChargingState = val
	}
	if val, ok := updates["locked"].(bool); ok {
		state.Locked = val
	}

	d.VehicleStates[vehicleId] = state
	return nil
}

func (d *Database) GetChargingSchedule(vehicleId string) (ChargingSchedule, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	schedule, exists := d.ChargingSchedules[vehicleId]
	if !exists {
		return ChargingSchedule{}, fiber.NewError(fiber.StatusNotFound, "Charging schedule not found")
	}

	return schedule, nil
}

func (d *Database) UpdateChargingSchedule(vehicleId string, schedule ChargingSchedule) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.Vehicles[vehicleId]; !exists {
		return fiber.NewError(fiber.StatusNotFound, "Vehicle not found")
	}

	d.ChargingSchedules[vehicleId] = schedule
	return nil
}

// HTTP Handlers
func getVehicles(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Email parameter is required")
	}

	vehicles, err := db.GetUserVehicles(email)
	if err != nil {
		return err
	}

	return c.JSON(vehicles)
}

func getVehicleState(c *fiber.Ctx) error {
	vehicleId := c.Params("vehicleId")
	if vehicleId == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Vehicle ID is required")
	}

	state, err := db.GetVehicleState(vehicleId)
	if err != nil {
		return err
	}

	return c.JSON(state)
}

func sendCommand(c *fiber.Ctx) error {
	vehicleId := c.Params("vehicleId")
	if vehicleId == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Vehicle ID is required")
	}

	var cmd Command
	if err := c.BodyParser(&cmd); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid command format")
	}

	// Process command and update vehicle state
	updates := make(map[string]interface{})
	result := CommandResult{Result: true}

	switch cmd.Command {
	case "wake_up":
		// Simulate wake up process
		time.Sleep(2 * time.Second)
	case "door_lock":
		updates["locked"] = true
	case "door_unlock":
		updates["locked"] = false
	case "climate_on":
		updates["climate_state"] = ClimateState{IsClimateOn: true}
	case "climate_off":
		updates["climate_state"] = ClimateState{IsClimateOn: false}
	case "charge_start":
		updates["charging_state"] = "Charging"
	case "charge_stop":
		updates["charging_state"] = "Stopped"
	default:
		return fiber.NewError(fiber.StatusBadRequest, "Unknown command")
	}

	if err := db.UpdateVehicleState(vehicleId, updates); err != nil {
		result.Result = false
		result.Reason = err.Error()
	}

	return c.JSON(result)
}

func getChargingSchedule(c *fiber.Ctx) error {
	vehicleId := c.Params("vehicleId")
	if vehicleId == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Vehicle ID is required")
	}

	schedule, err := db.GetChargingSchedule(vehicleId)
	if err != nil {
		return err
	}

	return c.JSON(schedule)
}

func updateChargingSchedule(c *fiber.Ctx) error {
	vehicleId := c.Params("vehicleId")
	if vehicleId == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Vehicle ID is required")
	}

	var schedule ChargingSchedule
	if err := c.BodyParser(&schedule); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid charging schedule format")
	}

	if err := db.UpdateChargingSchedule(vehicleId, schedule); err != nil {
		return err
	}

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
	api.Get("/vehicles", getVehicles)
	api.Get("/vehicles/:vehicleId/state", getVehicleState)
	api.Post("/vehicles/:vehicleId/command", sendCommand)
	api.Get("/vehicles/:vehicleId/charging/schedule", getChargingSchedule)
	api.Put("/vehicles/:vehicleId/charging/schedule", updateChargingSchedule)
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

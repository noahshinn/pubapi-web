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

// Domain Models
type DeviceType string

const (
	DeviceTypeThermostat DeviceType = "thermostat"
	DeviceTypeCamera     DeviceType = "camera"
	DeviceTypeSensor     DeviceType = "sensor"
)

type ThermostatMode string

const (
	ThermostatModeHeat ThermostatMode = "heat"
	ThermostatModeCool ThermostatMode = "cool"
	ThermostatModeEco  ThermostatMode = "eco"
	ThermostatModeOff  ThermostatMode = "off"
)

type Device struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Type       DeviceType `json:"type"`
	Room       string     `json:"room"`
	HomeID     string     `json:"home_id"`
	Status     string     `json:"status"`
	Battery    int        `json:"battery"`
	LastUpdate time.Time  `json:"last_update"`
}

type Thermostat struct {
	Device
	CurrentTemp float64        `json:"current_temp"`
	TargetTemp  float64        `json:"target_temp"`
	Humidity    int            `json:"humidity"`
	Mode        ThermostatMode `json:"mode"`
	FanStatus   string         `json:"fan_status"`
}

type Home struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Address string   `json:"address"`
	UserID  string   `json:"user_id"`
	Devices []Device `json:"devices"`
}

type EnergyUsage struct {
	Timestamp time.Time `json:"timestamp"`
	Usage     float64   `json:"usage"`
}

type EnergyReport struct {
	Period         string        `json:"period"`
	TotalUsage     float64       `json:"total_usage"`
	AverageUsage   float64       `json:"average_usage"`
	SavingsPercent float64       `json:"savings_percent"`
	Details        []EnergyUsage `json:"details"`
}

// Database represents our in-memory database
type Database struct {
	Homes       map[string]Home       `json:"homes"`
	Devices     map[string]Device     `json:"devices"`
	Thermostats map[string]Thermostat `json:"thermostats"`
	mu          sync.RWMutex
}

var db *Database

// Database operations
func (d *Database) GetHome(id string) (Home, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	home, exists := d.Homes[id]
	if !exists {
		return Home{}, fiber.NewError(fiber.StatusNotFound, "Home not found")
	}
	return home, nil
}

func (d *Database) GetDevice(id string) (Device, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	device, exists := d.Devices[id]
	if !exists {
		return Device{}, fiber.NewError(fiber.StatusNotFound, "Device not found")
	}
	return device, nil
}

func (d *Database) GetThermostat(id string) (Thermostat, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	thermostat, exists := d.Thermostats[id]
	if !exists {
		return Thermostat{}, fiber.NewError(fiber.StatusNotFound, "Thermostat not found")
	}
	return thermostat, nil
}

func (d *Database) UpdateThermostat(id string, temp float64, mode ThermostatMode) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	thermostat, exists := d.Thermostats[id]
	if !exists {
		return fiber.NewError(fiber.StatusNotFound, "Thermostat not found")
	}

	thermostat.TargetTemp = temp
	thermostat.Mode = mode
	thermostat.LastUpdate = time.Now()
	d.Thermostats[id] = thermostat

	return nil
}

// Handlers
func getUserDevices(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Email is required")
	}

	var userDevices []Device
	db.mu.RLock()
	for _, home := range db.Homes {
		if home.UserID == email {
			userDevices = append(userDevices, home.Devices...)
		}
	}
	db.mu.RUnlock()

	return c.JSON(userDevices)
}

func getDeviceStatus(c *fiber.Ctx) error {
	deviceID := c.Params("deviceId")

	device, err := db.GetDevice(deviceID)
	if err != nil {
		return err
	}

	if device.Type == DeviceTypeThermostat {
		thermostat, err := db.GetThermostat(deviceID)
		if err != nil {
			return err
		}
		return c.JSON(fiber.Map{
			"online":      true,
			"currentTemp": thermostat.CurrentTemp,
			"targetTemp":  thermostat.TargetTemp,
			"humidity":    thermostat.Humidity,
			"mode":        thermostat.Mode,
			"fanStatus":   thermostat.FanStatus,
		})
	}

	return c.JSON(fiber.Map{
		"online":     true,
		"status":     device.Status,
		"battery":    device.Battery,
		"lastUpdate": device.LastUpdate,
	})
}

type TemperatureUpdate struct {
	Temperature float64        `json:"temperature"`
	Mode        ThermostatMode `json:"mode"`
}

func updateTemperature(c *fiber.Ctx) error {
	deviceID := c.Params("deviceId")

	var update TemperatureUpdate
	if err := c.BodyParser(&update); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if update.Temperature < 50 || update.Temperature > 90 {
		return fiber.NewError(fiber.StatusBadRequest, "Temperature must be between 50°F and 90°F")
	}

	if err := db.UpdateThermostat(deviceID, update.Temperature, update.Mode); err != nil {
		return err
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Temperature updated successfully",
	})
}

func getEnergyReport(c *fiber.Ctx) error {
	homeID := c.Params("homeId")
	period := c.Query("period", "day")

	_, err := db.GetHome(homeID)
	if err != nil {
		return err
	}

	// Generate sample energy report data
	now := time.Now()
	var details []EnergyUsage

	switch period {
	case "day":
		for i := 0; i < 24; i++ {
			details = append(details, EnergyUsage{
				Timestamp: now.Add(time.Duration(-i) * time.Hour),
				Usage:     float64(20 + i%5),
			})
		}
	case "week":
		for i := 0; i < 7; i++ {
			details = append(details, EnergyUsage{
				Timestamp: now.Add(time.Duration(-i*24) * time.Hour),
				Usage:     float64(140 + i%20),
			})
		}
	case "month":
		for i := 0; i < 30; i++ {
			details = append(details, EnergyUsage{
				Timestamp: now.Add(time.Duration(-i*24) * time.Hour),
				Usage:     float64(600 + i%50),
			})
		}
	}

	// Calculate totals and averages
	var totalUsage float64
	for _, detail := range details {
		totalUsage += detail.Usage
	}

	report := EnergyReport{
		Period:         period,
		TotalUsage:     totalUsage,
		AverageUsage:   totalUsage / float64(len(details)),
		SavingsPercent: 15.5, // Example savings percentage
		Details:        details,
	}

	return c.JSON(report)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Homes:       make(map[string]Home),
		Devices:     make(map[string]Device),
		Thermostats: make(map[string]Thermostat),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	api.Get("/devices", getUserDevices)
	api.Get("/devices/:deviceId/status", getDeviceStatus)
	api.Put("/devices/:deviceId/temperature", updateTemperature)
	api.Get("/homes/:homeId/energy-report", getEnergyReport)
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

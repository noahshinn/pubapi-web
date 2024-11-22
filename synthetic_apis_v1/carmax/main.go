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
type Car struct {
	ID       string    `json:"id"`
	Make     string    `json:"make"`
	Model    string    `json:"model"`
	Year     int       `json:"year"`
	Price    float64   `json:"price"`
	Mileage  int       `json:"mileage"`
	Color    string    `json:"color"`
	VIN      string    `json:"vin"`
	Features []string  `json:"features"`
	Images   []string  `json:"images"`
	AddedAt  time.Time `json:"added_at"`
}

type SavedCar struct {
	Car     Car       `json:"car"`
	SavedAt time.Time `json:"saved_at"`
	Notes   string    `json:"notes"`
}

type User struct {
	Email     string     `json:"email"`
	Name      string     `json:"name"`
	Phone     string     `json:"phone"`
	SavedCars []SavedCar `json:"saved_cars"`
}

type AppointmentStatus string

const (
	AppointmentStatusPending   AppointmentStatus = "pending"
	AppointmentStatusConfirmed AppointmentStatus = "confirmed"
	AppointmentStatusCanceled  AppointmentStatus = "canceled"
	AppointmentStatusCompleted AppointmentStatus = "completed"
)

type Appointment struct {
	ID        string            `json:"id"`
	Car       Car               `json:"car"`
	UserEmail string            `json:"user_email"`
	DateTime  time.Time         `json:"datetime"`
	Location  string            `json:"location"`
	Status    AppointmentStatus `json:"status"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// Database represents our in-memory database
type Database struct {
	Users        map[string]User        `json:"users"`
	Cars         map[string]Car         `json:"cars"`
	Appointments map[string]Appointment `json:"appointments"`
	mu           sync.RWMutex
}

// Global database instance
var db *Database

// Database operations
func (d *Database) GetUser(email string) (User, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	user, exists := d.Users[email]
	if !exists {
		return User{}, errors.New("user not found")
	}
	return user, nil
}

func (d *Database) GetCar(id string) (Car, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	car, exists := d.Cars[id]
	if !exists {
		return Car{}, errors.New("car not found")
	}
	return car, nil
}

func (d *Database) SaveCar(email string, savedCar SavedCar) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	user, exists := d.Users[email]
	if !exists {
		return errors.New("user not found")
	}

	user.SavedCars = append(user.SavedCars, savedCar)
	d.Users[email] = user
	return nil
}

func (d *Database) CreateAppointment(appointment Appointment) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Appointments[appointment.ID] = appointment
	return nil
}

// HTTP Handlers
func searchCars(c *fiber.Ctx) error {
	make := c.Query("make")
	model := c.Query("model")
	maxPrice := c.QueryFloat("maxPrice", 1000000)
	maxMileage := c.QueryInt("maxMileage", 1000000)

	var matchingCars []Car

	db.mu.RLock()
	for _, car := range db.Cars {
		if (make == "" || car.Make == make) &&
			(model == "" || car.Model == model) &&
			car.Price <= maxPrice &&
			car.Mileage <= maxMileage {
			matchingCars = append(matchingCars, car)
		}
	}
	db.mu.RUnlock()

	return c.JSON(matchingCars)
}

func getSavedCars(c *fiber.Ctx) error {
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

	return c.JSON(user.SavedCars)
}

func saveCar(c *fiber.Ctx) error {
	var req struct {
		CarID     string `json:"carId"`
		UserEmail string `json:"userEmail"`
		Notes     string `json:"notes"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	car, err := db.GetCar(req.CarID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	savedCar := SavedCar{
		Car:     car,
		SavedAt: time.Now(),
		Notes:   req.Notes,
	}

	if err := db.SaveCar(req.UserEmail, savedCar); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(savedCar)
}

func getAppointments(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	var userAppointments []Appointment

	db.mu.RLock()
	for _, appointment := range db.Appointments {
		if appointment.UserEmail == email {
			userAppointments = append(userAppointments, appointment)
		}
	}
	db.mu.RUnlock()

	return c.JSON(userAppointments)
}

func createAppointment(c *fiber.Ctx) error {
	var req struct {
		CarID     string    `json:"carId"`
		UserEmail string    `json:"userEmail"`
		DateTime  time.Time `json:"datetime"`
		Location  string    `json:"location"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	car, err := db.GetCar(req.CarID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	appointment := Appointment{
		ID:        uuid.New().String(),
		Car:       car,
		UserEmail: req.UserEmail,
		DateTime:  req.DateTime,
		Location:  req.Location,
		Status:    AppointmentStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.CreateAppointment(appointment); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(appointment)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:        make(map[string]User),
		Cars:         make(map[string]Car),
		Appointments: make(map[string]Appointment),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Car inventory routes
	api.Get("/inventory", searchCars)
	api.Get("/inventory/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		car, err := db.GetCar(id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.JSON(car)
	})

	// Saved cars routes
	api.Get("/saved-cars", getSavedCars)
	api.Post("/saved-cars", saveCar)

	// Appointment routes
	api.Get("/appointments", getAppointments)
	api.Post("/appointments", createAppointment)
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

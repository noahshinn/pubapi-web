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
type Venue struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Address  string `json:"address"`
	City     string `json:"city"`
	State    string `json:"state"`
	Capacity int    `json:"capacity"`
}

type PriceRange struct {
	Category  string  `json:"category"`
	Price     float64 `json:"price"`
	Available int     `json:"available"`
}

type Event struct {
	ID               string       `json:"id"`
	Name             string       `json:"name"`
	Category         string       `json:"category"`
	Venue            Venue        `json:"venue"`
	DateTime         time.Time    `json:"datetime"`
	Description      string       `json:"description"`
	ImageURL         string       `json:"image_url"`
	PriceRanges      []PriceRange `json:"price_ranges"`
	AvailableTickets int          `json:"available_tickets"`
}

type Ticket struct {
	ID      string  `json:"id"`
	Section string  `json:"section"`
	Row     string  `json:"row"`
	Seat    string  `json:"seat"`
	Price   float64 `json:"price"`
	Barcode string  `json:"barcode"`
}

type Order struct {
	ID        string    `json:"id"`
	UserEmail string    `json:"user_email"`
	Event     Event     `json:"event"`
	Tickets   []Ticket  `json:"tickets"`
	Total     float64   `json:"total"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type User struct {
	Email          string          `json:"email"`
	Name           string          `json:"name"`
	Phone          string          `json:"phone"`
	PaymentMethods []PaymentMethod `json:"payment_methods"`
}

type PaymentMethod struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Last4 string `json:"last4"`
}

// Database represents our in-memory database
type Database struct {
	Users  map[string]User  `json:"users"`
	Events map[string]Event `json:"events"`
	Orders map[string]Order `json:"orders"`
	mu     sync.RWMutex
}

// Global database instance
var db *Database

// Custom errors
var (
	ErrUserNotFound  = errors.New("user not found")
	ErrEventNotFound = errors.New("event not found")
	ErrOrderNotFound = errors.New("order not found")
	ErrSoldOut       = errors.New("event sold out")
	ErrInvalidInput  = errors.New("invalid input")
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

func (d *Database) GetEvent(id string) (Event, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	event, exists := d.Events[id]
	if !exists {
		return Event{}, ErrEventNotFound
	}
	return event, nil
}

func (d *Database) CreateOrder(order Order) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Update available tickets
	event := d.Events[order.Event.ID]
	event.AvailableTickets -= len(order.Tickets)
	d.Events[order.Event.ID] = event

	d.Orders[order.ID] = order
	return nil
}

// HTTP Handlers
func searchEvents(c *fiber.Ctx) error {
	city := c.Query("city")
	category := c.Query("category")
	dateFrom := c.Query("date_from")
	dateTo := c.Query("date_to")

	var filteredEvents []Event
	db.mu.RLock()
	for _, event := range db.Events {
		if (city == "" || event.Venue.City == city) &&
			(category == "" || event.Category == category) {

			// Parse and check dates if provided
			if dateFrom != "" && dateTo != "" {
				from, err := time.Parse("2006-01-02", dateFrom)
				if err != nil {
					continue
				}
				to, err := time.Parse("2006-01-02", dateTo)
				if err != nil {
					continue
				}

				if event.DateTime.Before(from) || event.DateTime.After(to) {
					continue
				}
			}

			filteredEvents = append(filteredEvents, event)
		}
	}
	db.mu.RUnlock()

	return c.JSON(filteredEvents)
}

func getEventDetails(c *fiber.Ctx) error {
	eventID := c.Params("eventId")
	event, err := db.GetEvent(eventID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(event)
}

func getUserOrders(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
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

type PurchaseRequest struct {
	EventID         string `json:"event_id"`
	UserEmail       string `json:"user_email"`
	TicketCategory  string `json:"ticket_category"`
	Quantity        int    `json:"quantity"`
	PaymentMethodID string `json:"payment_method_id"`
}

func purchaseTickets(c *fiber.Ctx) error {
	var req PurchaseRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user
	user, err := db.GetUser(req.UserEmail)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Validate payment method
	validPayment := false
	for _, pm := range user.PaymentMethods {
		if pm.ID == req.PaymentMethodID {
			validPayment = true
			break
		}
	}
	if !validPayment {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid payment method",
		})
	}

	// Get event
	event, err := db.GetEvent(req.EventID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Check ticket availability
	if event.AvailableTickets < req.Quantity {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Not enough tickets available",
		})
	}

	// Find ticket price
	var ticketPrice float64
	for _, pr := range event.PriceRanges {
		if pr.Category == req.TicketCategory {
			ticketPrice = pr.Price
			break
		}
	}

	// Generate tickets
	var tickets []Ticket
	for i := 0; i < req.Quantity; i++ {
		tickets = append(tickets, Ticket{
			ID:      uuid.New().String(),
			Section: req.TicketCategory,
			Row:     "TBD",
			Seat:    "TBD",
			Price:   ticketPrice,
			Barcode: uuid.New().String(),
		})
	}

	// Calculate total
	total := ticketPrice * float64(req.Quantity)

	// Create order
	order := Order{
		ID:        uuid.New().String(),
		UserEmail: req.UserEmail,
		Event:     event,
		Tickets:   tickets,
		Total:     total,
		Status:    "confirmed",
		CreatedAt: time.Now(),
	}

	if err := db.CreateOrder(order); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create order",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(order)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err

	}

	db = &Database{
		Users:  make(map[string]User),
		Events: make(map[string]Event),
		Orders: make(map[string]Order),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Event routes
	api.Get("/events", searchEvents)
	api.Get("/events/:eventId", getEventDetails)

	// Order routes
	api.Get("/orders", getUserOrders)
	api.Post("/orders", purchaseTickets)
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

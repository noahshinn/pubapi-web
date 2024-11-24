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
type Venue struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Address  string `json:"address"`
	City     string `json:"city"`
	State    string `json:"state"`
	Capacity int    `json:"capacity"`
}

type Event struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Category         string    `json:"category"`
	Venue            Venue     `json:"venue"`
	Date             time.Time `json:"date"`
	MinPrice         float64   `json:"min_price"`
	MaxPrice         float64   `json:"max_price"`
	AvailableTickets int       `json:"available_tickets"`
}

type Ticket struct {
	ID             string  `json:"id"`
	EventID        string  `json:"event_id"`
	Section        string  `json:"section"`
	Row            string  `json:"row"`
	Seat           string  `json:"seat"`
	Price          float64 `json:"price"`
	ServiceFee     float64 `json:"service_fee"`
	DeliveryMethod string  `json:"delivery_method"`
	Status         string  `json:"status"` // available, sold, reserved
}

type User struct {
	Email          string          `json:"email"`
	Name           string          `json:"name"`
	Phone          string          `json:"phone"`
	PaymentMethods []PaymentMethod `json:"payment_methods"`
}

type PaymentMethod struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Last4    string `json:"last4"`
	ExpiryMM int    `json:"expiry_mm"`
	ExpiryYY int    `json:"expiry_yy"`
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

// Database represents our in-memory database
type Database struct {
	Users   map[string]User   `json:"users"`
	Events  map[string]Event  `json:"events"`
	Tickets map[string]Ticket `json:"tickets"`
	Orders  map[string]Order  `json:"orders"`
	mu      sync.RWMutex
}

var (
	ErrUserNotFound   = errors.New("user not found")
	ErrEventNotFound  = errors.New("event not found")
	ErrTicketNotFound = errors.New("ticket not found")
	ErrTicketSoldOut  = errors.New("ticket sold out")
	ErrInvalidPayment = errors.New("invalid payment method")
)

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

func (d *Database) GetEvent(id string) (Event, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	event, exists := d.Events[id]
	if !exists {
		return Event{}, ErrEventNotFound
	}
	return event, nil
}

func (d *Database) GetTicket(id string) (Ticket, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	ticket, exists := d.Tickets[id]
	if !exists {
		return Ticket{}, ErrTicketNotFound
	}
	return ticket, nil
}

func (d *Database) CreateOrder(order Order) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Orders[order.ID] = order

	// Update ticket status
	for _, ticket := range order.Tickets {
		t := d.Tickets[ticket.ID]
		t.Status = "sold"
		d.Tickets[ticket.ID] = t
	}

	// Update event available tickets
	event := d.Events[order.Event.ID]
	event.AvailableTickets -= len(order.Tickets)
	d.Events[order.Event.ID] = event

	return nil
}

// HTTP Handlers
func searchEvents(c *fiber.Ctx) error {
	query := c.Query("query")
	category := c.Query("category")
	city := c.Query("city")
	dateFrom := c.Query("date_from")
	dateTo := c.Query("date_to")

	var filteredEvents []Event

	db.mu.RLock()
	for _, event := range db.Events {
		// Apply filters
		if query != "" && !contains(event.Name, query) {
			continue
		}
		if category != "" && event.Category != category {
			continue
		}
		if city != "" && event.Venue.City != city {
			continue
		}
		if dateFrom != "" {
			fromDate, err := time.Parse("2006-01-02", dateFrom)
			if err == nil && event.Date.Before(fromDate) {
				continue
			}
		}
		if dateTo != "" {
			toDate, err := time.Parse("2006-01-02", dateTo)
			if err == nil && event.Date.After(toDate) {
				continue
			}
		}

		filteredEvents = append(filteredEvents, event)
	}
	db.mu.RUnlock()

	return c.JSON(filteredEvents)
}

func getEventDetails(c *fiber.Ctx) error {
	eventId := c.Params("eventId")

	event, err := db.GetEvent(eventId)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(event)
}

func getEventTickets(c *fiber.Ctx) error {
	eventId := c.Params("eventId")

	var availableTickets []Ticket

	db.mu.RLock()
	for _, ticket := range db.Tickets {
		if ticket.EventID == eventId && ticket.Status == "available" {
			availableTickets = append(availableTickets, ticket)
		}
	}
	db.mu.RUnlock()

	return c.JSON(availableTickets)
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
	EventID       string   `json:"event_id"`
	UserEmail     string   `json:"user_email"`
	TicketIDs     []string `json:"ticket_ids"`
	PaymentMethod string   `json:"payment_method_id"`
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

	// Validate event and tickets
	event, err := db.GetEvent(req.EventID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	var tickets []Ticket
	var total float64

	db.mu.RLock()
	for _, ticketID := range req.TicketIDs {
		ticket, exists := db.Tickets[ticketID]
		if !exists || ticket.Status != "available" {
			db.mu.RUnlock()
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "One or more tickets are not available",
			})
		}
		tickets = append(tickets, ticket)
		total += ticket.Price + ticket.ServiceFee
	}
	db.mu.RUnlock()

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

func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:   make(map[string]User),
		Events:  make(map[string]Event),
		Tickets: make(map[string]Ticket),
		Orders:  make(map[string]Order),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Event routes
	api.Get("/events", searchEvents)
	api.Get("/events/:eventId", getEventDetails)
	api.Get("/events/:eventId/tickets", getEventTickets)

	// Order routes
	api.Get("/orders", getUserOrders)
	api.Post("/orders", purchaseTickets)
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

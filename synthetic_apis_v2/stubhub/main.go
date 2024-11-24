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

// Models
type Venue struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Address  string `json:"address"`
	City     string `json:"city"`
	State    string `json:"state"`
	Country  string `json:"country"`
	Capacity int    `json:"capacity"`
}

type Event struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Category    string    `json:"category"`
	Venue       Venue     `json:"venue"`
	Date        time.Time `json:"date"`
	Description string    `json:"description"`
	MinPrice    float64   `json:"min_price"`
	MaxPrice    float64   `json:"max_price"`
	ImageURL    string    `json:"image_url"`
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
	SellerRating   float64 `json:"seller_rating"`
	Available      bool    `json:"available"`
}

type Order struct {
	ID             string    `json:"id"`
	UserEmail      string    `json:"user_email"`
	Event          Event     `json:"event"`
	Tickets        []Ticket  `json:"tickets"`
	TotalPrice     float64   `json:"total_price"`
	ServiceFees    float64   `json:"service_fees"`
	DeliveryMethod string    `json:"delivery_method"`
	Status         string    `json:"status"`
	PurchaseDate   time.Time `json:"purchase_date"`
	PaymentMethod  string    `json:"payment_method"`
}

type Database struct {
	Events  map[string]Event  `json:"events"`
	Tickets map[string]Ticket `json:"tickets"`
	Orders  map[string]Order  `json:"orders"`
	Users   map[string]User   `json:"users"`
	mu      sync.RWMutex
}

type User struct {
	Email          string          `json:"email"`
	Name           string          `json:"name"`
	PaymentMethods []PaymentMethod `json:"payment_methods"`
}

type PaymentMethod struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Last4 string `json:"last4"`
}

var db *Database

// Handlers
func searchEvents(c *fiber.Ctx) error {
	query := c.Query("query")
	category := c.Query("category")
	dateFrom := c.Query("date_from")
	dateTo := c.Query("date_to")

	var filteredEvents []Event
	db.mu.RLock()
	for _, event := range db.Events {
		// Apply filters
		if query != "" && !contains(event.Title, query) {
			continue
		}
		if category != "" && event.Category != category {
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
	eventID := c.Params("eventId")

	db.mu.RLock()
	event, exists := db.Events[eventID]
	db.mu.RUnlock()

	if !exists {
		return c.Status(404).JSON(fiber.Map{
			"error": "Event not found",
		})
	}

	return c.JSON(event)
}

func getEventTickets(c *fiber.Ctx) error {
	eventID := c.Params("eventId")

	var availableTickets []Ticket
	db.mu.RLock()
	for _, ticket := range db.Tickets {
		if ticket.EventID == eventID && ticket.Available {
			availableTickets = append(availableTickets, ticket)
		}
	}
	db.mu.RUnlock()

	return c.JSON(availableTickets)
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

type PurchaseRequest struct {
	EventID        string   `json:"event_id"`
	TicketIDs      []string `json:"ticket_ids"`
	UserEmail      string   `json:"user_email"`
	PaymentMethod  string   `json:"payment_method"`
	DeliveryMethod string   `json:"delivery_method"`
}

func purchaseTickets(c *fiber.Ctx) error {
	var req PurchaseRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user
	db.mu.RLock()
	user, exists := db.Users[req.UserEmail]
	db.mu.RUnlock()
	if !exists {
		return c.Status(404).JSON(fiber.Map{
			"error": "User not found",
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
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid payment method",
		})
	}

	// Get event
	db.mu.RLock()
	event, exists := db.Events[req.EventID]
	db.mu.RUnlock()
	if !exists {
		return c.Status(404).JSON(fiber.Map{
			"error": "Event not found",
		})
	}

	// Validate and collect tickets
	var tickets []Ticket
	var totalPrice float64
	var serviceFees float64

	db.mu.Lock()
	defer db.mu.Unlock()

	for _, ticketID := range req.TicketIDs {
		ticket, exists := db.Tickets[ticketID]
		if !exists || !ticket.Available {
			return c.Status(400).JSON(fiber.Map{
				"error": "One or more tickets are not available",
			})
		}
		tickets = append(tickets, ticket)
		totalPrice += ticket.Price
		serviceFees += ticket.ServiceFee

		// Mark ticket as unavailable
		ticket.Available = false
		db.Tickets[ticketID] = ticket
	}

	// Create order
	order := Order{
		ID:             uuid.New().String(),
		UserEmail:      req.UserEmail,
		Event:          event,
		Tickets:        tickets,
		TotalPrice:     totalPrice,
		ServiceFees:    serviceFees,
		DeliveryMethod: req.DeliveryMethod,
		Status:         "confirmed",
		PurchaseDate:   time.Now(),
		PaymentMethod:  req.PaymentMethod,
	}

	db.Orders[order.ID] = order

	return c.Status(201).JSON(order)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[0:len(substr)] == substr
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Events:  make(map[string]Event),
		Tickets: make(map[string]Ticket),
		Orders:  make(map[string]Order),
		Users:   make(map[string]User),
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

	// Event routes
	api.Get("/events", searchEvents)
	api.Get("/events/:eventId", getEventDetails)
	api.Get("/events/:eventId/tickets", getEventTickets)

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

	app := fiber.New()

	app.Use(logger.New())
	app.Use(cors.New())

	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

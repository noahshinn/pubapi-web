package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/google/uuid"
)

// Data models
type Seller struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Rating     float64   `json:"rating"`
	TotalSales int       `json:"total_sales"`
	JoinedDate time.Time `json:"joined_date"`
}

type Listing struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	Price         float64   `json:"price"`
	ShippingPrice float64   `json:"shipping_price"`
	Category      string    `json:"category"`
	Seller        Seller    `json:"seller"`
	Images        []string  `json:"images"`
	Tags          []string  `json:"tags"`
	CreatedAt     time.Time `json:"created_at"`
}

type CartItem struct {
	Listing         Listing `json:"listing"`
	Quantity        int     `json:"quantity"`
	Personalization string  `json:"personalization"`
}

type Cart struct {
	UserEmail     string     `json:"user_email"`
	Items         []CartItem `json:"items"`
	Subtotal      float64    `json:"subtotal"`
	ShippingTotal float64    `json:"shipping_total"`
	Total         float64    `json:"total"`
}

type Order struct {
	ID              string     `json:"id"`
	UserEmail       string     `json:"user_email"`
	Items           []CartItem `json:"items"`
	Status          string     `json:"status"`
	ShippingAddress string     `json:"shipping_address"`
	PaymentMethod   string     `json:"payment_method"`
	Subtotal        float64    `json:"subtotal"`
	ShippingTotal   float64    `json:"shipping_total"`
	Total           float64    `json:"total"`
	CreatedAt       time.Time  `json:"created_at"`
}

type User struct {
	Email            string   `json:"email"`
	Name             string   `json:"name"`
	Address          string   `json:"address"`
	PaymentMethods   []string `json:"payment_methods"`
	FavoriteListings []string `json:"favorite_listings"`
}

// Database
type Database struct {
	Users    map[string]User    `json:"users"`
	Listings map[string]Listing `json:"listings"`
	Carts    map[string]Cart    `json:"carts"`
	Orders   map[string]Order   `json:"orders"`
	mu       sync.RWMutex
}

var db *Database

// Database operations
func (d *Database) GetUser(email string) (User, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if user, ok := d.Users[email]; ok {
		return user, nil
	}
	return User{}, fiber.NewError(fiber.StatusNotFound, "User not found")
}

func (d *Database) GetListing(id string) (Listing, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if listing, ok := d.Listings[id]; ok {
		return listing, nil
	}
	return Listing{}, fiber.NewError(fiber.StatusNotFound, "Listing not found")
}

func (d *Database) GetCart(email string) (Cart, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if cart, ok := d.Carts[email]; ok {
		return cart, nil
	}
	return Cart{UserEmail: email}, nil
}

func (d *Database) UpdateCart(cart Cart) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Carts[cart.UserEmail] = cart
	return nil
}

func (d *Database) CreateOrder(order Order) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Orders[order.ID] = order
	return nil
}

// Handlers
func searchListings(c *fiber.Ctx) error {
	query := c.Query("query")
	category := c.Query("category")
	maxPrice := c.QueryFloat("max_price", 0)
	minPrice := c.QueryFloat("min_price", 0)

	var results []Listing

	db.mu.RLock()
	for _, listing := range db.Listings {
		if (query == "" || contains(listing.Title, query) || contains(listing.Description, query)) &&
			(category == "" || listing.Category == category) &&
			(maxPrice == 0 || listing.Price <= maxPrice) &&
			(minPrice == 0 || listing.Price >= minPrice) {
			results = append(results, listing)
		}
	}
	db.mu.RUnlock()

	return c.JSON(results)
}

func getCart(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Email is required")
	}

	cart, err := db.GetCart(email)
	if err != nil {
		return err
	}

	return c.JSON(cart)
}

func addToCart(c *fiber.Ctx) error {
	var req struct {
		UserEmail       string `json:"user_email"`
		ListingID       string `json:"listing_id"`
		Quantity        int    `json:"quantity"`
		Personalization string `json:"personalization"`
	}

	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	listing, err := db.GetListing(req.ListingID)
	if err != nil {
		return err
	}

	cart, _ := db.GetCart(req.UserEmail)

	// Add or update item in cart
	itemExists := false
	for i, item := range cart.Items {
		if item.Listing.ID == req.ListingID {
			cart.Items[i].Quantity += req.Quantity
			itemExists = true
			break
		}
	}

	if !itemExists {
		cart.Items = append(cart.Items, CartItem{
			Listing:         listing,
			Quantity:        req.Quantity,
			Personalization: req.Personalization,
		})
	}

	// Recalculate totals
	cart.Subtotal = 0
	cart.ShippingTotal = 0
	for _, item := range cart.Items {
		cart.Subtotal += item.Listing.Price * float64(item.Quantity)
		cart.ShippingTotal += item.Listing.ShippingPrice * float64(item.Quantity)
	}
	cart.Total = cart.Subtotal + cart.ShippingTotal

	if err := db.UpdateCart(cart); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to update cart")
	}

	return c.JSON(cart)
}

func placeOrder(c *fiber.Ctx) error {
	var req struct {
		UserEmail       string `json:"user_email"`
		ShippingAddress string `json:"shipping_address"`
		PaymentMethod   string `json:"payment_method"`
	}

	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	// Get user's cart
	cart, err := db.GetCart(req.UserEmail)
	if err != nil {
		return err
	}

	if len(cart.Items) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "Cart is empty")
	}

	// Create order
	order := Order{
		ID:              uuid.New().String(),
		UserEmail:       req.UserEmail,
		Items:           cart.Items,
		Status:          "pending",
		ShippingAddress: req.ShippingAddress,
		PaymentMethod:   req.PaymentMethod,
		Subtotal:        cart.Subtotal,
		ShippingTotal:   cart.ShippingTotal,
		Total:           cart.Total,
		CreatedAt:       time.Now(),
	}

	if err := db.CreateOrder(order); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create order")
	}

	// Clear cart after successful order
	cart.Items = []CartItem{}
	cart.Subtotal = 0
	cart.ShippingTotal = 0
	cart.Total = 0
	if err := db.UpdateCart(cart); err != nil {
		log.Printf("Failed to clear cart after order: %v", err)
	}

	return c.Status(fiber.StatusCreated).JSON(order)
}

func getFavorites(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Email is required")
	}

	user, err := db.GetUser(email)
	if err != nil {
		return err
	}

	var favorites []Listing
	for _, listingID := range user.FavoriteListings {
		if listing, err := db.GetListing(listingID); err == nil {
			favorites = append(favorites, listing)
		}
	}

	return c.JSON(favorites)
}

func getOrders(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Email is required")
	}

	var orders []Order
	db.mu.RLock()
	for _, order := range db.Orders {
		if order.UserEmail == email {
			orders = append(orders, order)
		}
	}
	db.mu.RUnlock()

	return c.JSON(orders)
}

// Utility functions
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:    make(map[string]User),
		Listings: make(map[string]Listing),
		Carts:    make(map[string]Cart),
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

	api.Get("/listings", searchListings)
	api.Get("/cart", getCart)
	api.Post("/cart", addToCart)
	api.Get("/orders", getOrders)
	api.Post("/orders", placeOrder)
	api.Get("/favorites", getFavorites)
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
	app.Use(cors.New())

	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

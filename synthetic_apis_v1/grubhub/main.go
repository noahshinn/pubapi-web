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
type CustomizationChoice struct {
	Name  string  `json:"name"`
	Price float64 `json:"price"`
}

type CustomizationOption struct {
	Name    string                `json:"name"`
	Choices []CustomizationChoice `json:"choices"`
}

type MenuItem struct {
	ID                   string                `json:"id"`
	Name                 string                `json:"name"`
	Description          string                `json:"description"`
	Price                float64               `json:"price"`
	Category             string                `json:"category"`
	CustomizationOptions []CustomizationOption `json:"customization_options"`
	Available            bool                  `json:"available"`
}

type Restaurant struct {
	ID                    string     `json:"id"`
	Name                  string     `json:"name"`
	CuisineType           string     `json:"cuisine_type"`
	Rating                float64    `json:"rating"`
	EstimatedDeliveryTime int        `json:"estimated_delivery_time"`
	DeliveryFee           float64    `json:"delivery_fee"`
	MinimumOrder          float64    `json:"minimum_order"`
	Address               string     `json:"address"`
	Latitude              float64    `json:"latitude"`
	Longitude             float64    `json:"longitude"`
	Menu                  []MenuItem `json:"menu"`
	IsOpen                bool       `json:"is_open"`
}

type CartItemCustomization struct {
	OptionName string `json:"option_name"`
	Choice     string `json:"choice"`
}

type CartItem struct {
	MenuItemID          string                  `json:"menu_item_id"`
	Quantity            int                     `json:"quantity"`
	Customizations      []CartItemCustomization `json:"customizations"`
	SpecialInstructions string                  `json:"special_instructions"`
	Price               float64                 `json:"price"`
}

type Cart struct {
	ID           string     `json:"id"`
	UserEmail    string     `json:"user_email"`
	RestaurantID string     `json:"restaurant_id"`
	Items        []CartItem `json:"items"`
	Subtotal     float64    `json:"subtotal"`
	Tax          float64    `json:"tax"`
	DeliveryFee  float64    `json:"delivery_fee"`
	Total        float64    `json:"total"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type Order struct {
	ID              string    `json:"id"`
	UserEmail       string    `json:"user_email"`
	Cart            Cart      `json:"cart"`
	Status          string    `json:"status"`
	DeliveryAddress string    `json:"delivery_address"`
	PaymentMethodID string    `json:"payment_method_id"`
	TipAmount       float64   `json:"tip_amount"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Database struct {
	Restaurants map[string]Restaurant `json:"restaurants"`
	Carts       map[string]Cart       `json:"carts"`
	Orders      map[string]Order      `json:"orders"`
	mu          sync.RWMutex
}

var (
	db                    *Database
	ErrRestaurantNotFound = errors.New("restaurant not found")
	ErrCartNotFound       = errors.New("cart not found")
	ErrOrderNotFound      = errors.New("order not found")
)

// Database operations
func (d *Database) GetRestaurant(id string) (Restaurant, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	restaurant, exists := d.Restaurants[id]
	if !exists {
		return Restaurant{}, ErrRestaurantNotFound
	}
	return restaurant, nil
}

func (d *Database) GetCart(id string) (Cart, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	cart, exists := d.Carts[id]
	if !exists {
		return Cart{}, ErrCartNotFound
	}
	return cart, nil
}

func (d *Database) UpdateCart(cart Cart) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Carts[cart.ID] = cart
	return nil
}

func (d *Database) CreateOrder(order Order) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Orders[order.ID] = order
	return nil
}

// Handlers
func searchHandler(c *fiber.Ctx) error {
	query := c.Query("query")
	lat := c.QueryFloat("latitude", 0)
	lon := c.QueryFloat("longitude", 0)
	cuisine := c.Query("cuisine")

	if lat == 0 || lon == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "latitude and longitude are required",
		})
	}

	var results []Restaurant
	db.mu.RLock()
	for _, restaurant := range db.Restaurants {
		// Filter by location (simplified)
		distance := calculateDistance(lat, lon, restaurant.Latitude, restaurant.Longitude)
		if distance > 10 { // 10km radius
			continue
		}

		// Filter by cuisine if specified
		if cuisine != "" && restaurant.CuisineType != cuisine {
			continue
		}

		// Filter by search query if specified
		if query != "" {
			matches := false
			// Search in restaurant name
			if contains(restaurant.Name, query) {
				matches = true
			}
			// Search in menu items
			for _, item := range restaurant.Menu {
				if contains(item.Name, query) || contains(item.Description, query) {
					matches = true
					break
				}
			}
			if !matches {
				continue
			}
		}

		results = append(results, restaurant)
	}
	db.mu.RUnlock()

	return c.JSON(fiber.Map{
		"restaurants": results,
	})
}

func getRestaurantMenu(c *fiber.Ctx) error {
	restaurantId := c.Params("restaurantId")

	restaurant, err := db.GetRestaurant(restaurantId)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Restaurant not found",
		})
	}

	return c.JSON(restaurant.Menu)
}

func getCart(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	var userCart Cart
	found := false

	db.mu.RLock()
	for _, cart := range db.Carts {
		if cart.UserEmail == email {
			userCart = cart
			found = true
			break
		}
	}
	db.mu.RUnlock()

	if !found {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Cart not found",
		})
	}

	return c.JSON(userCart)
}

func addToCart(c *fiber.Ctx) error {
	var req struct {
		UserEmail    string   `json:"user_email"`
		RestaurantID string   `json:"restaurant_id"`
		Item         CartItem `json:"item"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Find or create cart
	var cart Cart
	found := false

	db.mu.RLock()
	for _, c := range db.Carts {
		if c.UserEmail == req.UserEmail {
			cart = c
			found = true
			break
		}
	}
	db.mu.RUnlock()

	if !found {
		cart = Cart{
			ID:           uuid.New().String(),
			UserEmail:    req.UserEmail,
			RestaurantID: req.RestaurantID,
			Items:        []CartItem{},
			CreatedAt:    time.Now(),
		}
	} else if cart.RestaurantID != req.RestaurantID {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot add items from different restaurants to cart",
		})
	}

	// Validate menu item
	restaurant, err := db.GetRestaurant(req.RestaurantID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Restaurant not found",
		})
	}

	var menuItem *MenuItem
	for _, item := range restaurant.Menu {
		if item.ID == req.Item.MenuItemID {
			menuItem = &item
			break
		}
	}

	if menuItem == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Menu item not found",
		})
	}

	// Add item to cart
	cart.Items = append(cart.Items, req.Item)
	cart.UpdatedAt = time.Now()

	// Recalculate totals
	cart.Subtotal = 0
	for _, item := range cart.Items {
		cart.Subtotal += item.Price * float64(item.Quantity)
	}
	cart.Tax = cart.Subtotal * 0.0825 // 8.25% tax
	cart.DeliveryFee = restaurant.DeliveryFee
	cart.Total = cart.Subtotal + cart.Tax + cart.DeliveryFee

	// Save cart
	if err := db.UpdateCart(cart); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update cart",
		})
	}

	return c.JSON(cart)
}

func placeOrder(c *fiber.Ctx) error {
	var req struct {
		Email           string  `json:"email"`
		CartID          string  `json:"cart_id"`
		DeliveryAddress string  `json:"delivery_address"`
		PaymentMethodID string  `json:"payment_method_id"`
		TipAmount       float64 `json:"tip_amount"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Get cart
	cart, err := db.GetCart(req.CartID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Cart not found",
		})
	}

	// Validate cart belongs to user
	if cart.UserEmail != req.Email {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Unauthorized",
		})
	}

	// Create order
	order := Order{
		ID:              uuid.New().String(),
		UserEmail:       req.Email,
		Cart:            cart,
		Status:          "pending",
		DeliveryAddress: req.DeliveryAddress,
		PaymentMethodID: req.PaymentMethodID,
		TipAmount:       req.TipAmount,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if err := db.CreateOrder(order); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create order",
		})
	}

	// Clear cart
	delete(db.Carts, cart.ID)

	return c.Status(fiber.StatusCreated).JSON(order)
}

// Utility functions
func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	// Simplified distance calculation
	return ((lat2 - lat1) * (lat2 - lat1)) + ((lon2 - lon1) * (lon2 - lon1))
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
		Restaurants: make(map[string]Restaurant),
		Carts:       make(map[string]Cart),
		Orders:      make(map[string]Order),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	api.Get("/search", searchHandler)
	api.Get("/restaurants/:restaurantId/menu", getRestaurantMenu)
	api.Get("/cart", getCart)
	api.Post("/cart", addToCart)
	api.Post("/orders", placeOrder)
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

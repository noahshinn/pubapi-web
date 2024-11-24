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
	"github.com/google/uuid"
)

// Data models
type Pet struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Type                string   `json:"type"`
	Breed               string   `json:"breed"`
	BirthDate           string   `json:"birth_date"`
	Weight              float64  `json:"weight"`
	DietaryRestrictions []string `json:"dietary_restrictions"`
}

type Product struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	Brand            string  `json:"brand"`
	Category         string  `json:"category"`
	PetType          string  `json:"pet_type"`
	Price            float64 `json:"price"`
	AutoshipPrice    float64 `json:"autoship_price"`
	AutoshipEligible bool    `json:"autoship_eligible"`
	InStock          bool    `json:"in_stock"`
	Description      string  `json:"description"`
	Rating           float64 `json:"rating"`
}

type OrderItem struct {
	ProductID string  `json:"product_id"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
}

type Order struct {
	ID              string      `json:"id"`
	UserEmail       string      `json:"user_email"`
	Items           []OrderItem `json:"items"`
	Status          string      `json:"status"`
	Total           float64     `json:"total"`
	ShippingAddress string      `json:"shipping_address"`
	TrackingNumber  string      `json:"tracking_number"`
	CreatedAt       time.Time   `json:"created_at"`
}

type AutoshipSubscription struct {
	ID              string    `json:"id"`
	UserEmail       string    `json:"user_email"`
	ProductID       string    `json:"product_id"`
	Quantity        int       `json:"quantity"`
	FrequencyDays   int       `json:"frequency_days"`
	NextDelivery    time.Time `json:"next_delivery"`
	ShippingAddress string    `json:"shipping_address"`
	Status          string    `json:"status"`
}

type Database struct {
	Pets     map[string][]Pet                  `json:"pets"`     // key: user_email
	Products map[string]Product                `json:"products"` // key: product_id
	Orders   map[string][]Order                `json:"orders"`   // key: user_email
	Autoship map[string][]AutoshipSubscription `json:"autoship"` // key: user_email
	mu       sync.RWMutex
}

var db *Database

// Database operations
func (d *Database) GetPets(email string) []Pet {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.Pets[email]
}

func (d *Database) AddPet(email string, pet Pet) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.Pets == nil {
		d.Pets = make(map[string][]Pet)
	}
	d.Pets[email] = append(d.Pets[email], pet)
}

func (d *Database) GetProducts(category, petType, brand string) []Product {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var products []Product
	for _, product := range d.Products {
		if (category == "" || product.Category == category) &&
			(petType == "" || product.PetType == petType) &&
			(brand == "" || product.Brand == brand) {
			products = append(products, product)
		}
	}
	return products
}

func (d *Database) GetOrders(email string) []Order {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.Orders[email]
}

func (d *Database) AddOrder(email string, order Order) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.Orders == nil {
		d.Orders = make(map[string][]Order)
	}
	d.Orders[email] = append(d.Orders[email], order)
}

func (d *Database) GetAutoship(email string) []AutoshipSubscription {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.Autoship[email]
}

// HTTP Handlers
func getPets(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	pets := db.GetPets(email)
	return c.JSON(pets)
}

func addPet(c *fiber.Ctx) error {
	var pet Pet
	if err := c.BodyParser(&pet); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	pet.ID = uuid.New().String()
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	db.AddPet(email, pet)
	return c.Status(fiber.StatusCreated).JSON(pet)
}

func getProducts(c *fiber.Ctx) error {
	category := c.Query("category")
	petType := c.Query("pet_type")
	brand := c.Query("brand")

	products := db.GetProducts(category, petType, brand)
	return c.JSON(products)
}

func getOrders(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	orders := db.GetOrders(email)
	return c.JSON(orders)
}

func createOrder(c *fiber.Ctx) error {
	var newOrder struct {
		UserEmail       string      `json:"user_email"`
		Items           []OrderItem `json:"items"`
		ShippingAddress string      `json:"shipping_address"`
	}

	if err := c.BodyParser(&newOrder); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Calculate total
	var total float64
	for _, item := range newOrder.Items {
		if product, exists := db.Products[item.ProductID]; exists {
			total += product.Price * float64(item.Quantity)
		} else {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid product ID",
			})
		}
	}

	order := Order{
		ID:              uuid.New().String(),
		UserEmail:       newOrder.UserEmail,
		Items:           newOrder.Items,
		Status:          "processing",
		Total:           total,
		ShippingAddress: newOrder.ShippingAddress,
		TrackingNumber:  "",
		CreatedAt:       time.Now(),
	}

	db.AddOrder(newOrder.UserEmail, order)
	return c.Status(fiber.StatusCreated).JSON(order)
}

func getAutoship(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	subscriptions := db.GetAutoship(email)
	return c.JSON(subscriptions)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &db)
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

	// Pet routes
	api.Get("/pets", getPets)
	api.Post("/pets", addPet)

	// Product routes
	api.Get("/products", getProducts)

	// Order routes
	api.Get("/orders", getOrders)
	api.Post("/orders", createOrder)

	// Autoship routes
	api.Get("/autoship", getAutoship)
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
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE",
		AllowHeaders: "Origin, Content-Type, Accept",
	}))

	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

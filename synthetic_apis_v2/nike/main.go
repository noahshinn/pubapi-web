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

type Product struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Category    string    `json:"category"`
	Gender      string    `json:"gender"`
	Price       float64   `json:"price"`
	Colors      []string  `json:"colors"`
	Sizes       []string  `json:"sizes"`
	Description string    `json:"description"`
	Images      []string  `json:"images"`
	CreatedAt   time.Time `json:"created_at"`
}

type Workout struct {
	ID        string    `json:"id"`
	UserEmail string    `json:"user_email"`
	Type      string    `json:"type"`
	Duration  int       `json:"duration"`
	Distance  float64   `json:"distance"`
	Calories  int       `json:"calories"`
	Date      time.Time `json:"date"`
	Shoes     *Product  `json:"shoes"`
}

type OrderItem struct {
	Product  Product `json:"product"`
	Size     string  `json:"size"`
	Color    string  `json:"color"`
	Quantity int     `json:"quantity"`
}

type Order struct {
	ID              string      `json:"id"`
	UserEmail       string      `json:"user_email"`
	Items           []OrderItem `json:"items"`
	Total           float64     `json:"total"`
	Status          string      `json:"status"`
	ShippingAddress string      `json:"shipping_address"`
	CreatedAt       time.Time   `json:"created_at"`
}

type User struct {
	Email           string `json:"email"`
	Name            string `json:"name"`
	ShippingAddress string `json:"shipping_address"`
	WorkoutStats    struct {
		TotalWorkouts int     `json:"total_workouts"`
		TotalDistance float64 `json:"total_distance"`
		TotalDuration int     `json:"total_duration"`
	} `json:"workout_stats"`
}

type Database struct {
	Products map[string]Product `json:"products"`
	Workouts map[string]Workout `json:"workouts"`
	Orders   map[string]Order   `json:"orders"`
	Users    map[string]User    `json:"users"`
	mu       sync.RWMutex
}

var db *Database

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Products: make(map[string]Product),
		Workouts: make(map[string]Workout),
		Orders:   make(map[string]Order),
		Users:    make(map[string]User),
	}

	return json.Unmarshal(data, db)
}

func getProducts(c *fiber.Ctx) error {
	category := c.Query("category")
	gender := c.Query("gender")

	db.mu.RLock()
	defer db.mu.RUnlock()

	var products []Product
	for _, product := range db.Products {
		if (category == "" || product.Category == category) &&
			(gender == "" || product.Gender == gender) {
			products = append(products, product)
		}
	}

	return c.JSON(products)
}

func getWorkouts(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	var workouts []Workout
	for _, workout := range db.Workouts {
		if workout.UserEmail == email {
			workouts = append(workouts, workout)
		}
	}

	return c.JSON(workouts)
}

type NewWorkout struct {
	UserEmail string  `json:"user_email"`
	Type      string  `json:"type"`
	Duration  int     `json:"duration"`
	Distance  float64 `json:"distance"`
	ShoeID    string  `json:"shoe_id"`
}

func logWorkout(c *fiber.Ctx) error {
	var newWorkout NewWorkout
	if err := c.BodyParser(&newWorkout); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	// Validate user exists
	user, exists := db.Users[newWorkout.UserEmail]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// Calculate calories (simplified)
	calories := int(float64(newWorkout.Duration) * 7.5)

	var shoes *Product
	if newWorkout.ShoeID != "" {
		if product, exists := db.Products[newWorkout.ShoeID]; exists {
			shoes = &product
		}
	}

	workout := Workout{
		ID:        uuid.New().String(),
		UserEmail: newWorkout.UserEmail,
		Type:      newWorkout.Type,
		Duration:  newWorkout.Duration,
		Distance:  newWorkout.Distance,
		Calories:  calories,
		Date:      time.Now(),
		Shoes:     shoes,
	}

	// Update user stats
	user.WorkoutStats.TotalWorkouts++
	user.WorkoutStats.TotalDistance += newWorkout.Distance
	user.WorkoutStats.TotalDuration += newWorkout.Duration
	db.Users[newWorkout.UserEmail] = user

	db.Workouts[workout.ID] = workout

	return c.Status(fiber.StatusCreated).JSON(workout)
}

func getOrders(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	var orders []Order
	for _, order := range db.Orders {
		if order.UserEmail == email {
			orders = append(orders, order)
		}
	}

	return c.JSON(orders)
}

type NewOrder struct {
	UserEmail string `json:"user_email"`
	Items     []struct {
		ProductID string `json:"product_id"`
		Size      string `json:"size"`
		Color     string `json:"color"`
		Quantity  int    `json:"quantity"`
	} `json:"items"`
	ShippingAddress string `json:"shipping_address"`
}

func createOrder(c *fiber.Ctx) error {
	var newOrder NewOrder
	if err := c.BodyParser(&newOrder); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	// Validate user exists
	if _, exists := db.Users[newOrder.UserEmail]; !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	var orderItems []OrderItem
	var total float64

	for _, item := range newOrder.Items {
		product, exists := db.Products[item.ProductID]
		if !exists {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Product not found: " + item.ProductID,
			})
		}

		// Validate size and color
		validSize := false
		for _, size := range product.Sizes {
			if size == item.Size {
				validSize = true
				break
			}
		}
		if !validSize {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid size for product: " + product.ID,
			})
		}

		validColor := false
		for _, color := range product.Colors {
			if color == item.Color {
				validColor = true
				break
			}
		}
		if !validColor {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid color for product: " + product.ID,
			})
		}

		orderItems = append(orderItems, OrderItem{
			Product:  product,
			Size:     item.Size,
			Color:    item.Color,
			Quantity: item.Quantity,
		})

		total += product.Price * float64(item.Quantity)
	}

	order := Order{
		ID:              uuid.New().String(),
		UserEmail:       newOrder.UserEmail,
		Items:           orderItems,
		Total:           total,
		Status:          "pending",
		ShippingAddress: newOrder.ShippingAddress,
		CreatedAt:       time.Now(),
	}

	db.Orders[order.ID] = order

	return c.Status(fiber.StatusCreated).JSON(order)
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

	// Product routes
	api.Get("/products", getProducts)

	// Workout routes
	api.Get("/workouts", getWorkouts)
	api.Post("/workouts", logWorkout)

	// Order routes
	api.Get("/orders", getOrders)
	api.Post("/orders", createOrder)
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

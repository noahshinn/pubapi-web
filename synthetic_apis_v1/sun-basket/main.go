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
type DietaryPreference string

const (
	Paleo         DietaryPreference = "paleo"
	Vegetarian    DietaryPreference = "vegetarian"
	Vegan         DietaryPreference = "vegan"
	GlutenFree    DietaryPreference = "gluten-free"
	DairyFree     DietaryPreference = "dairy-free"
	Mediterranean DietaryPreference = "mediterranean"
)

type MealPlan struct {
	ID                 string              `json:"id"`
	Name               string              `json:"name"`
	MealsPerWeek       int                 `json:"meals_per_week"`
	ServingsPerMeal    int                 `json:"servings_per_meal"`
	PricePerServing    float64             `json:"price_per_serving"`
	Description        string              `json:"description"`
	DietaryPreferences []DietaryPreference `json:"dietary_preferences"`
}

type Recipe struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	PrepTime    int                 `json:"prep_time"`
	Calories    int                 `json:"calories"`
	Protein     int                 `json:"protein"`
	Carbs       int                 `json:"carbs"`
	Fat         int                 `json:"fat"`
	Ingredients []string            `json:"ingredients"`
	DietaryTags []DietaryPreference `json:"dietary_tags"`
	ImageURL    string              `json:"image_url"`
}

type WeeklyMenu struct {
	WeekOf  time.Time `json:"week_of"`
	Recipes []Recipe  `json:"recipes"`
}

type DeliveryStatus string

const (
	Scheduled  DeliveryStatus = "scheduled"
	Processing DeliveryStatus = "processing"
	Shipped    DeliveryStatus = "shipped"
	Delivered  DeliveryStatus = "delivered"
	Cancelled  DeliveryStatus = "cancelled"
)

type Delivery struct {
	ID             string         `json:"id"`
	SubscriptionID string         `json:"subscription_id"`
	DeliveryDate   time.Time      `json:"delivery_date"`
	Status         DeliveryStatus `json:"status"`
	TrackingNumber string         `json:"tracking_number"`
	Recipes        []Recipe       `json:"recipes"`
}

type Subscription struct {
	ID                 string              `json:"id"`
	UserEmail          string              `json:"user_email"`
	MealPlan           MealPlan            `json:"meal_plan"`
	DietaryPreferences []DietaryPreference `json:"dietary_preferences"`
	DeliveryDay        string              `json:"delivery_day"`
	Status             string              `json:"status"`
	NextDelivery       time.Time           `json:"next_delivery"`
	PaymentMethod      string              `json:"payment_method"`
}

// Database represents our in-memory database
type Database struct {
	MealPlans     map[string]MealPlan     `json:"meal_plans"`
	Recipes       map[string]Recipe       `json:"recipes"`
	WeeklyMenus   map[string]WeeklyMenu   `json:"weekly_menus"`
	Subscriptions map[string]Subscription `json:"subscriptions"`
	Deliveries    map[string]Delivery     `json:"deliveries"`
	mu            sync.RWMutex
}

var (
	db          *Database
	ErrNotFound = errors.New("not found")
)

// Database operations
func (d *Database) GetMealPlans() []MealPlan {
	d.mu.RLock()
	defer d.mu.RUnlock()

	plans := make([]MealPlan, 0, len(d.MealPlans))
	for _, plan := range d.MealPlans {
		plans = append(plans, plan)
	}
	return plans
}

func (d *Database) GetWeeklyMenu(weekOf time.Time) (WeeklyMenu, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	for _, menu := range d.WeeklyMenus {
		if menu.WeekOf.Equal(weekOf) {
			return menu, nil
		}
	}
	return WeeklyMenu{}, ErrNotFound
}

func (d *Database) GetSubscription(email string) (Subscription, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	for _, sub := range d.Subscriptions {
		if sub.UserEmail == email {
			return sub, nil
		}
	}
	return Subscription{}, ErrNotFound
}

func (d *Database) CreateSubscription(sub Subscription) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Subscriptions[sub.ID] = sub
	return nil
}

func (d *Database) GetDeliveries(email string) []Delivery {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var userDeliveries []Delivery
	for _, delivery := range d.Deliveries {
		sub := d.Subscriptions[delivery.SubscriptionID]
		if sub.UserEmail == email {
			userDeliveries = append(userDeliveries, delivery)
		}
	}
	return userDeliveries
}

// HTTP Handlers
func getMealPlans(c *fiber.Ctx) error {
	plans := db.GetMealPlans()
	return c.JSON(plans)
}

func getWeeklyMenu(c *fiber.Ctx) error {
	weekStr := c.Query("week")
	var weekOf time.Time
	var err error

	if weekStr == "" {
		// Default to next Monday
		weekOf = time.Now()
		for weekOf.Weekday() != time.Monday {
			weekOf = weekOf.AddDate(0, 0, 1)
		}
	} else {
		weekOf, err = time.Parse("2006-01-02", weekStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid date format. Use YYYY-MM-DD",
			})
		}
	}

	menu, err := db.GetWeeklyMenu(weekOf)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Menu not found for specified week",
		})
	}

	return c.JSON(menu)
}

func getSubscription(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email parameter is required",
		})
	}

	sub, err := db.GetSubscription(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Subscription not found",
		})
	}

	return c.JSON(sub)
}

type SubscriptionRequest struct {
	UserEmail          string              `json:"user_email"`
	MealPlanID         string              `json:"meal_plan_id"`
	DietaryPreferences []DietaryPreference `json:"dietary_preferences"`
	DeliveryDay        string              `json:"delivery_day"`
	PaymentMethod      string              `json:"payment_method"`
}

func createSubscription(c *fiber.Ctx) error {
	var req SubscriptionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate meal plan
	mealPlan, exists := db.MealPlans[req.MealPlanID]
	if !exists {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid meal plan ID",
		})
	}

	// Validate delivery day
	validDays := map[string]bool{
		"Monday": true, "Tuesday": true, "Wednesday": true,
		"Thursday": true, "Friday": true,
	}
	if !validDays[req.DeliveryDay] {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid delivery day",
		})
	}

	// Calculate next delivery
	nextDelivery := time.Now()
	for nextDelivery.Weekday().String() != req.DeliveryDay {
		nextDelivery = nextDelivery.AddDate(0, 0, 1)
	}

	// Create new subscription
	subscription := Subscription{
		ID:                 uuid.New().String(),
		UserEmail:          req.UserEmail,
		MealPlan:           mealPlan,
		DietaryPreferences: req.DietaryPreferences,
		DeliveryDay:        req.DeliveryDay,
		Status:             "active",
		NextDelivery:       nextDelivery,
		PaymentMethod:      req.PaymentMethod,
	}

	if err := db.CreateSubscription(subscription); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create subscription",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(subscription)
}

func getDeliveries(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email parameter is required",
		})
	}

	deliveries := db.GetDeliveries(email)
	return c.JSON(deliveries)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		MealPlans:     make(map[string]MealPlan),
		Recipes:       make(map[string]Recipe),
		WeeklyMenus:   make(map[string]WeeklyMenu),
		Subscriptions: make(map[string]Subscription),
		Deliveries:    make(map[string]Delivery),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	api.Get("/meal-plans", getMealPlans)
	api.Get("/weekly-menu", getWeeklyMenu)
	api.Get("/subscriptions", getSubscription)
	api.Post("/subscriptions", createSubscription)
	api.Get("/deliveries", getDeliveries)
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

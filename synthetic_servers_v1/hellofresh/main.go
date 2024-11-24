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
	Vegetarian  DietaryPreference = "vegetarian"
	Vegan       DietaryPreference = "vegan"
	Pescatarian DietaryPreference = "pescatarian"
	LowCarb     DietaryPreference = "low_carb"
	Keto        DietaryPreference = "keto"
)

type MealPlan struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	MealsPerWeek    int     `json:"meals_per_week"`
	ServingsPerMeal int     `json:"servings_per_meal"`
	PricePerServing float64 `json:"price_per_serving"`
	Description     string  `json:"description"`
}

type Recipe struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	PrepTime    int      `json:"prep_time"`
	Difficulty  string   `json:"difficulty"`
	Calories    int      `json:"calories"`
	Ingredients []string `json:"ingredients"`
	Tags        []string `json:"tags"`
	ImageURL    string   `json:"image_url"`
}

type Subscription struct {
	ID                 string              `json:"id"`
	UserEmail          string              `json:"user_email"`
	MealPlan           MealPlan            `json:"meal_plan"`
	DeliveryDay        string              `json:"delivery_day"`
	Status             string              `json:"status"`
	NextDelivery       time.Time           `json:"next_delivery"`
	DietaryPreferences []DietaryPreference `json:"dietary_preferences"`
	CreatedAt          time.Time           `json:"created_at"`
	UpdatedAt          time.Time           `json:"updated_at"`
}

type WeeklySelection struct {
	ID             string    `json:"id"`
	UserEmail      string    `json:"user_email"`
	Week           time.Time `json:"week"`
	Recipes        []Recipe  `json:"recipes"`
	DeliveryStatus string    `json:"delivery_status"`
	DeliveryDate   time.Time `json:"delivery_date"`
	CreatedAt      time.Time `json:"created_at"`
}

// Database represents our in-memory database
type Database struct {
	MealPlans        map[string]MealPlan        `json:"meal_plans"`
	Recipes          map[string]Recipe          `json:"recipes"`
	Subscriptions    map[string]Subscription    `json:"subscriptions"`
	WeeklySelections map[string]WeeklySelection `json:"weekly_selections"`
	mu               sync.RWMutex
}

// Custom errors
var (
	ErrUserNotFound         = errors.New("user not found")
	ErrMealPlanNotFound     = errors.New("meal plan not found")
	ErrRecipeNotFound       = errors.New("recipe not found")
	ErrInvalidInput         = errors.New("invalid input")
	ErrSubscriptionNotFound = errors.New("subscription not found")
)

// Global database instance
var db *Database

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

func (d *Database) GetWeeklyMenu(week time.Time) []Recipe {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// In a real implementation, this would filter recipes based on the week
	recipes := make([]Recipe, 0, len(d.Recipes))
	for _, recipe := range d.Recipes {
		recipes = append(recipes, recipe)
	}
	return recipes
}

func (d *Database) GetSubscription(email string) (Subscription, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	for _, sub := range d.Subscriptions {
		if sub.UserEmail == email {
			return sub, nil
		}
	}
	return Subscription{}, ErrSubscriptionNotFound
}

func (d *Database) CreateOrUpdateSubscription(sub Subscription) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if sub.ID == "" {
		sub.ID = uuid.New().String()
		sub.CreatedAt = time.Now()
	}
	sub.UpdatedAt = time.Now()

	d.Subscriptions[sub.ID] = sub
	return nil
}

func (d *Database) GetWeeklySelection(email string, week time.Time) (WeeklySelection, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	for _, selection := range d.WeeklySelections {
		if selection.UserEmail == email && selection.Week.Equal(week) {
			return selection, nil
		}
	}
	return WeeklySelection{}, errors.New("weekly selection not found")
}

// HTTP Handlers
func getMealPlans(c *fiber.Ctx) error {
	plans := db.GetMealPlans()
	return c.JSON(plans)
}

func getWeeklyMenu(c *fiber.Ctx) error {
	weekStr := c.Query("week")
	week, err := time.Parse("2006-01-02", weekStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid week format",
		})
	}

	recipes := db.GetWeeklyMenu(week)
	return c.JSON(recipes)
}

func getSubscription(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	subscription, err := db.GetSubscription(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(subscription)
}

type SubscriptionRequest struct {
	UserEmail          string              `json:"user_email"`
	MealPlanID         string              `json:"meal_plan_id"`
	DeliveryDay        string              `json:"delivery_day"`
	DietaryPreferences []DietaryPreference `json:"dietary_preferences"`
}

func createOrUpdateSubscription(c *fiber.Ctx) error {
	var req SubscriptionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate meal plan
	mealPlan, exists := db.MealPlans[req.MealPlanID]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "meal plan not found",
		})
	}

	// Create or update subscription
	subscription := Subscription{
		UserEmail:          req.UserEmail,
		MealPlan:           mealPlan,
		DeliveryDay:        req.DeliveryDay,
		Status:             "active",
		NextDelivery:       calculateNextDelivery(req.DeliveryDay),
		DietaryPreferences: req.DietaryPreferences,
	}

	if err := db.CreateOrUpdateSubscription(subscription); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create subscription",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(subscription)
}

type WeeklySelectionRequest struct {
	UserEmail string   `json:"user_email"`
	Week      string   `json:"week"`
	RecipeIDs []string `json:"recipe_ids"`
}

func createWeeklySelection(c *fiber.Ctx) error {
	var req WeeklySelectionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Parse week
	week, err := time.Parse("2006-01-02", req.Week)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid week format",
		})
	}

	// Validate subscription exists
	subscription, err := db.GetSubscription(req.UserEmail)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "subscription not found",
		})
	}

	// Validate recipe count matches subscription
	if len(req.RecipeIDs) != subscription.MealPlan.MealsPerWeek {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "number of recipes must match meal plan",
		})
	}

	// Validate recipes and build recipe list
	var recipes []Recipe
	for _, recipeID := range req.RecipeIDs {
		recipe, exists := db.Recipes[recipeID]
		if !exists {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "recipe not found: " + recipeID,
			})
		}
		recipes = append(recipes, recipe)
	}

	// Create weekly selection
	selection := WeeklySelection{
		ID:             uuid.New().String(),
		UserEmail:      req.UserEmail,
		Week:           week,
		Recipes:        recipes,
		DeliveryStatus: "scheduled",
		DeliveryDate:   calculateDeliveryDate(week, subscription.DeliveryDay),
		CreatedAt:      time.Now(),
	}

	db.mu.Lock()
	db.WeeklySelections[selection.ID] = selection
	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(selection)
}

func getWeeklySelection(c *fiber.Ctx) error {
	email := c.Query("email")
	weekStr := c.Query("week")

	if email == "" || weekStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email and week parameters are required",
		})
	}

	week, err := time.Parse("2006-01-02", weekStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid week format",
		})
	}

	selection, err := db.GetWeeklySelection(email, week)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(selection)
}

// Helper functions
func calculateNextDelivery(deliveryDay string) time.Time {
	now := time.Now()
	weekday := parseWeekday(deliveryDay)
	daysUntilDelivery := (int(weekday) - int(now.Weekday()) + 7) % 7
	if daysUntilDelivery == 0 {
		daysUntilDelivery = 7
	}
	return now.AddDate(0, 0, daysUntilDelivery)
}

func calculateDeliveryDate(week time.Time, deliveryDay string) time.Time {
	weekday := parseWeekday(deliveryDay)
	daysUntilDelivery := (int(weekday) - int(week.Weekday()) + 7) % 7
	return week.AddDate(0, 0, daysUntilDelivery)
}

func parseWeekday(day string) time.Weekday {
	weekdays := map[string]time.Weekday{
		"sunday":    time.Sunday,
		"monday":    time.Monday,
		"tuesday":   time.Tuesday,
		"wednesday": time.Wednesday,
		"thursday":  time.Thursday,
		"friday":    time.Friday,
		"saturday":  time.Saturday,
	}
	return weekdays[day]
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		MealPlans:        make(map[string]MealPlan),
		Recipes:          make(map[string]Recipe),
		Subscriptions:    make(map[string]Subscription),
		WeeklySelections: make(map[string]WeeklySelection),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	api.Get("/meal-plans", getMealPlans)
	api.Get("/weekly-menu", getWeeklyMenu)
	api.Get("/subscriptions", getSubscription)
	api.Post("/subscriptions", createOrUpdateSubscription)
	api.Post("/weekly-selections", createWeeklySelection)
	api.Get("/weekly-selections", getWeeklySelection)
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

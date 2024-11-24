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

type Ingredient struct {
	Name   string  `json:"name"`
	Amount float64 `json:"amount"`
	Unit   string  `json:"unit"`
}

type Recipe struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	PrepTime    int          `json:"prep_time"`
	Difficulty  string       `json:"difficulty"`
	Calories    int          `json:"calories"`
	Ingredients []Ingredient `json:"ingredients"`
	Tags        []string     `json:"tags"`
	ImageURL    string       `json:"image_url"`
}

type MealPlan struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	MealsPerWeek    int     `json:"meals_per_week"`
	ServingsPerMeal int     `json:"servings_per_meal"`
	PricePerServing float64 `json:"price_per_serving"`
	Description     string  `json:"description"`
}

type Subscription struct {
	ID                 string    `json:"id"`
	UserEmail          string    `json:"user_email"`
	MealPlan           MealPlan  `json:"meal_plan"`
	DeliveryDay        string    `json:"delivery_day"`
	Status             string    `json:"status"`
	NextDelivery       time.Time `json:"next_delivery"`
	DietaryPreferences []string  `json:"dietary_preferences"`
}

type WeeklySelection struct {
	UserEmail string    `json:"user_email"`
	Week      time.Time `json:"week"`
	Recipes   []Recipe  `json:"recipes"`
}

type Database struct {
	Users            map[string]User            `json:"users"`
	MealPlans        map[string]MealPlan        `json:"meal_plans"`
	Recipes          map[string]Recipe          `json:"recipes"`
	Subscriptions    map[string]Subscription    `json:"subscriptions"`
	WeeklySelections map[string]WeeklySelection `json:"weekly_selections"`
	WeeklyMenus      map[string][]Recipe        `json:"weekly_menus"`
	mu               sync.RWMutex
}

type User struct {
	Email              string    `json:"email"`
	Name               string    `json:"name"`
	Address            string    `json:"address"`
	Phone              string    `json:"phone"`
	DietaryPreferences []string  `json:"dietary_preferences"`
	CreatedAt          time.Time `json:"created_at"`
}

var db *Database

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:            make(map[string]User),
		MealPlans:        make(map[string]MealPlan),
		Recipes:          make(map[string]Recipe),
		Subscriptions:    make(map[string]Subscription),
		WeeklySelections: make(map[string]WeeklySelection),
		WeeklyMenus:      make(map[string][]Recipe),
	}

	return json.Unmarshal(data, db)
}

func getMealPlans(c *fiber.Ctx) error {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var plans []MealPlan
	for _, plan := range db.MealPlans {
		plans = append(plans, plan)
	}
	return c.JSON(plans)
}

func getWeeklyMenu(c *fiber.Ctx) error {
	week := c.Query("week")
	if week == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "week parameter is required",
		})
	}

	db.mu.RLock()
	menu, exists := db.WeeklyMenus[week]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "menu not found for specified week",
		})
	}

	return c.JSON(menu)
}

func getSubscription(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	for _, sub := range db.Subscriptions {
		if sub.UserEmail == email {
			return c.JSON(sub)
		}
	}

	return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
		"error": "subscription not found",
	})
}

type SubscriptionRequest struct {
	UserEmail          string   `json:"user_email"`
	MealPlanID         string   `json:"meal_plan_id"`
	DeliveryDay        string   `json:"delivery_day"`
	DietaryPreferences []string `json:"dietary_preferences"`
}

func createSubscription(c *fiber.Ctx) error {
	var req SubscriptionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	// Validate user exists
	if _, exists := db.Users[req.UserEmail]; !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "user not found",
		})
	}

	// Validate meal plan exists
	mealPlan, exists := db.MealPlans[req.MealPlanID]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "meal plan not found",
		})
	}

	// Create new subscription
	subscription := Subscription{
		ID:                 uuid.New().String(),
		UserEmail:          req.UserEmail,
		MealPlan:           mealPlan,
		DeliveryDay:        req.DeliveryDay,
		Status:             "active",
		NextDelivery:       time.Now().AddDate(0, 0, 7),
		DietaryPreferences: req.DietaryPreferences,
	}

	db.Subscriptions[subscription.ID] = subscription

	return c.Status(fiber.StatusCreated).JSON(subscription)
}

type WeeklySelectionRequest struct {
	UserEmail string   `json:"user_email"`
	Week      string   `json:"week"`
	RecipeIDs []string `json:"recipe_ids"`
}

func selectWeeklyMeals(c *fiber.Ctx) error {
	var req WeeklySelectionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	// Validate subscription exists
	var userSubscription *Subscription
	for _, sub := range db.Subscriptions {
		if sub.UserEmail == req.UserEmail {
			userSubscription = &sub
			break
		}
	}

	if userSubscription == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "no active subscription found",
		})
	}

	// Validate number of recipes matches subscription
	if len(req.RecipeIDs) != userSubscription.MealPlan.MealsPerWeek {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "number of selected recipes doesn't match meal plan",
		})
	}

	// Collect selected recipes
	var selectedRecipes []Recipe
	for _, recipeID := range req.RecipeIDs {
		recipe, exists := db.Recipes[recipeID]
		if !exists {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "recipe not found: " + recipeID,
			})
		}
		selectedRecipes = append(selectedRecipes, recipe)
	}

	weekTime, err := time.Parse("2006-01-02", req.Week)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid week format",
		})
	}

	selection := WeeklySelection{
		UserEmail: req.UserEmail,
		Week:      weekTime,
		Recipes:   selectedRecipes,
	}

	// Store weekly selection
	selectionKey := req.UserEmail + "_" + req.Week
	db.WeeklySelections[selectionKey] = selection

	return c.JSON(selection)
}

func getWeeklySelection(c *fiber.Ctx) error {
	email := c.Query("email")
	week := c.Query("week")
	if email == "" || week == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email and week parameters are required",
		})
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	selectionKey := email + "_" + week
	selection, exists := db.WeeklySelections[selectionKey]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "weekly selection not found",
		})
	}

	return c.JSON(selection)
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

	// Meal plan routes
	api.Get("/meal-plans", getMealPlans)

	// Weekly menu routes
	api.Get("/weekly-menu", getWeeklyMenu)

	// Subscription routes
	api.Get("/subscriptions", getSubscription)
	api.Post("/subscriptions", createSubscription)

	// Weekly selection routes
	api.Get("/weekly-selections", getWeeklySelection)
	api.Post("/weekly-selections", selectWeeklyMeals)
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

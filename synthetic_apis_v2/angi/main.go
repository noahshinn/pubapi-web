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

type ServiceCategory struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Subcategories []string `json:"subcategories"`
}

type ServiceProvider struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Services    []string `json:"services"`
	Rating      float64  `json:"rating"`
	ReviewCount int      `json:"review_count"`
	Verified    bool     `json:"verified"`
	ServiceArea []string `json:"service_area"`
	Description string   `json:"description"`
	YearsActive int      `json:"years_active"`
	License     string   `json:"license"`
	Insurance   bool     `json:"insurance"`
}

type Project struct {
	ID              string    `json:"id"`
	UserEmail       string    `json:"user_email"`
	ServiceCategory string    `json:"service_category"`
	Description     string    `json:"description"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	BudgetRange     struct {
		Min float64 `json:"min"`
		Max float64 `json:"max"`
	} `json:"budget_range"`
	Timeline string `json:"timeline"`
	Location struct {
		Address string `json:"address"`
		ZipCode string `json:"zip_code"`
	} `json:"location"`
	Proposals []Proposal `json:"proposals"`
}

type Proposal struct {
	ID         string    `json:"id"`
	ProviderID string    `json:"provider_id"`
	ProjectID  string    `json:"project_id"`
	Price      float64   `json:"price"`
	Timeline   string    `json:"timeline"`
	Message    string    `json:"message"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

type Review struct {
	ID         string    `json:"id"`
	ProviderID string    `json:"provider_id"`
	UserEmail  string    `json:"user_email"`
	ProjectID  string    `json:"project_id"`
	Rating     float64   `json:"rating"`
	Comment    string    `json:"comment"`
	CreatedAt  time.Time `json:"created_at"`
}

type Database struct {
	Categories []ServiceCategory          `json:"categories"`
	Providers  map[string]ServiceProvider `json:"providers"`
	Projects   map[string]Project         `json:"projects"`
	Reviews    map[string]Review          `json:"reviews"`
	mu         sync.RWMutex
}

var db *Database

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Providers: make(map[string]ServiceProvider),
		Projects:  make(map[string]Project),
		Reviews:   make(map[string]Review),
	}

	return json.Unmarshal(data, db)
}

func getServiceCategories(c *fiber.Ctx) error {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return c.JSON(db.Categories)
}

func searchProviders(c *fiber.Ctx) error {
	serviceID := c.Query("service_id")
	zipCode := c.Query("zip_code")

	if serviceID == "" || zipCode == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "service_id and zip_code are required",
		})
	}

	var matchingProviders []ServiceProvider
	db.mu.RLock()
	for _, provider := range db.Providers {
		// Check if provider serves this area and service
		servesArea := false
		for _, area := range provider.ServiceArea {
			if area == zipCode {
				servesArea = true
				break
			}
		}

		providesService := false
		for _, service := range provider.Services {
			if service == serviceID {
				providesService = true
				break
			}
		}

		if servesArea && providesService {
			matchingProviders = append(matchingProviders, provider)
		}
	}
	db.mu.RUnlock()

	return c.JSON(matchingProviders)
}

func getUserProjects(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	var userProjects []Project
	db.mu.RLock()
	for _, project := range db.Projects {
		if project.UserEmail == email {
			userProjects = append(userProjects, project)
		}
	}
	db.mu.RUnlock()

	return c.JSON(userProjects)
}

func createProject(c *fiber.Ctx) error {
	var newProject Project
	if err := c.BodyParser(&newProject); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	newProject.ID = uuid.New().String()
	newProject.CreatedAt = time.Now()
	newProject.Status = "open"

	db.mu.Lock()
	db.Projects[newProject.ID] = newProject
	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(newProject)
}

func getProviderReviews(c *fiber.Ctx) error {
	providerID := c.Query("provider_id")
	if providerID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "provider_id is required",
		})
	}

	var providerReviews []Review
	db.mu.RLock()
	for _, review := range db.Reviews {
		if review.ProviderID == providerID {
			providerReviews = append(providerReviews, review)
		}
	}
	db.mu.RUnlock()

	return c.JSON(providerReviews)
}

func submitReview(c *fiber.Ctx) error {
	var newReview Review
	if err := c.BodyParser(&newReview); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if newReview.Rating < 1 || newReview.Rating > 5 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Rating must be between 1 and 5",
		})
	}

	newReview.ID = uuid.New().String()
	newReview.CreatedAt = time.Now()

	db.mu.Lock()
	db.Reviews[newReview.ID] = newReview

	// Update provider rating
	provider := db.Providers[newReview.ProviderID]
	totalReviews := float64(provider.ReviewCount)
	newRating := ((provider.Rating * totalReviews) + newReview.Rating) / (totalReviews + 1)
	provider.Rating = newRating
	provider.ReviewCount++
	db.Providers[newReview.ProviderID] = provider

	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(newReview)
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

	api.Get("/services", getServiceCategories)
	api.Get("/providers", searchProviders)
	api.Get("/projects", getUserProjects)
	api.Post("/projects", createProject)
	api.Get("/reviews", getProviderReviews)
	api.Post("/reviews", submitReview)
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

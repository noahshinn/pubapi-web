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
	"github.com/google/uuid"
)

// Domain Models
type Address struct {
	Street  string `json:"street"`
	City    string `json:"city"`
	State   string `json:"state"`
	ZipCode string `json:"zip_code"`
}

type User struct {
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Phone     string    `json:"phone"`
	Address   Address   `json:"address"`
	CreatedAt time.Time `json:"created_at"`
}

type ServiceCategory struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Subcategories []string `json:"subcategories"`
}

type Contractor struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Services    []string `json:"services"`
	Rating      float64  `json:"rating"`
	ReviewCount int      `json:"review_count"`
	Verified    bool     `json:"verified"`
	ServiceArea []string `json:"service_area"`
	Address     Address  `json:"address"`
}

type BudgetRange struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

type ProjectStatus string

const (
	ProjectStatusPending    ProjectStatus = "pending"
	ProjectStatusScheduled  ProjectStatus = "scheduled"
	ProjectStatusInProgress ProjectStatus = "in_progress"
	ProjectStatusCompleted  ProjectStatus = "completed"
	ProjectStatusCancelled  ProjectStatus = "cancelled"
)

type Project struct {
	ID              string        `json:"id"`
	UserEmail       string        `json:"user_email"`
	ServiceCategory string        `json:"service_category"`
	Description     string        `json:"description"`
	Status          ProjectStatus `json:"status"`
	BudgetRange     BudgetRange   `json:"budget_range"`
	Timeline        string        `json:"timeline"`
	Address         Address       `json:"address"`
	Contractor      *Contractor   `json:"contractor,omitempty"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
}

type Review struct {
	ID           string    `json:"id"`
	ContractorID string    `json:"contractor_id"`
	ProjectID    string    `json:"project_id"`
	UserEmail    string    `json:"user_email"`
	Rating       float64   `json:"rating"`
	Comment      string    `json:"comment"`
	CreatedAt    time.Time `json:"created_at"`
}

// Database represents our in-memory database
type Database struct {
	Users             map[string]User            `json:"users"`
	ServiceCategories map[string]ServiceCategory `json:"service_categories"`
	Contractors       map[string]Contractor      `json:"contractors"`
	Projects          map[string]Project         `json:"projects"`
	Reviews           map[string]Review          `json:"reviews"`
	mu                sync.RWMutex
}

var db *Database

// Database operations
func (d *Database) GetUser(email string) (User, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	user, exists := d.Users[email]
	if !exists {
		return User{}, errors.New("user not found")
	}
	return user, nil
}

func (d *Database) GetServiceCategories() []ServiceCategory {
	d.mu.RLock()
	defer d.mu.RUnlock()

	categories := make([]ServiceCategory, 0, len(d.ServiceCategories))
	for _, category := range d.ServiceCategories {
		categories = append(categories, category)
	}
	return categories
}

func (d *Database) FindContractors(serviceID, zipCode string) []Contractor {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var matches []Contractor
	for _, contractor := range d.Contractors {
		// Check if contractor provides the service
		providesService := false
		for _, service := range contractor.Services {
			if service == serviceID {
				providesService = true
				break
			}
		}

		// Check if contractor serves the area
		servesArea := false
		for _, area := range contractor.ServiceArea {
			if area == zipCode {
				servesArea = true
				break
			}
		}

		if providesService && servesArea {
			matches = append(matches, contractor)
		}
	}
	return matches
}

func (d *Database) GetUserProjects(email string) []Project {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var projects []Project
	for _, project := range d.Projects {
		if project.UserEmail == email {
			projects = append(projects, project)
		}
	}
	return projects
}

func (d *Database) CreateProject(project Project) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Projects[project.ID] = project
	return nil
}

func (d *Database) CreateReview(review Review) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Reviews[review.ID] = review

	// Update contractor rating
	contractor := d.Contractors[review.ContractorID]
	totalRating := contractor.Rating * float64(contractor.ReviewCount)
	contractor.ReviewCount++
	contractor.Rating = (totalRating + review.Rating) / float64(contractor.ReviewCount)
	d.Contractors[review.ContractorID] = contractor

	return nil
}

// HTTP Handlers
func getServiceCategories(c *fiber.Ctx) error {
	categories := db.GetServiceCategories()
	return c.JSON(categories)
}

func searchContractors(c *fiber.Ctx) error {
	serviceID := c.Query("service_id")
	zipCode := c.Query("zip_code")

	if serviceID == "" || zipCode == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "service_id and zip_code are required",
		})
	}

	contractors := db.FindContractors(serviceID, zipCode)
	return c.JSON(contractors)
}

func getUserProjects(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	projects := db.GetUserProjects(email)
	return c.JSON(projects)
}

type CreateProjectRequest struct {
	ServiceCategoryID string      `json:"service_category_id"`
	Description       string      `json:"description"`
	UserEmail         string      `json:"user_email"`
	BudgetRange       BudgetRange `json:"budget_range"`
	Timeline          string      `json:"timeline"`
	Address           Address     `json:"address"`
}

func createProject(c *fiber.Ctx) error {
	var req CreateProjectRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user exists
	if _, err := db.GetUser(req.UserEmail); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	project := Project{
		ID:              uuid.New().String(),
		UserEmail:       req.UserEmail,
		ServiceCategory: req.ServiceCategoryID,
		Description:     req.Description,
		Status:          ProjectStatusPending,
		BudgetRange:     req.BudgetRange,
		Timeline:        req.Timeline,
		Address:         req.Address,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if err := db.CreateProject(project); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create project",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(project)
}

type CreateReviewRequest struct {
	ContractorID string  `json:"contractor_id"`
	ProjectID    string  `json:"project_id"`
	Rating       float64 `json:"rating"`
	Comment      string  `json:"comment"`
	UserEmail    string  `json:"user_email"`
}

func createReview(c *fiber.Ctx) error {
	var req CreateReviewRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.Rating < 1 || req.Rating > 5 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Rating must be between 1 and 5",
		})
	}

	review := Review{
		ID:           uuid.New().String(),
		ContractorID: req.ContractorID,
		ProjectID:    req.ProjectID,
		UserEmail:    req.UserEmail,
		Rating:       req.Rating,
		Comment:      req.Comment,
		CreatedAt:    time.Now(),
	}

	if err := db.CreateReview(review); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create review",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(review)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:             make(map[string]User),
		ServiceCategories: make(map[string]ServiceCategory),
		Contractors:       make(map[string]Contractor),
		Projects:          make(map[string]Project),
		Reviews:           make(map[string]Review),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	api.Get("/services", getServiceCategories)
	api.Get("/contractors", searchContractors)
	api.Get("/projects", getUserProjects)
	api.Post("/projects", createProject)
	api.Post("/reviews", createReview)
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

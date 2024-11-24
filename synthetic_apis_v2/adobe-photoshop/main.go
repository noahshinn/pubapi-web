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

// Domain Models
type Dimensions struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type Project struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	UserEmail    string     `json:"user_email"`
	Dimensions   Dimensions `json:"dimensions"`
	CreatedAt    time.Time  `json:"created_at"`
	ModifiedAt   time.Time  `json:"modified_at"`
	ThumbnailURL string     `json:"thumbnail_url"`
	Size         int64      `json:"size"`
	Layers       []Layer    `json:"layers"`
}

type Layer struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Type      string  `json:"type"`
	Visible   bool    `json:"visible"`
	Opacity   float64 `json:"opacity"`
	BlendMode string  `json:"blend_mode"`
	Locked    bool    `json:"locked"`
	Content   string  `json:"content,omitempty"`
}

type StorageUsage struct {
	TotalStorage     int64 `json:"total_storage"`
	UsedStorage      int64 `json:"used_storage"`
	AvailableStorage int64 `json:"available_storage"`
	StorageLimit     int64 `json:"storage_limit"`
}

type User struct {
	Email        string       `json:"email"`
	Name         string       `json:"name"`
	Subscription string       `json:"subscription"`
	StorageUsage StorageUsage `json:"storage_usage"`
}

// Database represents our in-memory database
type Database struct {
	Users    map[string]User    `json:"users"`
	Projects map[string]Project `json:"projects"`
	mu       sync.RWMutex
}

var db *Database

// Database operations
func (d *Database) GetUser(email string) (User, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	user, exists := d.Users[email]
	if !exists {
		return User{}, fiber.NewError(fiber.StatusNotFound, "User not found")
	}
	return user, nil
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

func (d *Database) GetProject(id string) (Project, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	project, exists := d.Projects[id]
	if !exists {
		return Project{}, fiber.NewError(fiber.StatusNotFound, "Project not found")
	}
	return project, nil
}

func (d *Database) AddLayer(projectId string, layer Layer) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	project, exists := d.Projects[projectId]
	if !exists {
		return fiber.NewError(fiber.StatusNotFound, "Project not found")
	}

	project.Layers = append(project.Layers, layer)
	project.ModifiedAt = time.Now()
	d.Projects[projectId] = project

	return nil
}

// HTTP Handlers
func getUserProjects(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Email is required")
	}

	if _, err := db.GetUser(email); err != nil {
		return err
	}

	projects := db.GetUserProjects(email)
	return c.JSON(projects)
}

func createProject(c *fiber.Ctx) error {
	var req struct {
		Name       string     `json:"name"`
		UserEmail  string     `json:"user_email"`
		Dimensions Dimensions `json:"dimensions"`
	}

	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if _, err := db.GetUser(req.UserEmail); err != nil {
		return err
	}

	project := Project{
		ID:         uuid.New().String(),
		Name:       req.Name,
		UserEmail:  req.UserEmail,
		Dimensions: req.Dimensions,
		CreatedAt:  time.Now(),
		ModifiedAt: time.Now(),
		Layers:     make([]Layer, 0),
	}

	if err := db.CreateProject(project); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create project")
	}

	return c.Status(fiber.StatusCreated).JSON(project)
}

func getProjectLayers(c *fiber.Ctx) error {
	projectId := c.Params("projectId")

	project, err := db.GetProject(projectId)
	if err != nil {
		return err
	}

	return c.JSON(project.Layers)
}

func addProjectLayer(c *fiber.Ctx) error {
	projectId := c.Params("projectId")

	var layer Layer
	if err := c.BodyParser(&layer); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	layer.ID = uuid.New().String()
	layer.Visible = true
	layer.Opacity = 1.0
	layer.BlendMode = "normal"

	if err := db.AddLayer(projectId, layer); err != nil {
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(layer)
}

func getStorageUsage(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Email is required")
	}

	user, err := db.GetUser(email)
	if err != nil {
		return err
	}

	return c.JSON(user.StorageUsage)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:    make(map[string]User),
		Projects: make(map[string]Project),
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

	// Project routes
	api.Get("/projects", getUserProjects)
	api.Post("/projects", createProject)

	// Layer routes
	api.Get("/projects/:projectId/layers", getProjectLayers)
	api.Post("/projects/:projectId/layers", addProjectLayer)

	// Storage routes
	api.Get("/storage/usage", getStorageUsage)
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

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
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

type ColorMode string

const (
	ColorModeRGB  ColorMode = "RGB"
	ColorModeCMYK ColorMode = "CMYK"
)

type Project struct {
	ID           string     `json:"id"`
	UserEmail    string     `json:"user_email"`
	Name         string     `json:"name"`
	Dimensions   Dimensions `json:"dimensions"`
	ColorMode    ColorMode  `json:"color_mode"`
	Layers       []Layer    `json:"layers"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	ThumbnailURL string     `json:"thumbnail_url"`
}

type LayerType string

const (
	LayerTypeRaster LayerType = "raster"
	LayerTypeText   LayerType = "text"
	LayerTypeShape  LayerType = "shape"
	LayerTypeGroup  LayerType = "group"
)

type BlendMode string

const (
	BlendModeNormal   BlendMode = "normal"
	BlendModeMultiply BlendMode = "multiply"
	BlendModeScreen   BlendMode = "screen"
	BlendModeOverlay  BlendMode = "overlay"
)

type Layer struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Type      LayerType `json:"type"`
	Visible   bool      `json:"visible"`
	Opacity   float64   `json:"opacity"`
	BlendMode BlendMode `json:"blend_mode"`
	Content   string    `json:"content"`
	Position  struct {
		X int `json:"x"`
		Y int `json:"y"`
	} `json:"position"`
}

type User struct {
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	Subscription string    `json:"subscription"`
	StorageUsed  int64     `json:"storage_used"`
	StorageLimit int64     `json:"storage_limit"`
	LastLogin    time.Time `json:"last_login"`
	CreatedAt    time.Time `json:"created_at"`
}

// Database represents our in-memory database
type Database struct {
	Users    map[string]User    `json:"users"`
	Projects map[string]Project `json:"projects"`
	mu       sync.RWMutex
}

// Custom errors
var (
	ErrUserNotFound    = errors.New("user not found")
	ErrProjectNotFound = errors.New("project not found")
	ErrLayerNotFound   = errors.New("layer not found")
	ErrStorageFull     = errors.New("storage limit exceeded")
)

// Global database instance
var db *Database

// Database operations
func (d *Database) GetUser(email string) (User, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	user, exists := d.Users[email]
	if !exists {
		return User{}, ErrUserNotFound
	}
	return user, nil
}

func (d *Database) GetUserProjects(email string) ([]Project, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var projects []Project
	for _, project := range d.Projects {
		if project.UserEmail == email {
			projects = append(projects, project)
		}
	}
	return projects, nil
}

func (d *Database) CreateProject(project Project) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Check user storage limit
	user, exists := d.Users[project.UserEmail]
	if !exists {
		return ErrUserNotFound
	}

	// Assuming each project takes 100MB
	if user.StorageUsed+100*1024*1024 > user.StorageLimit {
		return ErrStorageFull
	}

	d.Projects[project.ID] = project
	user.StorageUsed += 100 * 1024 * 1024
	d.Users[project.UserEmail] = user

	return nil
}

func (d *Database) GetProject(id string) (Project, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	project, exists := d.Projects[id]
	if !exists {
		return Project{}, ErrProjectNotFound
	}
	return project, nil
}

func (d *Database) UpdateProject(project Project) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.Projects[project.ID]; !exists {
		return ErrProjectNotFound
	}

	project.UpdatedAt = time.Now()
	d.Projects[project.ID] = project
	return nil
}

// HTTP Handlers
func getUserProjects(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	projects, err := db.GetUserProjects(email)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(projects)
}

type CreateProjectRequest struct {
	Name      string    `json:"name"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	ColorMode ColorMode `json:"color_mode"`
	UserEmail string    `json:"user_email"`
}

func createProject(c *fiber.Ctx) error {
	var req CreateProjectRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user
	_, err := db.GetUser(req.UserEmail)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	project := Project{
		ID:        uuid.New().String(),
		UserEmail: req.UserEmail,
		Name:      req.Name,
		Dimensions: Dimensions{
			Width:  req.Width,
			Height: req.Height,
		},
		ColorMode: req.ColorMode,
		Layers:    []Layer{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.CreateProject(project); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(project)
}

func getProjectLayers(c *fiber.Ctx) error {
	projectId := c.Params("projectId")
	if projectId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Project ID is required",
		})
	}

	project, err := db.GetProject(projectId)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(project.Layers)
}

type NewLayerRequest struct {
	Name     string    `json:"name"`
	Type     LayerType `json:"type"`
	Content  string    `json:"content"`
	Position struct {
		X int `json:"x"`
		Y int `json:"y"`
	} `json:"position"`
}

func addLayer(c *fiber.Ctx) error {
	projectId := c.Params("projectId")
	if projectId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Project ID is required",
		})
	}

	var req NewLayerRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	project, err := db.GetProject(projectId)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	layer := Layer{
		ID:        uuid.New().String(),
		Name:      req.Name,
		Type:      req.Type,
		Visible:   true,
		Opacity:   1.0,
		BlendMode: BlendModeNormal,
		Content:   req.Content,
		Position:  req.Position,
	}

	project.Layers = append(project.Layers, layer)
	if err := db.UpdateProject(project); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(layer)
}

type ExportRequest struct {
	Format        string `json:"format"`
	Quality       int    `json:"quality"`
	IncludeLayers bool   `json:"include_layers"`
}

func exportProject(c *fiber.Ctx) error {
	projectId := c.Params("projectId")
	if projectId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Project ID is required",
		})
	}

	var req ExportRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	project, err := db.GetProject(projectId)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Simulate export process
	exportURL := fmt.Sprintf("https://storage.adobe.com/exports/%s.%s", project.ID, req.Format)

	return c.JSON(fiber.Map{
		"export_url": exportURL,
		"format":     req.Format,
		"quality":    req.Quality,
	})
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
	api := app.Group("/api/v1")

	// Project routes
	api.Get("/projects", getUserProjects)
	api.Post("/projects", createProject)
	api.Get("/projects/:projectId", func(c *fiber.Ctx) error {
		projectId := c.Params("projectId")
		project, err := db.GetProject(projectId)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.JSON(project)
	})

	// Layer routes
	api.Get("/projects/:projectId/layers", getProjectLayers)
	api.Post("/projects/:projectId/layers", addLayer)

	// Export route
	api.Post("/projects/:projectId/export", exportProject)

	// User routes
	api.Get("/users/:email", func(c *fiber.Ctx) error {
		email := c.Params("email")
		user, err := db.GetUser(email)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.JSON(user)
	})
}

func main() {
	// Command line flags
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

	// Middleware
	app.Use(logger.New())
	app.Use(recover.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE",
		AllowHeaders: "Origin, Content-Type, Accept",
	}))

	// Setup routes
	setupRoutes(app)

	// Start server
	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

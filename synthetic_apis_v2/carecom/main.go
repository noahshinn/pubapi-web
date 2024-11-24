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

type Caregiver struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Email           string    `json:"email"`
	Phone           string    `json:"phone"`
	Address         Address   `json:"address"`
	CareTypes       []string  `json:"care_types"`
	HourlyRate      float64   `json:"hourly_rate"`
	YearsExperience int       `json:"years_experience"`
	Bio             string    `json:"bio"`
	Availability    string    `json:"availability"`
	BackgroundCheck bool      `json:"background_check"`
	Reviews         []Review  `json:"reviews"`
	CreatedAt       time.Time `json:"created_at"`
}

type Review struct {
	ID        string    `json:"id"`
	Rating    int       `json:"rating"`
	Comment   string    `json:"comment"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"created_at"`
}

type Job struct {
	ID          string    `json:"id"`
	UserEmail   string    `json:"user_email"`
	CareType    string    `json:"care_type"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Schedule    string    `json:"schedule"`
	HourlyRate  float64   `json:"hourly_rate"`
	Location    Address   `json:"location"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type JobApplication struct {
	ID          string    `json:"id"`
	JobID       string    `json:"job_id"`
	CaregiverID string    `json:"caregiver_id"`
	CoverLetter string    `json:"cover_letter"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

// Database represents our in-memory database
type Database struct {
	Users        map[string]User           `json:"users"`
	Caregivers   map[string]Caregiver      `json:"caregivers"`
	Jobs         map[string]Job            `json:"jobs"`
	Applications map[string]JobApplication `json:"applications"`
	mu           sync.RWMutex
}

var db *Database

// Database operations
func (d *Database) GetUser(email string) (User, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	user, exists := d.Users[email]
	return user, exists
}

func (d *Database) GetCaregiver(id string) (Caregiver, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	caregiver, exists := d.Caregivers[id]
	return caregiver, exists
}

func (d *Database) GetJob(id string) (Job, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	job, exists := d.Jobs[id]
	return job, exists
}

func (d *Database) CreateJob(job Job) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Jobs[job.ID] = job
	return nil
}

func (d *Database) CreateApplication(app JobApplication) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Applications[app.ID] = app
	return nil
}

// HTTP Handlers
func searchCaregivers(c *fiber.Ctx) error {
	careType := c.Query("care_type")
	zipCode := c.Query("zip_code")

	if careType == "" || zipCode == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "care_type and zip_code are required",
		})
	}

	var matchingCaregivers []Caregiver
	db.mu.RLock()
	for _, caregiver := range db.Caregivers {
		// Check if caregiver provides the requested care type
		for _, ct := range caregiver.CareTypes {
			if ct == careType && caregiver.Address.ZipCode == zipCode {
				matchingCaregivers = append(matchingCaregivers, caregiver)
				break
			}
		}
	}
	db.mu.RUnlock()

	return c.JSON(matchingCaregivers)
}

func getUserJobs(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	var userJobs []Job
	db.mu.RLock()
	for _, job := range db.Jobs {
		if job.UserEmail == email {
			userJobs = append(userJobs, job)
		}
	}
	db.mu.RUnlock()

	return c.JSON(userJobs)
}

type CreateJobRequest struct {
	UserEmail   string  `json:"user_email"`
	CareType    string  `json:"care_type"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Schedule    string  `json:"schedule"`
	HourlyRate  float64 `json:"hourly_rate"`
	Location    Address `json:"location"`
}

func createJob(c *fiber.Ctx) error {
	var req CreateJobRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate user exists
	if _, exists := db.GetUser(req.UserEmail); !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	job := Job{
		ID:          uuid.New().String(),
		UserEmail:   req.UserEmail,
		CareType:    req.CareType,
		Title:       req.Title,
		Description: req.Description,
		Schedule:    req.Schedule,
		HourlyRate:  req.HourlyRate,
		Location:    req.Location,
		Status:      "open",
		CreatedAt:   time.Now(),
	}

	if err := db.CreateJob(job); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create job",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(job)
}

type CreateApplicationRequest struct {
	JobID       string `json:"job_id"`
	CaregiverID string `json:"caregiver_id"`
	CoverLetter string `json:"cover_letter"`
}

func createApplication(c *fiber.Ctx) error {
	var req CreateApplicationRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate job exists
	if _, exists := db.GetJob(req.JobID); !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Job not found",
		})
	}

	// Validate caregiver exists
	if _, exists := db.GetCaregiver(req.CaregiverID); !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Caregiver not found",
		})
	}

	application := JobApplication{
		ID:          uuid.New().String(),
		JobID:       req.JobID,
		CaregiverID: req.CaregiverID,
		CoverLetter: req.CoverLetter,
		Status:      "pending",
		CreatedAt:   time.Now(),
	}

	if err := db.CreateApplication(application); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create application",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(application)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:        make(map[string]User),
		Caregivers:   make(map[string]Caregiver),
		Jobs:         make(map[string]Job),
		Applications: make(map[string]JobApplication),
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

	// Caregiver routes
	api.Get("/caregivers", searchCaregivers)
	api.Get("/caregivers/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		caregiver, exists := db.GetCaregiver(id)
		if !exists {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Caregiver not found",
			})
		}
		return c.JSON(caregiver)
	})

	// Job routes
	api.Get("/jobs", getUserJobs)
	api.Post("/jobs", createJob)
	api.Get("/jobs/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		job, exists := db.GetJob(id)
		if !exists {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Job not found",
			})
		}
		return c.JSON(job)
	})

	// Application routes
	api.Post("/applications", createApplication)
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

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
type ServiceType string

const (
	ServiceTypeChildcare    ServiceType = "childcare"
	ServiceTypeSeniorcare   ServiceType = "seniorcare"
	ServiceTypePetcare      ServiceType = "petcare"
	ServiceTypeHousekeeping ServiceType = "housekeeping"
)

type User struct {
	Email           string    `json:"email"`
	Name            string    `json:"name"`
	Phone           string    `json:"phone"`
	Address         string    `json:"address"`
	ZipCode         string    `json:"zip_code"`
	JoinDate        time.Time `json:"join_date"`
	VerifiedID      bool      `json:"verified_id"`
	BackgroundCheck bool      `json:"background_check"`
}

type Caregiver struct {
	ID              string        `json:"id"`
	UserEmail       string        `json:"user_email"`
	ServiceTypes    []ServiceType `json:"service_types"`
	HourlyRate      float64       `json:"hourly_rate"`
	YearsExperience int           `json:"years_experience"`
	Bio             string        `json:"bio"`
	Availability    []string      `json:"availability"`
	Rating          float64       `json:"rating"`
	ReviewsCount    int           `json:"reviews_count"`
	Certifications  []string      `json:"certifications"`
}

type JobStatus string

const (
	JobStatusOpen       JobStatus = "open"
	JobStatusInProgress JobStatus = "in_progress"
	JobStatusCompleted  JobStatus = "completed"
	JobStatusCancelled  JobStatus = "cancelled"
)

type JobPosting struct {
	ID           string      `json:"id"`
	UserEmail    string      `json:"user_email"`
	ServiceType  ServiceType `json:"service_type"`
	Title        string      `json:"title"`
	Description  string      `json:"description"`
	Requirements string      `json:"requirements"`
	Schedule     string      `json:"schedule"`
	HourlyRate   float64     `json:"hourly_rate"`
	Location     string      `json:"location"`
	ZipCode      string      `json:"zip_code"`
	Status       JobStatus   `json:"status"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
}

type ApplicationStatus string

const (
	ApplicationStatusPending   ApplicationStatus = "pending"
	ApplicationStatusAccepted  ApplicationStatus = "accepted"
	ApplicationStatusRejected  ApplicationStatus = "rejected"
	ApplicationStatusWithdrawn ApplicationStatus = "withdrawn"
)

type Application struct {
	ID          string            `json:"id"`
	JobID       string            `json:"job_id"`
	CaregiverID string            `json:"caregiver_id"`
	CoverLetter string            `json:"cover_letter"`
	Status      ApplicationStatus `json:"status"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// Database represents our in-memory database
type Database struct {
	Users        map[string]User        `json:"users"`
	Caregivers   map[string]Caregiver   `json:"caregivers"`
	JobPostings  map[string]JobPosting  `json:"job_postings"`
	Applications map[string]Application `json:"applications"`
	mu           sync.RWMutex
}

// Global database instance
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

func (d *Database) GetCaregiver(id string) (Caregiver, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	caregiver, exists := d.Caregivers[id]
	if !exists {
		return Caregiver{}, errors.New("caregiver not found")
	}
	return caregiver, nil
}

func (d *Database) SearchCaregivers(serviceType ServiceType, zipCode string, radius int) []Caregiver {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var results []Caregiver
	for _, caregiver := range d.Caregivers {
		// Check if caregiver provides the requested service
		for _, st := range caregiver.ServiceTypes {
			if st == serviceType {
				// In a real implementation, we would check the distance between zip codes
				results = append(results, caregiver)
				break
			}
		}
	}
	return results
}

func (d *Database) CreateJobPosting(job JobPosting) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.JobPostings[job.ID] = job
	return nil
}

func (d *Database) GetJobPosting(id string) (JobPosting, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	job, exists := d.JobPostings[id]
	if !exists {
		return JobPosting{}, errors.New("job posting not found")
	}
	return job, nil
}

func (d *Database) CreateApplication(app Application) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Applications[app.ID] = app
	return nil
}

// HTTP Handlers
func searchCaregivers(c *fiber.Ctx) error {
	serviceType := ServiceType(c.Query("service_type"))
	zipCode := c.Query("zip_code")
	radius := c.QueryInt("radius", 10)

	if serviceType == "" || zipCode == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "service_type and zip_code are required",
		})
	}

	caregivers := db.SearchCaregivers(serviceType, zipCode, radius)
	return c.JSON(caregivers)
}

func getUserJobs(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	var userJobs []JobPosting
	db.mu.RLock()
	for _, job := range db.JobPostings {
		if job.UserEmail == email {
			userJobs = append(userJobs, job)
		}
	}
	db.mu.RUnlock()

	return c.JSON(userJobs)
}

type CreateJobRequest struct {
	ServiceType  ServiceType `json:"service_type"`
	Title        string      `json:"title"`
	Description  string      `json:"description"`
	Requirements string      `json:"requirements"`
	Schedule     string      `json:"schedule"`
	HourlyRate   float64     `json:"hourly_rate"`
	Location     string      `json:"location"`
	ZipCode      string      `json:"zip_code"`
	UserEmail    string      `json:"user_email"`
}

func createJob(c *fiber.Ctx) error {
	var req CreateJobRequest
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

	job := JobPosting{
		ID:           uuid.New().String(),
		UserEmail:    req.UserEmail,
		ServiceType:  req.ServiceType,
		Title:        req.Title,
		Description:  req.Description,
		Requirements: req.Requirements,
		Schedule:     req.Schedule,
		HourlyRate:   req.HourlyRate,
		Location:     req.Location,
		ZipCode:      req.ZipCode,
		Status:       JobStatusOpen,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := db.CreateJobPosting(job); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create job posting",
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
	job, err := db.GetJobPosting(req.JobID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Job posting not found",
		})
	}

	// Validate caregiver exists
	caregiver, err := db.GetCaregiver(req.CaregiverID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Caregiver not found",
		})
	}

	// Validate caregiver provides the required service
	validService := false
	for _, st := range caregiver.ServiceTypes {
		if st == job.ServiceType {
			validService = true
			break
		}
	}
	if !validService {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Caregiver does not provide the required service type",
		})
	}

	application := Application{
		ID:          uuid.New().String(),
		JobID:       req.JobID,
		CaregiverID: req.CaregiverID,
		CoverLetter: req.CoverLetter,
		Status:      ApplicationStatusPending,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := db.CreateApplication(application); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create application",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(application)
}

func getApplications(c *fiber.Ctx) error {
	jobID := c.Query("job_id")
	if jobID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "job_id parameter is required",
		})
	}

	var jobApplications []Application
	db.mu.RLock()
	for _, app := range db.Applications {
		if app.JobID == jobID {
			jobApplications = append(jobApplications, app)
		}
	}
	db.mu.RUnlock()

	return c.JSON(jobApplications)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:        make(map[string]User),
		Caregivers:   make(map[string]Caregiver),
		JobPostings:  make(map[string]JobPosting),
		Applications: make(map[string]Application),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Caregiver routes
	api.Get("/caregivers", searchCaregivers)
	api.Get("/caregivers/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		caregiver, err := db.GetCaregiver(id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.JSON(caregiver)
	})

	// Job posting routes
	api.Get("/jobs", getUserJobs)
	api.Post("/jobs", createJob)
	api.Get("/jobs/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		job, err := db.GetJobPosting(id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.JSON(job)
	})

	// Application routes
	api.Get("/applications", getApplications)
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
	app.Use(recover.New())
	app.Use(cors.New())

	setupRoutes(app)

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

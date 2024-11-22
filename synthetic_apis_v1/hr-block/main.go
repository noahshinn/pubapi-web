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
type FilingStatus string

const (
	FilingStatusSingle          FilingStatus = "single"
	FilingStatusMarried         FilingStatus = "married_joint"
	FilingStatusMarriedSeparate FilingStatus = "married_separate"
	FilingStatusHeadOfHousehold FilingStatus = "head_of_household"
)

type TaxReturnStatus string

const (
	TaxReturnStatusDraft      TaxReturnStatus = "draft"
	TaxReturnStatusInProgress TaxReturnStatus = "in_progress"
	TaxReturnStatusReview     TaxReturnStatus = "under_review"
	TaxReturnStatusComplete   TaxReturnStatus = "complete"
	TaxReturnStatusFiled      TaxReturnStatus = "filed"
)

type User struct {
	Email        string       `json:"email"`
	Name         string       `json:"name"`
	SSN          string       `json:"ssn"`
	DateOfBirth  string       `json:"date_of_birth"`
	FilingStatus FilingStatus `json:"filing_status"`
	Address      Address      `json:"address"`
	Phone        string       `json:"phone"`
	Dependents   []Dependent  `json:"dependents"`
}

type Address struct {
	Street  string `json:"street"`
	City    string `json:"city"`
	State   string `json:"state"`
	ZipCode string `json:"zip_code"`
}

type Dependent struct {
	Name         string `json:"name"`
	SSN          string `json:"ssn"`
	Relationship string `json:"relationship"`
	DateOfBirth  string `json:"date_of_birth"`
}

type TaxDocument struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	TaxYear    int       `json:"tax_year"`
	FileName   string    `json:"file_name"`
	UserEmail  string    `json:"user_email"`
	UploadedAt time.Time `json:"uploaded_at"`
}

type TaxProfessional struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Expertise string `json:"expertise"`
	Years     int    `json:"years_experience"`
}

type Appointment struct {
	ID              string          `json:"id"`
	UserEmail       string          `json:"user_email"`
	TaxProfessional TaxProfessional `json:"tax_professional"`
	DateTime        time.Time       `json:"datetime"`
	Type            string          `json:"type"`
	Status          string          `json:"status"`
	Notes           string          `json:"notes"`
}

type TaxReturn struct {
	ID              string          `json:"id"`
	UserEmail       string          `json:"user_email"`
	TaxYear         int             `json:"tax_year"`
	Status          TaxReturnStatus `json:"status"`
	FilingType      string          `json:"filing_type"`
	TotalIncome     float64         `json:"total_income"`
	TotalDeductions float64         `json:"total_deductions"`
	TotalTax        float64         `json:"total_tax"`
	RefundAmount    float64         `json:"refund_amount"`
	Documents       []TaxDocument   `json:"documents"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// Database represents our in-memory database
type Database struct {
	Users            map[string]User            `json:"users"`
	TaxReturns       map[string]TaxReturn       `json:"tax_returns"`
	TaxDocuments     map[string]TaxDocument     `json:"tax_documents"`
	Appointments     map[string]Appointment     `json:"appointments"`
	TaxProfessionals map[string]TaxProfessional `json:"tax_professionals"`
	mu               sync.RWMutex
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

func (d *Database) GetTaxReturns(email string) []TaxReturn {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var returns []TaxReturn
	for _, tr := range d.TaxReturns {
		if tr.UserEmail == email {
			returns = append(returns, tr)
		}
	}
	return returns
}

func (d *Database) GetTaxDocuments(email string) []TaxDocument {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var docs []TaxDocument
	for _, doc := range d.TaxDocuments {
		if doc.UserEmail == email {
			docs = append(docs, doc)
		}
	}
	return docs
}

func (d *Database) CreateTaxReturn(tr TaxReturn) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.TaxReturns[tr.ID] = tr
	return nil
}

func (d *Database) CreateAppointment(apt Appointment) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Appointments[apt.ID] = apt
	return nil
}

// HTTP Handlers
func getTaxReturns(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	returns := db.GetTaxReturns(email)
	return c.JSON(returns)
}

func createTaxReturn(c *fiber.Ctx) error {
	var req struct {
		TaxYear    int    `json:"tax_year"`
		FilingType string `json:"filing_type"`
		UserEmail  string `json:"user_email"`
	}

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

	taxReturn := TaxReturn{
		ID:         uuid.New().String(),
		UserEmail:  req.UserEmail,
		TaxYear:    req.TaxYear,
		FilingType: req.FilingType,
		Status:     TaxReturnStatusDraft,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := db.CreateTaxReturn(taxReturn); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create tax return",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(taxReturn)
}

func getTaxDocuments(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	docs := db.GetTaxDocuments(email)
	return c.JSON(docs)
}

func uploadTaxDocument(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "No file uploaded",
		})
	}

	email := c.FormValue("email")
	docType := c.FormValue("type")

	doc := TaxDocument{
		ID:         uuid.New().String(),
		Type:       docType,
		FileName:   file.Filename,
		UserEmail:  email,
		UploadedAt: time.Now(),
	}

	// In a real implementation, save the file to storage
	// For this demo, we'll just save the metadata
	db.mu.Lock()
	db.TaxDocuments[doc.ID] = doc
	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(doc)
}

func getAppointments(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	db.mu.RLock()
	var appointments []Appointment
	for _, apt := range db.Appointments {
		if apt.UserEmail == email {
			appointments = append(appointments, apt)
		}
	}
	db.mu.RUnlock()

	return c.JSON(appointments)
}

func scheduleAppointment(c *fiber.Ctx) error {
	var req struct {
		TaxProfessionalID string    `json:"tax_professional_id"`
		DateTime          time.Time `json:"datetime"`
		Type              string    `json:"type"`
		UserEmail         string    `json:"user_email"`
	}

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

	// Validate tax professional exists
	db.mu.RLock()
	professional, exists := db.TaxProfessionals[req.TaxProfessionalID]
	db.mu.RUnlock()
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Tax professional not found",
		})
	}

	appointment := Appointment{
		ID:              uuid.New().String(),
		UserEmail:       req.UserEmail,
		TaxProfessional: professional,
		DateTime:        req.DateTime,
		Type:            req.Type,
		Status:          "scheduled",
	}

	if err := db.CreateAppointment(appointment); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create appointment",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(appointment)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:            make(map[string]User),
		TaxReturns:       make(map[string]TaxReturn),
		TaxDocuments:     make(map[string]TaxDocument),
		Appointments:     make(map[string]Appointment),
		TaxProfessionals: make(map[string]TaxProfessional),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Tax returns routes
	api.Get("/tax-returns", getTaxReturns)
	api.Post("/tax-returns", createTaxReturn)
	api.Get("/tax-returns/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		db.mu.RLock()
		tr, exists := db.TaxReturns[id]
		db.mu.RUnlock()
		if !exists {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Tax return not found",
			})
		}
		return c.JSON(tr)
	})

	// Tax documents routes
	api.Get("/documents", getTaxDocuments)
	api.Post("/documents", uploadTaxDocument)

	// Appointments routes
	api.Get("/appointments", getAppointments)
	api.Post("/appointments", scheduleAppointment)
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

package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
)

// Models
type Industry struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type Company struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type ViewerBreakdown struct {
	Industries []Industry `json:"industries"`
	Companies  []Company  `json:"companies"`
}

type ProfileInsights struct {
	ProfileViews      int             `json:"profile_views"`
	SearchAppearances int             `json:"search_appearances"`
	PostImpressions   int             `json:"post_impressions"`
	ViewerBreakdown   ViewerBreakdown `json:"viewer_breakdown"`
}

type Job struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Company     string    `json:"company"`
	Location    string    `json:"location"`
	SalaryRange string    `json:"salary_range"`
	Description string    `json:"description"`
	MatchScore  int       `json:"match_score"`
	PostedAt    time.Time `json:"posted_at"`
}

type Course struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Author         string   `json:"author"`
	Duration       string   `json:"duration"`
	Level          string   `json:"level"`
	Description    string   `json:"description"`
	CompletionRate int      `json:"completion_rate"`
	SkillsCovered  []string `json:"skills_covered"`
}

type Lead struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Title             string `json:"title"`
	Company           string `json:"company"`
	Industry          string `json:"industry"`
	ConnectionDegree  int    `json:"connection_degree"`
	MutualConnections int    `json:"mutual_connections"`
	Notes             string `json:"notes"`
}

type User struct {
	Email           string    `json:"email"`
	Name            string    `json:"name"`
	Title           string    `json:"title"`
	Company         string    `json:"company"`
	Location        string    `json:"location"`
	Industry        string    `json:"industry"`
	About           string    `json:"about"`
	Skills          []string  `json:"skills"`
	Experience      []string  `json:"experience"`
	Education       []string  `json:"education"`
	PremiumTier     string    `json:"premium_tier"`
	SubscribedSince time.Time `json:"subscribed_since"`
}

// Database struct
type Database struct {
	Users           map[string]User            `json:"users"`
	ProfileInsights map[string]ProfileInsights `json:"profile_insights"`
	Jobs            []Job                      `json:"jobs"`
	Courses         []Course                   `json:"courses"`
	Leads           []Lead                     `json:"leads"`
}

var db Database

// Handlers
func getProfileInsights(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email is required",
		})
	}

	timeRange := c.Query("timeRange", "30d")
	insights, exists := db.ProfileInsights[email]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// Adjust insights based on time range
	switch timeRange {
	case "7d":
		insights.ProfileViews /= 4
		insights.SearchAppearances /= 4
		insights.PostImpressions /= 4
	case "90d":
		insights.ProfileViews *= 3
		insights.SearchAppearances *= 3
		insights.PostImpressions *= 3
	}

	return c.JSON(insights)
}

func getJobRecommendations(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email is required",
		})
	}

	user, exists := db.Users[email]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// Filter jobs based on user's skills and experience
	var recommendations []Job
	for _, job := range db.Jobs {
		// Simple matching algorithm
		if contains(user.Skills, job.Title) || contains(user.Experience, job.Company) {
			recommendations = append(recommendations, job)
		}
	}

	return c.JSON(recommendations)
}

func getLearningCourses(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email is required",
		})
	}

	user, exists := db.Users[email]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// Filter courses based on user's skills and industry
	var recommendations []Course
	for _, course := range db.Courses {
		if containsAny(user.Skills, course.SkillsCovered) {
			recommendations = append(recommendations, course)
		}
	}

	return c.JSON(recommendations)
}

func getNetworkLeads(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email is required",
		})
	}

	user, exists := db.Users[email]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// Filter leads based on user's industry and company
	var relevantLeads []Lead
	for _, lead := range db.Leads {
		if lead.Industry == user.Industry || lead.Company == user.Company {
			relevantLeads = append(relevantLeads, lead)
		}
	}

	return c.JSON(relevantLeads)
}

// Helper functions
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func containsAny(slice1, slice2 []string) bool {
	for _, s1 := range slice1 {
		for _, s2 := range slice2 {
			if s1 == s2 {
				return true
			}
		}
	}
	return false
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Profile routes
	api.Get("/profile/insights", getProfileInsights)

	// Jobs routes
	api.Get("/jobs/recommendations", getJobRecommendations)

	// Learning routes
	api.Get("/learning/courses", getLearningCourses)

	// Network routes
	api.Get("/network/leads", getNetworkLeads)
}

func main() {
	// Command line flags
	port := flag.String("port", "3000", "Port to run the server on")
	flag.Parse()

	// Load database
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
	app.Use(cors.New())

	// Setup routes
	setupRoutes(app)

	// Start server
	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

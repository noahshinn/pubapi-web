package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/google/uuid"
)

// Domain Models
type FileType string

const (
	FileTypeFile   FileType = "file"
	FileTypeFolder FileType = "folder"
)

type FileMetadata struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	Type     FileType  `json:"type"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	Owner    string    `json:"owner"`
}

type ShareLink struct {
	ID         string    `json:"id"`
	URL        string    `json:"url"`
	FileID     string    `json:"fileId"`
	Expiration time.Time `json:"expiration"`
	Created    time.Time `json:"created"`
}

type User struct {
	Email        string `json:"email"`
	Name         string `json:"name"`
	StorageUsed  int64  `json:"storage_used"`
	StorageLimit int64  `json:"storage_limit"`
}

// Database represents our in-memory database
type Database struct {
	Users      map[string]User         `json:"users"`
	Files      map[string]FileMetadata `json:"files"`
	ShareLinks map[string]ShareLink    `json:"share_links"`
	FileData   map[string][]byte       `json:"file_data"`
	mu         sync.RWMutex
}

var db *Database

// Error types
var (
	ErrUserNotFound = fmt.Errorf("user not found")
	ErrFileNotFound = fmt.Errorf("file not found")
	ErrStorageFull  = fmt.Errorf("storage quota exceeded")
	ErrInvalidPath  = fmt.Errorf("invalid path")
	ErrUnauthorized = fmt.Errorf("unauthorized")
)

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

func (d *Database) GetFile(fileId string) (FileMetadata, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	file, exists := d.Files[fileId]
	if !exists {
		return FileMetadata{}, ErrFileNotFound
	}
	return file, nil
}

func (d *Database) SaveFile(metadata FileMetadata, data []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Check storage quota
	user, exists := d.Users[metadata.Owner]
	if !exists {
		return ErrUserNotFound
	}

	newSize := user.StorageUsed + int64(len(data))
	if newSize > user.StorageLimit {
		return ErrStorageFull
	}

	// Update storage usage
	user.StorageUsed = newSize
	d.Users[metadata.Owner] = user

	// Save file metadata and data
	d.Files[metadata.ID] = metadata
	d.FileData[metadata.ID] = data

	return nil
}

func (d *Database) DeleteFile(fileId string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	file, exists := d.Files[fileId]
	if !exists {
		return ErrFileNotFound
	}

	// Update storage usage
	user := d.Users[file.Owner]
	user.StorageUsed -= file.Size
	d.Users[file.Owner] = user

	// Delete file
	delete(d.Files, fileId)
	delete(d.FileData, fileId)

	return nil
}

func (d *Database) CreateShareLink(link ShareLink) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.ShareLinks[link.ID] = link
	return nil
}

// HTTP Handlers
func listFiles(c *fiber.Ctx) error {
	path := c.Query("path", "/")
	email := c.Get("X-User-Email")

	if email == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "User email is required",
		})
	}

	// Verify user exists
	_, err := db.GetUser(email)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	var files []FileMetadata
	db.mu.RLock()
	for _, file := range db.Files {
		if file.Owner == email && filepath.Dir(file.Path) == path {
			files = append(files, file)
		}
	}
	db.mu.RUnlock()

	return c.JSON(files)
}

func uploadFile(c *fiber.Ctx) error {
	email := c.Get("X-User-Email")
	if email == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "User email is required",
		})
	}

	// Get the file from form
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "File is required",
		})
	}

	path := c.FormValue("path", "/")

	// Read file content
	fileContent, err := file.Open()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to read file",
		})
	}
	defer fileContent.Close()

	// Create file metadata
	metadata := FileMetadata{
		ID:       uuid.New().String(),
		Name:     file.Filename,
		Path:     filepath.Join(path, file.Filename),
		Type:     FileTypeFile,
		Size:     file.Size,
		Modified: time.Now(),
		Owner:    email,
	}

	// Read file data
	data := make([]byte, file.Size)
	if _, err := fileContent.Read(data); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to read file content",
		})
	}

	// Save file
	if err := db.SaveFile(metadata, data); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(metadata)
}

func downloadFile(c *fiber.Ctx) error {
	fileId := c.Params("fileId")
	email := c.Get("X-User-Email")

	// Get file metadata
	file, err := db.GetFile(fileId)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "File not found",
		})
	}

	// Check ownership or shared access
	if file.Owner != email {
		// Check if file is shared
		hasAccess := false
		db.mu.RLock()
		for _, link := range db.ShareLinks {
			if link.FileID == fileId && link.Expiration.After(time.Now()) {
				hasAccess = true
				break
			}
		}
		db.mu.RUnlock()

		if !hasAccess {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Unauthorized access",
			})
		}
	}

	// Get file data
	db.mu.RLock()
	data, exists := db.FileData[fileId]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "File data not found",
		})
	}

	return c.Send(data)
}

func deleteFile(c *fiber.Ctx) error {
	fileId := c.Params("fileId")
	email := c.Get("X-User-Email")

	// Get file metadata
	file, err := db.GetFile(fileId)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "File not found",
		})
	}

	// Check ownership
	if file.Owner != email {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Unauthorized access",
		})
	}

	// Delete file
	if err := db.DeleteFile(fileId); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to delete file",
		})
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func createShareLink(c *fiber.Ctx) error {
	var req struct {
		FileID     string    `json:"fileId"`
		Expiration time.Time `json:"expiration"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	email := c.Get("X-User-Email")

	// Get file metadata
	file, err := db.GetFile(req.FileID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "File not found",
		})
	}

	// Check ownership
	if file.Owner != email {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Unauthorized access",
		})
	}

	// Create share link
	link := ShareLink{
		ID:         uuid.New().String(),
		URL:        fmt.Sprintf("https://dropbox.com/share/%s", uuid.New().String()),
		FileID:     req.FileID,
		Expiration: req.Expiration,
		Created:    time.Now(),
	}

	if err := db.CreateShareLink(link); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create share link",
		})
	}

	return c.JSON(link)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:      make(map[string]User),
		Files:      make(map[string]FileMetadata),
		ShareLinks: make(map[string]ShareLink),
		FileData:   make(map[string][]byte),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// File routes
	api.Get("/files", listFiles)
	api.Post("/files", uploadFile)
	api.Get("/files/:fileId", downloadFile)
	api.Delete("/files/:fileId", deleteFile)

	// Share routes
	api.Post("/shares", createShareLink)
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
		AllowHeaders: "Origin, Content-Type, Accept, X-User-Email",
	}))

	// Setup routes
	setupRoutes(app)

	// Start server
	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

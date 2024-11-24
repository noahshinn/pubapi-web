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
	"github.com/google/uuid"
)

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
	Shared   bool      `json:"shared"`
}

type SharingLink struct {
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

type Database struct {
	Users        map[string]User         `json:"users"`
	Files        map[string]FileMetadata `json:"files"`
	FileContents map[string][]byte       `json:"file_contents"`
	SharingLinks map[string]SharingLink  `json:"sharing_links"`
	mu           sync.RWMutex
}

var db *Database

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:        make(map[string]User),
		Files:        make(map[string]FileMetadata),
		FileContents: make(map[string][]byte),
		SharingLinks: make(map[string]SharingLink),
	}

	return json.Unmarshal(data, db)
}

func (d *Database) GetUser(email string) (User, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	user, exists := d.Users[email]
	if !exists {
		return User{}, fmt.Errorf("user not found")
	}
	return user, nil
}

func (d *Database) ListFiles(path string) []FileMetadata {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var files []FileMetadata
	for _, file := range d.Files {
		if filepath.Dir(file.Path) == path {
			files = append(files, file)
		}
	}
	return files
}

func (d *Database) GetFile(fileID string) (FileMetadata, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	file, exists := d.Files[fileID]
	if !exists {
		return FileMetadata{}, fmt.Errorf("file not found")
	}
	return file, nil
}

func (d *Database) SaveFile(metadata FileMetadata, content []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Files[metadata.ID] = metadata
	if content != nil {
		d.FileContents[metadata.ID] = content
	}
	return nil
}

func (d *Database) DeleteFile(fileID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.Files, fileID)
	delete(d.FileContents, fileID)
	return nil
}

func (d *Database) CreateSharingLink(link SharingLink) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.SharingLinks[link.ID] = link
	return nil
}

func listFiles(c *fiber.Ctx) error {
	path := c.Query("path", "/")
	files := db.ListFiles(path)
	return c.JSON(files)
}

func uploadFile(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "No file provided",
		})
	}

	path := c.FormValue("path", "/")
	if path == "" {
		path = "/"
	}

	// Read file content
	fileContent, err := file.Open()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to read file",
		})
	}
	defer fileContent.Close()

	// Create file metadata
	fileID := uuid.New().String()
	metadata := FileMetadata{
		ID:       fileID,
		Name:     file.Filename,
		Path:     filepath.Join(path, file.Filename),
		Type:     FileTypeFile,
		Size:     file.Size,
		Modified: time.Now(),
		Shared:   false,
	}

	// Save file content
	buffer := make([]byte, file.Size)
	if _, err := fileContent.Read(buffer); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to read file content",
		})
	}

	if err := db.SaveFile(metadata, buffer); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to save file",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(metadata)
}

func downloadFile(c *fiber.Ctx) error {
	fileID := c.Params("fileId")

	file, err := db.GetFile(fileID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "File not found",
		})
	}

	content, exists := db.FileContents[fileID]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "File content not found",
		})
	}

	return c.Status(fiber.StatusOK).
		Type(filepath.Ext(file.Name)).
		Send(content)
}

func deleteFile(c *fiber.Ctx) error {
	fileID := c.Params("fileId")

	if err := db.DeleteFile(fileID); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "File not found",
		})
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func createSharingLink(c *fiber.Ctx) error {
	var req struct {
		FileID     string    `json:"fileId"`
		Expiration time.Time `json:"expiration"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	file, err := db.GetFile(req.FileID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "File not found",
		})
	}

	link := SharingLink{
		ID:         uuid.New().String(),
		URL:        fmt.Sprintf("https://dropbox.example.com/s/%s", uuid.New().String()),
		FileID:     file.ID,
		Expiration: req.Expiration,
		Created:    time.Now(),
	}

	if err := db.CreateSharingLink(link); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create sharing link",
		})
	}

	// Update file metadata to mark as shared
	file.Shared = true
	if err := db.SaveFile(file, nil); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update file metadata",
		})
	}

	return c.JSON(link)
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

	// File operations
	api.Get("/files", listFiles)
	api.Post("/files", uploadFile)
	api.Get("/files/:fileId", downloadFile)
	api.Delete("/files/:fileId", deleteFile)

	// Sharing operations
	api.Post("/sharing", createSharingLink)

	// User operations
	api.Get("/users/:email", func(c *fiber.Ctx) error {
		email := c.Params("email")
		user, err := db.GetUser(email)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "User not found",
			})
		}
		return c.JSON(user)
	})
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

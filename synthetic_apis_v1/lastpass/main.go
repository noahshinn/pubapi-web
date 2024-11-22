package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
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
type ItemType string

const (
	ItemTypeLogin       ItemType = "login"
	ItemTypeSecureNote  ItemType = "secure_note"
	ItemTypeCreditCard  ItemType = "credit_card"
	ItemTypeBankAccount ItemType = "bank_account"
)

type VaultItem struct {
	ID            string    `json:"id"`
	Type          ItemType  `json:"type"`
	Name          string    `json:"name"`
	EncryptedData string    `json:"encrypted_data"`
	LastModified  time.Time `json:"last_modified"`
}

type Vault struct {
	UserEmail    string      `json:"user_email"`
	VaultItems   []VaultItem `json:"vault_items"`
	LastModified time.Time   `json:"last_modified"`
	VaultVersion int         `json:"vault_version"`
}

type User struct {
	Email             string    `json:"email"`
	EncryptedKey      string    `json:"encrypted_key"`
	KeyIterations     int       `json:"key_iterations"`
	LastLogin         time.Time `json:"last_login"`
	TwoFactorEnabled  bool      `json:"two_factor_enabled"`
	SecurityQuestions []string  `json:"security_questions"`
}

// Database represents our in-memory database
type Database struct {
	Users  map[string]User  `json:"users"`
	Vaults map[string]Vault `json:"vaults"`
	mu     sync.RWMutex
}

// Custom errors
var (
	ErrUserNotFound  = errors.New("user not found")
	ErrVaultNotFound = errors.New("vault not found")
	ErrItemNotFound  = errors.New("item not found")
	ErrInvalidInput  = errors.New("invalid input")
	ErrUnauthorized  = errors.New("unauthorized")
)

// Global database instance
var db *Database

// Encryption helpers (simplified for demo)
func encrypt(data []byte, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func decrypt(encryptedData string, key []byte) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(encryptedData)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

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

func (d *Database) GetVault(email string) (Vault, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	vault, exists := d.Vaults[email]
	if !exists {
		return Vault{}, ErrVaultNotFound
	}
	return vault, nil
}

func (d *Database) AddVaultItem(email string, item VaultItem) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	vault, exists := d.Vaults[email]
	if !exists {
		return ErrVaultNotFound
	}

	vault.VaultItems = append(vault.VaultItems, item)
	vault.LastModified = time.Now()
	d.Vaults[email] = vault

	return nil
}

func (d *Database) UpdateVaultItem(email string, item VaultItem) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	vault, exists := d.Vaults[email]
	if !exists {
		return ErrVaultNotFound
	}

	for i, existingItem := range vault.VaultItems {
		if existingItem.ID == item.ID {
			vault.VaultItems[i] = item
			vault.LastModified = time.Now()
			d.Vaults[email] = vault
			return nil
		}
	}

	return ErrItemNotFound
}

func (d *Database) DeleteVaultItem(email string, itemID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	vault, exists := d.Vaults[email]
	if !exists {
		return ErrVaultNotFound
	}

	for i, item := range vault.VaultItems {
		if item.ID == itemID {
			vault.VaultItems = append(vault.VaultItems[:i], vault.VaultItems[i+1:]...)
			vault.LastModified = time.Now()
			d.Vaults[email] = vault
			return nil
		}
	}

	return ErrItemNotFound
}

// HTTP Handlers
func getVault(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	vault, err := db.GetVault(email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(vault)
}

func addVaultItem(c *fiber.Ctx) error {
	var item VaultItem
	if err := c.BodyParser(&item); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	item.ID = uuid.New().String()
	item.LastModified = time.Now()

	if err := db.AddVaultItem(email, item); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(item)
}

func updateVaultItem(c *fiber.Ctx) error {
	itemID := c.Params("itemId")
	if itemID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "itemId parameter is required",
		})
	}

	var item VaultItem
	if err := c.BodyParser(&item); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	item.ID = itemID
	item.LastModified = time.Now()

	if err := db.UpdateVaultItem(email, item); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(item)
}

func deleteVaultItem(c *fiber.Ctx) error {
	itemID := c.Params("itemId")
	if itemID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "itemId parameter is required",
		})
	}

	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email parameter is required",
		})
	}

	if err := db.DeleteVaultItem(email, itemID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.SendStatus(fiber.StatusNoContent)
}

type SecurityCheckRequest struct {
	Password string `json:"password"`
}

type SecurityCheckResponse struct {
	Strength       string   `json:"strength"`
	DuplicateCount int      `json:"duplicate_count"`
	Compromised    bool     `json:"compromised"`
	Suggestions    []string `json:"suggestions"`
}

func securityCheck(c *fiber.Ctx) error {
	var req SecurityCheckRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Simplified password strength check
	var strength string
	switch {
	case len(req.Password) < 8:
		strength = "weak"
	case len(req.Password) < 12:
		strength = "medium"
	default:
		strength = "strong"
	}

	// Simplified security check response
	response := SecurityCheckResponse{
		Strength:       strength,
		DuplicateCount: 0,     // Would check against vault items in production
		Compromised:    false, // Would check against known breaches in production
		Suggestions: []string{
			"Use a mix of letters, numbers, and symbols",
			"Make your password at least 12 characters long",
			"Avoid using personal information",
		},
	}

	return c.JSON(response)
}

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Users:  make(map[string]User),
		Vaults: make(map[string]Vault),
	}

	return json.Unmarshal(data, db)
}

func setupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")

	// Vault routes
	api.Get("/vault", getVault)
	api.Post("/vault/items", addVaultItem)
	api.Put("/vault/items/:itemId", updateVaultItem)
	api.Delete("/vault/items/:itemId", deleteVaultItem)

	// Security routes
	api.Post("/security-check", securityCheck)
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

	log.Printf("Server starting on port %s", *port)
	if err := app.Listen(":" + *port); err != nil {
		log.Fatal(err)
	}
}

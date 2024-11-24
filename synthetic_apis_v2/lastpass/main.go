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

type ItemType string

const (
	ItemTypeLogin      ItemType = "login"
	ItemTypeSecureNote ItemType = "secure_note"
	ItemTypeCreditCard ItemType = "credit_card"
	ItemTypeAddress    ItemType = "address"
)

type Permission string

const (
	PermissionRead  Permission = "read"
	PermissionWrite Permission = "write"
)

type VaultItem struct {
	ID            string    `json:"id"`
	Type          ItemType  `json:"type"`
	Name          string    `json:"name"`
	EncryptedData string    `json:"encrypted_data"`
	Favorite      bool      `json:"favorite"`
	LastModified  time.Time `json:"last_modified"`
}

type Vault struct {
	Email          string      `json:"email"`
	EncryptedItems []VaultItem `json:"encrypted_items"`
	LastModified   time.Time   `json:"last_modified"`
}

type ShareAccess struct {
	ItemID     string     `json:"item_id"`
	Email      string     `json:"email"`
	Permission Permission `json:"permission"`
}

type Database struct {
	Vaults       map[string]Vault       `json:"vaults"`
	SharedAccess map[string]ShareAccess `json:"shared_access"`
	mu           sync.RWMutex
}

var db *Database

func loadDatabase() error {
	data, err := os.ReadFile("database.json")
	if err != nil {
		return err
	}

	db = &Database{
		Vaults:       make(map[string]Vault),
		SharedAccess: make(map[string]ShareAccess),
	}

	return json.Unmarshal(data, db)
}

func getVault(c *fiber.Ctx) error {
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	db.mu.RLock()
	vault, exists := db.Vaults[email]
	db.mu.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "vault not found",
		})
	}

	// Add shared items to the vault
	var sharedItems []VaultItem
	db.mu.RLock()
	for _, access := range db.SharedAccess {
		if access.Email == email {
			for _, v := range db.Vaults {
				for _, item := range v.EncryptedItems {
					if item.ID == access.ItemID {
						sharedItems = append(sharedItems, item)
					}
				}
			}
		}
	}
	db.mu.RUnlock()

	vault.EncryptedItems = append(vault.EncryptedItems, sharedItems...)
	return c.JSON(vault)
}

func addVaultItem(c *fiber.Ctx) error {
	var item VaultItem
	if err := c.BodyParser(&item); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	item.ID = uuid.New().String()
	item.LastModified = time.Now()

	db.mu.Lock()
	vault, exists := db.Vaults[email]
	if !exists {
		vault = Vault{
			Email:        email,
			LastModified: time.Now(),
		}
	}

	vault.EncryptedItems = append(vault.EncryptedItems, item)
	vault.LastModified = time.Now()
	db.Vaults[email] = vault
	db.mu.Unlock()

	return c.Status(fiber.StatusCreated).JSON(item)
}

func updateVaultItem(c *fiber.Ctx) error {
	itemID := c.Params("itemId")
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	var updatedItem VaultItem
	if err := c.BodyParser(&updatedItem); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	vault, exists := db.Vaults[email]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "vault not found",
		})
	}

	found := false
	for i, item := range vault.EncryptedItems {
		if item.ID == itemID {
			updatedItem.ID = itemID
			updatedItem.LastModified = time.Now()
			vault.EncryptedItems[i] = updatedItem
			found = true
			break
		}
	}

	if !found {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "item not found",
		})
	}

	vault.LastModified = time.Now()
	db.Vaults[email] = vault

	return c.JSON(updatedItem)
}

func deleteVaultItem(c *fiber.Ctx) error {
	itemID := c.Params("itemId")
	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	vault, exists := db.Vaults[email]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "vault not found",
		})
	}

	found := false
	var updatedItems []VaultItem
	for _, item := range vault.EncryptedItems {
		if item.ID != itemID {
			updatedItems = append(updatedItems, item)
		} else {
			found = true
		}
	}

	if !found {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "item not found",
		})
	}

	vault.EncryptedItems = updatedItems
	vault.LastModified = time.Now()
	db.Vaults[email] = vault

	return c.SendStatus(fiber.StatusNoContent)
}

func shareVaultItem(c *fiber.Ctx) error {
	var shareReq struct {
		ItemID         string     `json:"item_id"`
		RecipientEmail string     `json:"recipient_email"`
		Permission     Permission `json:"permission"`
	}

	if err := c.BodyParser(&shareReq); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	email := c.Query("email")
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "email is required",
		})
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	// Verify item exists in user's vault
	vault, exists := db.Vaults[email]
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "vault not found",
		})
	}

	itemExists := false
	for _, item := range vault.EncryptedItems {
		if item.ID == shareReq.ItemID {
			itemExists = true
			break
		}
	}

	if !itemExists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "item not found",
		})
	}

	// Create share access
	shareID := uuid.New().String()
	db.SharedAccess[shareID] = ShareAccess{
		ItemID:     shareReq.ItemID,
		Email:      shareReq.RecipientEmail,
		Permission: shareReq.Permission,
	}

	return c.SendStatus(fiber.StatusOK)
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

	// Vault routes
	api.Get("/vault", getVault)
	api.Post("/vault/items", addVaultItem)
	api.Put("/vault/items/:itemId", updateVaultItem)
	api.Delete("/vault/items/:itemId", deleteVaultItem)
	api.Post("/sharing", shareVaultItem)
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

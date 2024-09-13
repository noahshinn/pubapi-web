package cache

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
)

type Cache interface {
	Get(key string) (any, error)
	Set(key string, value any) error
}

func GetCacheRootPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".cache", "pubapi-web"), nil
}

type DiskCache interface {
	Cache
	SaveToDisk() error
	Path() string
}

type BasicDiskCache struct {
	diskPath string
	cache    map[string]any
}

func NewDiskCacheFromPath(diskPath string) (DiskCache, error) {
	if _, err := os.Stat(diskPath); errors.Is(err, os.ErrNotExist) {
		log.Printf("cache not found at %s, starting with empty cache", diskPath)
		if err := os.MkdirAll(filepath.Dir(diskPath), 0o755); err != nil {
			return nil, err
		}
		file, err := os.Create(diskPath)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		log.Printf("created cache at %s", diskPath)
		return &BasicDiskCache{diskPath: diskPath, cache: make(map[string]any)}, nil
	}
	cache := make(map[string]any)
	file, err := os.ReadFile(diskPath)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(file, &cache)
	if err != nil {
		log.Printf("error unmarshalling cache, starting with empty cache: %v", err)
	}
	return &BasicDiskCache{diskPath: diskPath, cache: cache}, nil
}

func (c *BasicDiskCache) Get(key string) (any, error) {
	if value, ok := c.cache[key]; ok {
		return value, nil
	}
	return nil, errors.New("key not found in cache")
}

func (c *BasicDiskCache) Set(key string, value any) error {
	c.cache[key] = value
	return nil
}

func (c *BasicDiskCache) SaveToDisk() error {
	file, err := os.Create(c.diskPath)
	if err != nil {
		return err
	}
	defer file.Close()
	// serialize to json for now
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(c.cache)
}

func (c *BasicDiskCache) Path() string {
	return c.diskPath
}

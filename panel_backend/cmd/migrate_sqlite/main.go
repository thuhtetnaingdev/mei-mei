package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"panel_backend/internal/models"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func main() {
	sourceURL := os.Getenv("SOURCE_DATABASE_URL")
	if sourceURL == "" {
		log.Fatal("missing SOURCE_DATABASE_URL")
	}

	targetPath := os.Getenv("TARGET_DATABASE_PATH")
	if targetPath == "" {
		targetPath = "./panel.sqlite3"
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil && filepath.Dir(targetPath) != "." {
		log.Fatalf("failed to create sqlite directory: %v", err)
	}

	sourceDB, err := gorm.Open(postgres.Open(sourceURL), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect source database: %v", err)
	}

	targetDB, err := gorm.Open(sqlite.Open(targetPath), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect target database: %v", err)
	}

	if err := targetDB.AutoMigrate(&models.User{}, &models.Node{}, &models.AdminSetting{}); err != nil {
		log.Fatalf("failed to migrate sqlite schema: %v", err)
	}

	var admins []models.AdminSetting
	var nodes []models.Node
	var users []models.User

	if err := sourceDB.Order("id asc").Find(&admins).Error; err != nil {
		log.Fatalf("failed to read admins: %v", err)
	}
	if err := sourceDB.Order("id asc").Find(&nodes).Error; err != nil {
		log.Fatalf("failed to read nodes: %v", err)
	}
	if err := sourceDB.Order("id asc").Find(&users).Error; err != nil {
		log.Fatalf("failed to read users: %v", err)
	}

	if err := targetDB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM admin_settings").Error; err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM nodes").Error; err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM users").Error; err != nil {
			return err
		}

		if len(admins) > 0 {
			if err := tx.Create(&admins).Error; err != nil {
				return err
			}
		}
		if len(nodes) > 0 {
			if err := tx.Create(&nodes).Error; err != nil {
				return err
			}
		}
		if len(users) > 0 {
			if err := tx.Create(&users).Error; err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		log.Fatalf("failed to copy data: %v", err)
	}

	fmt.Printf("migrated %d admins, %d nodes, %d users to %s\n", len(admins), len(nodes), len(users), targetPath)
}

package db

import (
	"os"
	"path/filepath"
	"panel_backend/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func Connect(databasePath string) (*gorm.DB, error) {
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil && filepath.Dir(databasePath) != "." {
		return nil, err
	}

	conn, err := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	if err := conn.AutoMigrate(&models.User{}, &models.Node{}, &models.AdminSetting{}); err != nil {
		return nil, err
	}

	return conn, nil
}

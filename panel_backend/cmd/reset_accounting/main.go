package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"panel_backend/internal/db"
	"panel_backend/internal/services"

	"github.com/joho/godotenv"
)

func main() {
	var yes bool
	var databasePath string

	flag.BoolVar(&yes, "yes", false, "confirm destructive accounting reset")
	flag.StringVar(&databasePath, "database-path", "", "override database path")
	flag.Parse()

	if !yes {
		log.Fatal("refusing to reset accounting without --yes")
	}

	_ = godotenv.Load()
	if databasePath == "" {
		databasePath = os.Getenv("DATABASE_PATH")
	}
	if databasePath == "" {
		databasePath = "./panel.sqlite3"
	}

	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil && filepath.Dir(databasePath) != "." {
		log.Fatalf("failed to create database directory: %v", err)
	}

	database, err := db.Connect(databasePath)
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	summary, err := services.ResetAccounting(database)
	if err != nil {
		log.Fatalf("failed to reset accounting: %v", err)
	}

	fmt.Printf("accounting reset complete\n")
	fmt.Printf("users reset: %d\n", summary.UsersReset)
	fmt.Printf("nodes reset: %d\n", summary.NodesReset)
	fmt.Printf("miners reset: %d\n", summary.MinersReset)
	fmt.Printf("allocations reset: %d\n", summary.AllocationsReset)
	fmt.Printf("node usage entries deleted: %d\n", summary.NodeUsageEntriesDeleted)
	fmt.Printf("miner rewards deleted: %d\n", summary.MinerRewardsDeleted)
	fmt.Printf("mint events deleted: %d\n", summary.MintEventsDeleted)
	fmt.Printf("mint transfers deleted: %d\n", summary.MintTransfersDeleted)
}

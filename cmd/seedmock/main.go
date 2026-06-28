package main

import (
	"flag"
	"fmt"
	"log"

	"study-tracker/db"
)

func main() {
	dbPath := flag.String("db", "./data.db", "path to sqlite database")
	flag.Parse()

	database := db.New(*dbPath)
	if err := database.SeedMockData(); err != nil {
		log.Fatalf("failed to seed mock data: %v", err)
	}

	fmt.Printf("mock data seeded successfully into %s\n", *dbPath)
}

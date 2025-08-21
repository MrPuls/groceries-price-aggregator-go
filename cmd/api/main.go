package main

import (
	"context"
	"log"

	"github.com/MrPuls/groceries-price-aggregator-go/internal/api"
	"github.com/MrPuls/groceries-price-aggregator-go/internal/db"
)

func main() {
	ctx := context.Background()

	database, err := db.NewDB(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer database.Pool.Close()

	server := api.NewServer(8080, database)
	server.Start()
}

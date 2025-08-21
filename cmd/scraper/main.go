package main

import (
	"context"
	"log"
	"time"

	"github.com/MrPuls/groceries-price-aggregator-go/cmd/scraper/runner"
)

func main() {
	log.Println("Starting main program")
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(5*time.Minute))
	defer cancel()
	r := runner.NewRunner(ctx)
	r.Run()
	log.Println("All scrapers are done!")

	log.Println("Writing CSV data")
	db, err := r.ConnectToDB(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Pool.Close()

	wErr := r.WriteCSVData(ctx, db, r.Files)
	if wErr != nil {
		log.Fatal(err)
	}
	log.Println("CSV data written successfully")
}

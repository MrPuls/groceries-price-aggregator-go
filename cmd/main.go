package main

import (
	"context"
	"log"
	"time"

	"github.com/MrPuls/groceries-price-aggregator-go/cmd/runner"
)

func main() {
	log.Println("Starting main program")
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(1*time.Minute))
	defer cancel()
	r := runner.NewRunner(ctx)
	r.Run()
	log.Println("All done!")
}

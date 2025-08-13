package main

import (
	"github.com/MrPuls/groceries-price-aggregator-go/internal/scrapers"
)

func main() {
	//log.Println("Starting main program")
	//ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(1*time.Minute))
	//defer cancel()
	//r := runner.NewRunner(ctx)
	//r.Run()
	//log.Println("All done!")

	scrapers.ParseHTML()
}

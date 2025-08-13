package main

import (
	"context"
	"log"
	"time"

	"github.com/MrPuls/groceries-price-aggregator-go/internal/scrapers"
	"github.com/MrPuls/groceries-price-aggregator-go/internal/utils"
)

func main() {
	//log.Println("Starting main program")
	//slp := scrapers.NewSilpoScraper()
	//ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(1*time.Minute))
	//defer cancel()
	//cts, ctsErr := slp.GetCategories(ctx)
	//if ctsErr != nil {
	//	log.Fatal(ctsErr)
	//}
	//
	//products, prErr := slp.GetProducts(ctx, cts.Items)
	//if prErr != nil {
	//	log.Fatal(prErr)
	//}
	//wErr := utils.WriteToCsv("silpo", slp.CSVHeader, products)
	//if wErr != nil {
	//	log.Fatal(wErr)
	//}

	//log.Println("Starting main program")
	//mt := scrapers.NewMetroScraper()
	//ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(30*time.Second))
	//defer cancel()
	//cts, ctsErr := mt.GetCategories(ctx)
	//if ctsErr != nil {
	//	log.Fatal(ctsErr)
	//}
	//
	//products, prErr := mt.GetProducts(ctx, cts)
	//if prErr != nil {
	//	log.Fatal(prErr)
	//}
	//wErr := utils.WriteToCsv("metro", mt.CSVHeader, products)
	//if wErr != nil {
	//	log.Fatal(wErr)
	//}

	log.Println("Starting main program")
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(1*time.Minute))
	defer cancel()
	vs := scrapers.NewVarusScraper()
	cts, err := vs.GetCategories(ctx)
	if err != nil {
		log.Fatal(err)
	}
	tErr := vs.GetProductsTotalValues(ctx, cts)
	if tErr != nil {
		log.Fatal(tErr)
	}
	products, prErr := vs.GetProducts(ctx, cts)
	if prErr != nil {
		log.Fatal(prErr)
	}
	wErr := utils.WriteToCsv("varus", vs.CSVHeader, products)
	if wErr != nil {
		log.Fatal(wErr)
	}

}

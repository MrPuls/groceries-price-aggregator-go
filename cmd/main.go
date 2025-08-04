package main

import (
	"github.com/MrPuls/groceries-price-aggregator-go/internal/scrappers"
	"github.com/MrPuls/groceries-price-aggregator-go/internal/utils"
	"log"
)

func main() {
	log.Println("Starting main program")
	slp := scrappers.NewSilpoClient()
	cts, ctsErr := slp.GetCategories()
	if ctsErr != nil {
		log.Fatal(ctsErr)
	}
	slp.GetCategoriesTitles(cts)

	products, prErr := slp.GetProducts(cts.Items)
	if prErr != nil {
		log.Fatal(prErr)
	}
	wErr := utils.WriteToCsv("silpo", slp.CSVHeader, products)
	if wErr != nil {
		return
	}

}

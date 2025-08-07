package main

import (
	"fmt"
	"log"

	"github.com/MrPuls/groceries-price-aggregator-go/internal/scrappers"
)

func main() {
	//log.Println("Starting main program")
	//slp := scrappers.NewSilpoClient()
	//cts, ctsErr := slp.GetCategories()
	//if ctsErr != nil {
	//	log.Fatal(ctsErr)
	//}
	//slp.GetCategoriesTitles(cts)
	//
	//products, prErr := slp.GetProducts(cts.Items)
	//if prErr != nil {
	//	log.Fatal(prErr)
	//}
	//wErr := utils.WriteToCsv("silpo", slp.CSVHeader, products)
	//if wErr != nil {
	//	log.Fatal(wErr)
	//}

	//log.Println("Starting main program")
	//mt := scrappers.NewMetroClient()
	//cts, ctsErr := mt.GetCategories()
	//if ctsErr != nil {
	//	log.Fatal(ctsErr)
	//}
	//
	//products, prErr := mt.GetProducts(cts)
	//if prErr != nil {
	//	log.Fatal(prErr)
	//}
	//wErr := utils.WriteToCsv("metro", mt.CSVHeader, products)
	//if wErr != nil {
	//	log.Fatal(wErr)
	//}

	vs := scrappers.NewVarusClient()
	cts, err := vs.GetCategories()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(cts)

}

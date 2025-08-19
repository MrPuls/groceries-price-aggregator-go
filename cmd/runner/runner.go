package runner

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/MrPuls/groceries-price-aggregator-go/internal/db"
	"github.com/MrPuls/groceries-price-aggregator-go/internal/scrapers"
	"github.com/MrPuls/groceries-price-aggregator-go/internal/utils"
)

type Runner struct {
	ctx       context.Context
	csvHeader []string
	Files     []string
}

func NewRunner(ctx context.Context) *Runner {
	return &Runner{
		ctx:       ctx,
		csvHeader: []string{"Name", "Ref", "Price", "Category", "Shop"},
		Files:     []string{},
	}
}

func (r *Runner) Run() {
	var wg sync.WaitGroup
	defer wg.Wait()
	wg.Add(1)
	go r.startSilpoScraper(&wg)
	wg.Add(1)
	go r.startMetroScraper(&wg)
	wg.Add(1)
	go r.startVarusScraper(&wg)
	wg.Add(1)
	go r.startAtbScraper(&wg)
}

func (r *Runner) startSilpoScraper(wg *sync.WaitGroup) {
	defer wg.Done()
	log.Println("Starting Silpo scraper")
	slp := scrapers.NewSilpoScraper()
	cts, err := slp.GetCategories(r.ctx)
	if err != nil {
		fmt.Printf("[Silpo] error getting categories: %v", err)
	}
	products, err := slp.GetProducts(r.ctx, cts.Items)
	if err != nil {
		fmt.Printf("[Silpo] error getting products: %v", err)
	}
	filename, err := utils.WriteToCsv("silpo", r.csvHeader, products)
	if err != nil {
		fmt.Printf("[Silpo] error writing to csv: %v", err)
	}
	r.Files = append(r.Files, filename)
}

func (r *Runner) startMetroScraper(wg *sync.WaitGroup) {
	defer wg.Done()
	log.Println("Starting Metro scraper")
	mt := scrapers.NewMetroScraper()
	cts, err := mt.GetCategories(r.ctx)
	if err != nil {
		fmt.Printf("[Metro] error getting categories: %v", err)
	}

	products, err := mt.GetProducts(r.ctx, cts)
	if err != nil {
		fmt.Printf("[Metro] error getting products: %v", err)
	}
	filename, err := utils.WriteToCsv("metro", r.csvHeader, products)
	if err != nil {
		fmt.Printf("[Metro] error writing to csv: %v", err)
	}
	r.Files = append(r.Files, filename)
}

func (r *Runner) startVarusScraper(wg *sync.WaitGroup) {
	defer wg.Done()
	log.Println("Starting Varus scraper")
	vs := scrapers.NewVarusScraper()
	cts, err := vs.GetCategories(r.ctx)
	if err != nil {
		fmt.Printf("[Varus] error getting categories: %v", err)
	}
	tErr := vs.GetProductsTotalValues(r.ctx, cts)
	if tErr != nil {
		fmt.Printf("[Varus] error getting products total values: %v", tErr)
	}
	products, err := vs.GetProducts(r.ctx, cts)
	if err != nil {
		fmt.Printf("[Varus] error getting products: %v", err)
	}
	filename, err := utils.WriteToCsv("varus", r.csvHeader, products)
	if err != nil {
		fmt.Printf("[Varus] error writing to csv: %v", err)
	}
	r.Files = append(r.Files, filename)
}

func (r *Runner) startAtbScraper(wg *sync.WaitGroup) {
	defer wg.Done()
	log.Println("Starting Atb scraper")
	atb := scrapers.NewAtbScraper()
	cts, err := atb.GetCategories(r.ctx)
	if err != nil {
		fmt.Printf("[Atb] error getting categories: %v", err)
	}
	products, err := atb.GetProducts(r.ctx, cts)
	if err != nil {
		fmt.Printf("[Atb] error getting products: %v", err)
	}
	filename, err := utils.WriteToCsv("atb", r.csvHeader, products)
	if err != nil {
		fmt.Printf("[Atb] error writing to csv: %v", err)
	}
	r.Files = append(r.Files, filename)
}

func (r *Runner) ConnectToDB(ctx context.Context) (*db.DB, error) {
	database, err := db.NewDB(ctx)
	if err != nil {
		return nil, err
	}
	return database, nil
}

func (r *Runner) WriteCSVData(ctx context.Context, database *db.DB, files []string) error {
	wg := sync.WaitGroup{}
	for _, f := range files {
		wg.Add(1)
		go func() {
			defer wg.Done()
			products, err := database.ReadCSVData(f)
			if err != nil {
				log.Fatal("Failed to read CSV:", err)
			}

			log.Printf("Read %d products from CSV", len(products))

			err = database.BulkUpsertProducts(ctx, products)
			if err != nil {
				log.Fatal("Failed to bulk upsert products:", err)
			}

			log.Println("Successfully imported products and prices")
		}()
	}
	wg.Wait()
	return nil
}

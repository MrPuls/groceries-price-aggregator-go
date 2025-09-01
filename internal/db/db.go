package db

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	Pool *pgxpool.Pool
}

type Product struct {
	Name      string
	Ref       string
	Price     string
	Category  string
	Shop      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func NewDB(ctx context.Context) (*DB, error) {
	pool, err := connect(ctx)
	if err != nil {
		return nil, err
	}
	return &DB{Pool: pool}, nil
}

func connect(ctx context.Context) (*pgxpool.Pool, error) {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		dsn = "postgres://gp_user:gp_pass@localhost:5432/groceries?sslmode=disable"
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 100
	cfg.MaxConnLifetime = 5 * time.Minute
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	ctxPing, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(ctxPing); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func (db *DB) ReadCSVData(filename string) ([]Product, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open CSV file: %w", err)
	}
	defer func() { _ = file.Close() }()

	reader := csv.NewReader(file)

	// Read header
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV header: %w", err)
	}

	// Validate header
	expectedHeader := []string{"Name", "Ref", "Price", "Category", "Shop"}
	if len(header) != len(expectedHeader) {
		return nil, fmt.Errorf("invalid CSV header: expected %v, got %v", expectedHeader, header)
	}

	var products []Product
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read CSV record: %w", err)
		}

		// Parse price (remove currency and convert)
		//priceStr = strings.ReplaceAll(priceStr, " грн/шт", "")
		//priceStr = strings.ReplaceAll(priceStr, ",", ".")
		//price, err := strconv.ParseFloat(priceStr, 64)
		//if err != nil {
		//	return nil, fmt.Errorf("failed to parse price '%s': %w", record[2], err)
		//}
		name := strings.TrimSpace(record[0])
		name = strings.ReplaceAll(name, "\"", "")
		name = strings.ReplaceAll(name, "«", "")
		name = strings.ReplaceAll(name, "»", "")
		name = strings.ReplaceAll(name, ", ", "")

		product := Product{
			Name:     name,
			Ref:      strings.TrimSpace(record[1]),
			Price:    strings.TrimSpace(record[2]),
			Category: strings.TrimSpace(record[3]),
			Shop:     strings.TrimSpace(record[4]),
		}
		products = append(products, product)
	}

	return products, nil
}

func (db *DB) upsertProducts(ctx context.Context, tx pgx.Tx, products []Product, storeIDs map[string]int64) (map[string]int64, error) {
	productIDs := make(map[string]int64)

	for _, p := range products {
		storeID := storeIDs[p.Shop]

		var productID int64
		err := tx.QueryRow(ctx, `
			INSERT INTO products (store_id, ref, name, url, created_at, updated_at) 
			VALUES ($1, $2, $3, $4, now(), now())
			ON CONFLICT (store_id, ref) 
			DO UPDATE SET 
				name = EXCLUDED.name,
				url = EXCLUDED.url,
				updated_at = now()
			RETURNING id`,
			storeID, p.Ref, p.Name, p.Ref).Scan(&productID)

		if err != nil {
			return nil, fmt.Errorf("failed to upsert product '%s': %w", p.Name, err)
		}

		productIDs[fmt.Sprintf("%s:%s", p.Shop, p.Ref)] = productID
	}

	return productIDs, nil
}

// BulkUpsertProducts efficiently inserts/updates products and their prices
func (db *DB) BulkUpsertProducts(ctx context.Context, products []Product) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Get store IDs
	storeIDs, err := db.getStoreIDs(ctx, tx, products)
	if err != nil {
		return fmt.Errorf("failed to get store IDs: %w", err)
	}

	// Upsert categories
	_, err = db.upsertCategories(ctx, tx, products, storeIDs)
	if err != nil {
		return fmt.Errorf("failed to upsert categories: %w", err)
	}

	// Upsert products
	productIDs, err := db.upsertProducts(ctx, tx, products, storeIDs)
	if err != nil {
		return fmt.Errorf("failed to upsert products: %w", err)
	}

	// Insert prices
	if err := db.insertPrices(ctx, tx, products, productIDs); err != nil {
		return fmt.Errorf("failed to insert prices: %w", err)
	}

	return tx.Commit(ctx)
}

// getStoreIDs retrieves store IDs for all shops in the products
func (db *DB) getStoreIDs(ctx context.Context, tx pgx.Tx, products []Product) (map[string]int64, error) {
	// Get unique shop names
	shops := make(map[string]bool)
	for _, p := range products {
		shops[p.Shop] = true
	}

	storeIDs := make(map[string]int64)
	for shop := range shops {
		var storeID int64
		err := tx.QueryRow(ctx, "SELECT id FROM stores WHERE name = $1 OR code = $1", shop).Scan(&storeID)
		if err != nil {
			return nil, fmt.Errorf("store '%s' not found: %w", shop, err)
		}
		storeIDs[shop] = storeID
	}

	return storeIDs, nil
}

// upsertCategories inserts or updates categories
func (db *DB) upsertCategories(ctx context.Context, tx pgx.Tx, products []Product, storeIDs map[string]int64) (map[string]int64, error) {
	// Get unique categories per store
	categories := make(map[string]map[string]bool) // store -> category -> exists
	for _, p := range products {
		if categories[p.Shop] == nil {
			categories[p.Shop] = make(map[string]bool)
		}
		categories[p.Shop][p.Category] = true
	}

	categoryIDs := make(map[string]int64) // "store:category" -> id

	for shop, cats := range categories {
		storeID := storeIDs[shop]
		for category := range cats {
			slug := strings.ToLower(strings.ReplaceAll(category, " ", "-"))

			var categoryID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO categories (store_id, slug, name, created_at, updated_at) 
				VALUES ($1, $2, $3, now(), now())
				ON CONFLICT (store_id, slug) 
				DO UPDATE SET 
					name = EXCLUDED.name, 
					updated_at = now()
				RETURNING id`,
				storeID, slug, category).Scan(&categoryID)

			if err != nil {
				return nil, fmt.Errorf("failed to upsert category '%s' for store '%s': %w", category, shop, err)
			}

			categoryIDs[fmt.Sprintf("%s:%s", shop, category)] = categoryID
		}
	}

	return categoryIDs, nil
}

// insertPrices inserts new price records
func (db *DB) insertPrices(ctx context.Context, tx pgx.Tx, products []Product, productIDs map[string]int64) error {
	// Prepare batch insert
	batch := &pgx.Batch{}

	for _, p := range products {
		productID := productIDs[fmt.Sprintf("%s:%s", p.Shop, p.Ref)]
		batch.Queue(`
			INSERT INTO prices (product_id, price, currency, created_at, updated_at) 
			VALUES ($1, $2, $3, now(), now())`,
			productID, p.Price, "UAH")
	}

	results := tx.SendBatch(ctx, batch)
	defer func() { _ = results.Close() }()

	// Process all results
	for i := 0; i < len(products); i++ {
		_, err := results.Exec()
		if err != nil {
			return fmt.Errorf("failed to insert price for product %d: %w", i, err)
		}
	}

	return nil
}

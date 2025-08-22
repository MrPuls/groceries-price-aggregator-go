package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/MrPuls/groceries-price-aggregator-go/internal/db"
)

type Store struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Code string `json:"code"`
}

type Product struct {
	Name     string `json:"name"`
	Price    string `json:"price"`
	Currency string `json:"currency"`
	Ref      string `json:"url"`
}

type Server struct {
	Port   int
	DB     *db.DB
	Router *http.ServeMux
}

func NewServer(port int, db *db.DB) *Server {
	return &Server{
		Port:   port,
		DB:     db,
		Router: http.NewServeMux(),
	}
}

func (s *Server) Start() {
	log.Printf("Starting server on port %d", s.Port)

	s.Router.HandleFunc("GET /", s.helloWorld)
	s.Router.HandleFunc("GET /api/v1/stores", s.getStores)
	s.Router.HandleFunc("GET /api/v1/products", s.getProducts)

	err := http.ListenAndServe(fmt.Sprintf(":%v", s.Port), s.Router)
	if err != nil {
		log.Fatal(err)
	}
}

func (s *Server) helloWorld(w http.ResponseWriter, r *http.Request) {
	_, err := io.WriteString(w, "Hello World!")
	if err != nil {
		return
	}
}

func (s *Server) getStores(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := s.DB.Pool.Query(ctx, "SELECT id, name, code FROM stores")
	if err != nil {
		log.Printf("Database query failed: %v", err)
		http.Error(w, fmt.Sprintf("Database query failed: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var stores []Store
	for rows.Next() {
		var store Store
		err := rows.Scan(&store.ID, &store.Name, &store.Code)
		if err != nil {
			log.Printf("Failed to scan row: %v", err)
			http.Error(w, fmt.Sprintf("Failed to scan row: %v", err), http.StatusInternalServerError)
			return
		}
		stores = append(stores, store)
	}

	if err := rows.Err(); err != nil {
		log.Printf("Row iteration error: %v", err)
		http.Error(w, fmt.Sprintf("Row iteration error: %v", err), http.StatusInternalServerError)
		return
	}

	jsonData, err := json.Marshal(stores)
	if err != nil {
		log.Printf("JSON marshaling failed: %v", err)
		http.Error(w, fmt.Sprintf("JSON marshaling failed: %v", err), http.StatusInternalServerError)
		return
	}

	_, wErr := w.Write(jsonData)
	if wErr != nil {
		return
	}
}

// TODO: Perhaps, instead of fetching all products from the database,
//
//	we should fetch the unique products and then list their availability and prices for each store,
//		similar to what hotline does.
//		It can look like a product tile with a little store icon,
//		showing in which stores it is available and after the click on tile - show the prices in each store.
func (s *Server) getProducts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	query := strings.ReplaceAll(r.URL.Query().Get("search"), " ", " & ")
	rows, err := s.DB.Pool.Query(
		context.Background(),
		"SELECT p.name, p.url, pr.price, pr.currency FROM products p INNER JOIN prices pr on p.id = pr.product_id WHERE to_tsvector('ukrainian', name) @@ to_tsquery('ukrainian', $1)", query,
	)
	if err != nil {
		log.Printf("Database query failed: %v", err)
		http.Error(w, fmt.Sprintf("Database query failed: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var products []Product
	for rows.Next() {
		var product Product
		err := rows.Scan(&product.Name, &product.Ref, &product.Price, &product.Currency)
		if err != nil {
			log.Printf("Failed to scan row: %v", err)
			http.Error(w, fmt.Sprintf("Failed to scan row: %v", err), http.StatusInternalServerError)
		}
		products = append(products, product)
	}

	if err := rows.Err(); err != nil {
		log.Printf("Row iteration error: %v", err)
		http.Error(w, fmt.Sprintf("Row iteration error: %v", err), http.StatusInternalServerError)
		return
	}

	jsonData, err := json.Marshal(products)
	if err != nil {
		log.Printf("JSON marshaling failed: %v", err)
		http.Error(w, fmt.Sprintf("JSON marshaling failed: %v", err), http.StatusInternalServerError)
		return
	}

	_, wErr := w.Write(jsonData)
	if wErr != nil {
		return
	}
}

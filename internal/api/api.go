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

type ProductStoreMapping struct {
	ProductID int    `json:"product_id"`
	StoreName string `json:"store_name"`
}

type Product struct {
	Name         string      `json:"name"`
	Stores       []string    `json:"available_stores"`
	StoreMapping interface{} `json:"product_store_mapping"`
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
	s.Router.HandleFunc("GET /api/v1/products/{productId}", s.getProductById)

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

func (s *Server) getProducts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	query := strings.ReplaceAll(r.URL.Query().Get("search"), " ", " & ")
	rows, err := s.DB.Pool.Query(
		context.Background(), `
		SELECT p1.name,
	   	jsonb_object_agg(p_info.store_name, p_info.id) as product_store_mapping,
	   	array_agg(DISTINCT p_info.store_name) as available_stores
		FROM products p1
			 CROSS JOIN (
		SELECT p2.id, p2.name, p2.store_id, s2.name as store_name
		FROM products p2
				 JOIN stores s2 ON p2.store_id = s2.id
		) p_info
		WHERE p1.store_id != p_info.store_id
		  AND to_tsvector('ukrainian', p1.name) @@ to_tsquery('ukrainian', $1)
		  AND to_tsvector('ukrainian', p_info.name) @@ to_tsquery('ukrainian', $1)
		  AND similarity(p1.name, p_info.name) > 0.9
		GROUP BY p1.name, p1.id;`, query)
	if err != nil {
		log.Printf("Database query failed: %v", err)
		http.Error(w, fmt.Sprintf("Database query failed: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var products []Product
	for rows.Next() {
		var product Product
		err := rows.Scan(&product.Name, &product.StoreMapping, &product.Stores)
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

func (s *Server) getProductById(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	productId := r.PathValue("productId")
	fmt.Println(productId)
}

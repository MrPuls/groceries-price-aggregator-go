package scrapers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MrPuls/groceries-price-aggregator-go/internal/utils"
)

const (
	metroBaseURL         = "https://stores-api.zakaz.ua/stores/48215614/categories"
	metroProductPageSize = 30
	metroSemaphoreSize   = 35
)

type MetroScraper struct {
	Client  *http.Client
	Headers map[string]string
}

type MetroCategoryItem struct {
	Title string `json:"title"`
	Slug  string `json:"id"`
	Total int    `json:"count"`
}

type MetroProduct struct {
	Name  string  `json:"title"`
	Price float64 `json:"price"`
	Ref   string  `json:"web_url"`
}

type MetroProducts struct {
	Total int            `json:"total"`
	Items []MetroProduct `json:"results"`
}

func NewMetroScraper() *MetroScraper {
	return &MetroScraper{
		Client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				MaxConnsPerHost:     75,
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
				DisableKeepAlives:   false,
			},
		},
		Headers: map[string]string{
			"Host":             "stores-api.zakaz.ua",
			"User-Agent":       "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:141.0) Gecko/20100101 Firefox/141.0",
			"Accept":           "*/*",
			"Accept-Language":  "uk",
			"Accept-Encoding":  "utf-8",
			"Referer":          "https://metro.zakaz.ua/uk/",
			"Content-Type":     "application/json",
			"x-chain":          "metro",
			"X-Delivery-Type":  "plan",
			"x-version":        "65",
			"Origin":           "https://metro.zakaz.ua",
			"Sec-GPC":          "1",
			"Connection":       "keep-alive",
			"Sec-Fetch-Dest":   "empty",
			"Sec-Fetch-Mode":   "cors",
			"Sec-Fetch-Site":   "same-site",
			"content-language": "uk",
		},
	}
}

func (m *MetroScraper) GetCategories(ctx context.Context) ([]MetroCategoryItem, error) {
	params := map[string]string{
		"only_parents": "true",
	}
	p := utils.PrepareURLParams(params)
	req, err := utils.MakeGetRequest(ctx, metroBaseURL, m.Headers, p)
	if err != nil {
		return nil, err
	}

	resp, err := m.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("[Metro] getting categories: status code %d", resp.StatusCode)
	}

	readBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var c []MetroCategoryItem
	jsonErr := json.Unmarshal(readBody, &c)
	if jsonErr != nil {
		return nil, jsonErr
	}
	var total int
	for _, v := range c {
		total += v.Total
	}
	fmt.Printf("[Metro] Found %v categories with total amount of items: %v\n", len(c), total)

	return c, nil
}

func (m *MetroScraper) GetProducts(ctx context.Context, cts []MetroCategoryItem) ([][]string, error) {
	var result [][]string
	var wg sync.WaitGroup
	httpSemaphore := make(chan struct{}, metroSemaphoreSize)
	resultsChan := make(chan []string)
	for _, ci := range cts {
		wg.Add(1)
		numPages := (ci.Total / metroProductPageSize) + 1
		go func(ci MetroCategoryItem) {
			defer wg.Done()
			var pageWg sync.WaitGroup
			for page := 1; page <= numPages; page++ {
				pageWg.Add(1)
				go func(page int) {
					httpSemaphore <- struct{}{}
					defer func() { <-httpSemaphore }()
					defer pageWg.Done()
					select {
					case <-ctx.Done():
						return
					default:
					}
					products, err := m.getProductsFromPage(ctx, page, ci.Slug)
					if err != nil {
						log.Printf("[Metro] Error fetching products from page %d: %v", page, err)
						return
					}
					for _, v := range products.Items {
						resultsChan <- []string{
							strings.ReplaceAll(v.Name, ",", "."),
							v.Ref,
							fmt.Sprintf("%.2f грн", v.Price/100),
							ci.Title,
							"metro",
						}
					}
				}(page)
			}
			pageWg.Wait()
		}(ci)
	}
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	for product := range resultsChan {
		result = append(result, product)
	}

	return result, nil
}

func (m *MetroScraper) getProductsFromPage(ctx context.Context, page int, slug string) (*MetroProducts, error) {
	p := utils.PrepareURLParams(map[string]string{
		"page": strconv.Itoa(page),
	})
	reqURL := fmt.Sprintf("%s/%s/products", metroBaseURL, slug)
	req, err := utils.MakeGetRequest(ctx, reqURL, m.Headers, p)
	if err != nil {
		return nil, fmt.Errorf("[Metro] error making HTTP request: %v", err)
	}
	fmt.Printf("[Metro] Getting products from: %s\n", req.URL)
	resp, err := m.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("[Metro] error getting request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("[Metro] bad status for %s: %s", reqURL, resp.Status)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("[Metro] error reading response body: %v", err)
	}
	var prd MetroProducts
	jsonErr := json.Unmarshal(respBody, &prd)
	if jsonErr != nil {
		return nil, fmt.Errorf("[Metro] error unmarshalling response body: %v", jsonErr)
	}

	return &prd, nil
}

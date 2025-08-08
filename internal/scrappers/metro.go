package scrappers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/MrPuls/groceries-price-aggregator-go/internal/utils"
)

type MetroScraper struct {
	Client    *http.Client
	CSVHeader []string
	Headers   map[string]string
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

func NewMetroClient() *MetroScraper {
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
		CSVHeader: CSVHeader,
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

func (m *MetroScraper) GetCategories() ([]MetroCategoryItem, error) {
	params := map[string]string{
		"only_parents": "true",
	}
	p := utils.PrepareURLParams(params)
	req, reqErr := utils.MakeGetRequest(MetroCategoriesURL, m.Headers, p)
	if reqErr != nil {
		return nil, reqErr
	}

	resp, respErr := m.Client.Do(req)
	if respErr != nil {
		return nil, respErr
	}

	bb, _ := io.ReadAll(resp.Body)
	err := resp.Body.Close()
	if err != nil {
		return nil, err
	}
	var c []MetroCategoryItem
	rb := json.Unmarshal(bb, &c)
	if rb != nil {
		return nil, rb
	}
	total := 0
	for _, v := range c {
		total += v.Total
	}
	fmt.Printf("Found %v categories with total amount of items: %v\n", len(c), total)

	return c, nil
}

// GetProducts TODO logging instead of fatal. And refactor according to https://gemini.google.com/app/d9366ea6ab5d45b3
func (m *MetroScraper) GetProducts(cts []MetroCategoryItem) ([][]string, error) {
	var result [][]string
	pageSize := 30
	var wg sync.WaitGroup
	var httpSemaphore = make(chan struct{}, 30)
	resultsChan := make(chan []string)

	go func() {
		for product := range resultsChan {
			result = append(result, product)
		}
	}()

	for _, ci := range cts {
		wg.Add(1)
		go func(ci MetroCategoryItem) {
			defer wg.Done()
			var pageWg sync.WaitGroup
			for page := 1; page <= (ci.Total/pageSize)+1; page++ {
				pageWg.Add(1)
				go func(page int) {
					httpSemaphore <- struct{}{}
					defer func() { <-httpSemaphore }()
					defer pageWg.Done()
					p := utils.PrepareURLParams(map[string]string{
						"page": strconv.Itoa(page),
					})
					reqUrl := fmt.Sprintf("%s/%s/products", MetroProductsURL, ci.Slug)
					req, reqErr := utils.MakeGetRequest(reqUrl, m.Headers, p)
					if reqErr != nil {
						log.Fatal(reqErr)
					}
					fmt.Printf("Request URL: %s\n", req.URL)
					resp, respErr := m.Client.Do(req)
					if respErr != nil {
						log.Fatal(respErr)
					}
					respBody, respBodyErr := io.ReadAll(resp.Body)
					if respBodyErr != nil {
						log.Fatal(respBodyErr)
					}
					err := resp.Body.Close()
					if err != nil {
						log.Fatal(err)
					}
					var prd MetroProducts
					rbJson := json.Unmarshal(respBody, &prd)
					if rbJson != nil {
						log.Fatal(rbJson)
					}
					if len(prd.Items) == 0 {
						return
					}
					for _, v := range prd.Items {
						resultsChan <- []string{
							v.Name,
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

	return result, nil
}

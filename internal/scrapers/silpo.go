package scrapers

import (
	"context"
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

const (
	silpoCategoriesURL      = "https://sf-ecom-api.silpo.ua/v1/branches/00000000-0000-0000-0000-000000000000/categories/tree"
	silpoCategoryDetailsURL = "https://sf-ecom-api.silpo.ua/v1/uk/branches/00000000-0000-0000-0000-000000000000/categories"
	silpoProductsURL        = "https://sf-ecom-api.silpo.ua/v1/uk/branches/00000000-0000-0000-0000-000000000000/products"
	silpoProductsQuerySize  = 100
	silpoSemaphoreSize      = 35
)

type SilpoScraper struct {
	Client    *http.Client
	CSVHeader []string
	Headers   map[string]string
}

type SilpoCategoryItem struct {
	Title string
	Slug  string `json:"slug"`
	Total int    `json:"total"`
}

type SilpoCategories struct {
	Total int64               `json:"total"`
	Items []SilpoCategoryItem `json:"items"`
}

type SilpoProduct struct {
	Name         string  `json:"title"`
	SectionSlug  string  `json:"sectionSlug"`
	DisplayPrice float64 `json:"displayPrice"`
	DisplayRatio string  `json:"displayRatio"`
}

type SilpoProducts struct {
	Total int            `json:"total"`
	Items []SilpoProduct `json:"items"`
}

func NewSilpoScraper() *SilpoScraper {
	return &SilpoScraper{
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
			"Accept":          "application/json",
			"Accept-Encoding": "utf-8",
			"Host":            "sf-ecom-api.silpo.ua",
			"Origin":          "https://silpo.ua",
			"Referer":         "https://silpo.ua/",
			"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:140.0) Gecko/20100101 Firefox/140.0",
			"Sec-Fetch-Mode":  "cors",
			"Sec-Fetch-Site":  "same-site",
			"Sec-Fetch-Dest":  "empty",
			"Sec-GPC":         "1",
			"TE":              "trailers",
			"Accept-Language": "en-GB,en;q=0.5",
		},
	}
}

func (s *SilpoScraper) GetCategories(ctx context.Context) (*SilpoCategories, error) {
	params := map[string]string{
		"deliveryType": "DeliveryHome",
		"depth":        "1",
	}
	reqParams := utils.PrepareURLParams(params)
	req, err := utils.MakeGetRequest(ctx, silpoCategoriesURL, s.Headers, reqParams)
	if err != nil {
		return nil, fmt.Errorf("[Silpo] error making GET Request: %v", err)
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("[Silpo] error getting response from Silpo: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("[Silpo] getting categories: status code %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("[Silpo] error reading response from Silpo: %v", err)
	}
	var c *SilpoCategories
	jsonErr := json.Unmarshal(body, &c)
	if jsonErr != nil {
		return nil, fmt.Errorf("[Silpo] error unmarshalling response from Silpo: %v", jsonErr)
	}
	var total int
	for _, v := range c.Items {
		total += v.Total
	}
	titlesErr := s.getCategoriesTitles(ctx, c)
	if titlesErr != nil {
		return nil, fmt.Errorf("[Silpo] error getting categories titles: %v", err)
	}
	fmt.Printf("[Silpo] Found %v categories with total amount of items: %v\n", c.Total, total)
	return c, nil
}

func (s *SilpoScraper) getCategoriesTitles(ctx context.Context, cts *SilpoCategories) error {
	var wg sync.WaitGroup
	for k, v := range cts.Items {
		wg.Add(1)
		go func(k int, v SilpoCategoryItem) {
			defer wg.Done()
			ctUrl := fmt.Sprintf("%s/%s", silpoCategoryDetailsURL, v.Slug)
			req, err := utils.MakeGetRequest(ctx, ctUrl, s.Headers, nil)
			if err != nil {
				fmt.Printf("[Silpo] error making GET Request: %v", err)
				return
			}
			resp, err := s.Client.Do(req)
			if err != nil {
				fmt.Printf("[Silpo] error getting response from Silpo: %v", err)
				return
			}
			if resp.StatusCode != http.StatusOK {
				fmt.Printf("[Silpo] getting categories: status code %d", resp.StatusCode)
				return
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				fmt.Printf("[Silpo] error reading response from Silpo: %v", err)
				return
			}
			closeErr := resp.Body.Close()
			if closeErr != nil {
				fmt.Printf("[Silpo] error closing response body from Silpo: %v", closeErr)
				return
			}
			var ci SilpoCategoryItem
			jsonErr := json.Unmarshal(body, &ci)
			if jsonErr != nil {
				fmt.Printf("[Silpo] error unmarshalling response from Silpo: %v", jsonErr)
			}
			cts.Items[k].Title = ci.Title
		}(k, v)
	}
	wg.Wait()
	return nil
}

func (s *SilpoScraper) GetProducts(ctx context.Context, cti []SilpoCategoryItem) ([][]string, error) {
	var result [][]string
	var wg sync.WaitGroup
	var httpSemaphore = make(chan struct{}, silpoSemaphoreSize)
	resultsChan := make(chan []string)
	for _, ci := range cti {
		wg.Add(1)
		go func(ci SilpoCategoryItem) {
			defer wg.Done()
			var offsetWg sync.WaitGroup
			for offset := 0; offset <= ci.Total; offset += silpoProductsQuerySize {
				offsetWg.Add(1)
				go func(offset int) {
					httpSemaphore <- struct{}{}
					defer func() { <-httpSemaphore }()
					defer offsetWg.Done()
					select {
					case <-ctx.Done():
						return
					default:
					}
					products, err := s.getProductsFromOffset(ctx, ci.Slug, offset)
					if err != nil {
						log.Printf("[Silpo] Error fetching products from offset %d: %v", offset, err)
						return
					}
					for _, v := range products.Items {
						resultsChan <- []string{
							v.Name,
							fmt.Sprintf("https://silpo.ua/product/%s", v.SectionSlug),
							fmt.Sprintf("%.2f грн/%s", v.DisplayPrice, v.DisplayRatio),
							ci.Title,
							"silpo",
						}
					}
				}(offset)
			}
			offsetWg.Wait()
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

func (s *SilpoScraper) getProductsFromOffset(ctx context.Context, slug string, offset int) (*SilpoProducts, error) {
	p := utils.PrepareURLParams(map[string]string{
		"deliveryType":           "DeliveryHome",
		"category":               slug,
		"includeChildCategories": "true",
		"sortBy":                 "popularity",
		"sortDirection":          "desc",
		"inStock":                "false",
		"limit":                  strconv.Itoa(silpoProductsQuerySize),
		"offset":                 strconv.Itoa(offset),
	})
	req, err := utils.MakeGetRequest(ctx, silpoProductsURL, s.Headers, p)
	if err != nil {
		return nil, fmt.Errorf("[Silpo] error making GET Request: %v", err)
	}
	fmt.Printf("[Silpo] Getting products from: %s\n", req.URL)
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("[Silpo] error getting response from Silpo: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("[Silpo] bad status for %s: %s", req.URL, resp.Status)
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("[Silpo] error reading response from Silpo: %v", err)
	}
	var prd SilpoProducts
	jsonErr := json.Unmarshal(respBody, &prd)
	if jsonErr != nil {
		return nil, fmt.Errorf("[Silpo] error unmarshalling response from Silpo: %v", jsonErr)
	}
	return &prd, nil
}

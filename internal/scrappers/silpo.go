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

func NewSilpoClient() *SilpoScraper {
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

func (s *SilpoScraper) GetCategories() (*SilpoCategories, error) {
	params := map[string]string{
		"deliveryType": "DeliveryHome",
		"depth":        "1",
	}
	reqParams := utils.PrepareURLParams(params)
	req, reqErr := utils.MakeGetRequest(SilpoCategoriesURL, s.Headers, reqParams)
	if reqErr != nil {
		return nil, reqErr
	}
	resp, respErr := s.Client.Do(req)
	if respErr != nil {
		return nil, respErr
	}
	bb, _ := io.ReadAll(resp.Body)
	err := resp.Body.Close()
	if err != nil {
		return nil, err
	}
	var c *SilpoCategories
	rb := json.Unmarshal(bb, &c)
	if rb != nil {
		return nil, rb
	}
	var total int
	for _, v := range c.Items {
		total += v.Total
	}
	fmt.Printf("Found %v categories with total amount of items: %v\n", c.Total, total)
	return c, nil
}

func (s *SilpoScraper) GetCategoriesTitles(cts *SilpoCategories) {
	for k, v := range cts.Items {
		ctUrl := fmt.Sprintf("%s/%s", SilpoCategoryDetailsURL, v.Slug)
		req, reqErr := utils.MakeGetRequest(ctUrl, s.Headers, nil)
		if reqErr != nil {
			log.Fatal(reqErr)
		}
		resp, err := s.Client.Do(req)
		if err != nil {
			panic(err)
		}
		bb, _ := io.ReadAll(resp.Body)
		bcErr := resp.Body.Close()
		if bcErr != nil {
			return
		}
		var ci SilpoCategoryItem
		rb := json.Unmarshal(bb, &ci)
		if rb != nil {
			panic(rb)
		}
		cts.Items[k].Title = ci.Title
	}
}
func (s *SilpoScraper) GetProducts(cti []SilpoCategoryItem) ([][]string, error) {
	querySize := 100
	mu := &sync.Mutex{}
	var result [][]string
	var wg sync.WaitGroup
	var httpSemaphore = make(chan struct{}, 25)
	resultsChan := make(chan []string)

	for _, ci := range cti {
		wg.Add(1)
		go func(ci SilpoCategoryItem) {
			defer wg.Done()
			params := map[string]string{
				"deliveryType":           "DeliveryHome",
				"category":               ci.Slug,
				"includeChildCategories": "true",
				"sortBy":                 "popularity",
				"sortDirection":          "desc",
				"inStock":                "false",
				"limit":                  strconv.Itoa(querySize),
				"offset":                 strconv.Itoa(0),
			}
			fmt.Println("Fetching products for", ci.Slug)
			var offsetWg sync.WaitGroup
			for offset := 0; offset <= ci.Total; offset += querySize {
				offsetWg.Add(1)
				go func(offset int) {
					httpSemaphore <- struct{}{}
					defer func() { <-httpSemaphore }()
					defer offsetWg.Done()
					params["offset"] = strconv.Itoa(offset)
					mu.Lock()
					p := utils.PrepareURLParams(params)
					mu.Unlock()
					req, reqErr := utils.MakeGetRequest(SilpoProductsURL, s.Headers, p)
					if reqErr != nil {
						log.Fatal(reqErr)
					}
					fmt.Printf("Request URL: %s\n", req.URL)
					resp, respErr := s.Client.Do(req)
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
					var prd SilpoProducts
					rbJson := json.Unmarshal(respBody, &prd)
					if rbJson != nil {
						log.Fatal(rbJson)
					}
					for _, v := range prd.Items {
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

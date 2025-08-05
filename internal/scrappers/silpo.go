package scrappers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

type SilpoScraper struct {
	Client    *http.Client
	CSVHeader []string
}

type CategoryItem struct {
	Title string
	Slug  string `json:"slug"`
	Total int    `json:"total"`
}

type Categories struct {
	Total int64          `json:"total"`
	Items []CategoryItem `json:"items"`
}

type ProductDetails struct {
	Title        string  `json:"title"`
	SectionSlug  string  `json:"sectionSlug"`
	DisplayPrice float64 `json:"displayPrice"`
	DisplayRatio string  `json:"displayRatio"`
}

type Product struct {
	Total int              `json:"total"`
	Items []ProductDetails `json:"items"`
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
	}
}

func (s *SilpoScraper) makeRequest(reqUrl string, params map[string]string) (resp *http.Response, err error) {
	if params != nil {
		p := url.Values{}
		for k, v := range params {
			p.Add(k, v)
		}
		queryString := p.Encode()

		reqUrl = fmt.Sprintf("%s?%s", reqUrl, queryString)
	}

	req, err := http.NewRequest("GET", reqUrl, nil)
	if err != nil {
		panic(err)
	}

	req.Header.Add("Accept", "application/json")
	req.Header.Add("Accept-Encoding", "utf-8")
	req.Host = "sf-ecom-api.scrappers.ua"
	req.Header.Add("Origin", "https://silpo.ua")
	req.Header.Add("Referer", "https://silpo.ua/")
	req.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:140.0) Gecko/20100101 Firefox/140.0")
	req.Header.Add("Sec-Fetch-Mode", "cors")
	req.Header.Add("Sec-Fetch-Site", "same-site")
	req.Header.Add("Sec-Fetch-Dest", "empty")
	req.Header.Add("Sec-GPC", "1")
	req.Header.Add("TE", "trailers")
	req.Header.Add("Accept-Language", "en-GB,en;q=0.5")
	fmt.Printf("Requesting: %s\n", reqUrl)
	resp, respErr := s.Client.Do(req)
	if respErr != nil {
		return nil, respErr
	}

	return resp, nil
}

func (s *SilpoScraper) GetCategories() (*Categories, error) {
	params := map[string]string{
		"deliveryType": "DeliveryHome",
		"depth":        "1",
	}
	resp, respErr := s.makeRequest(SilpoCategoriesURL, params)
	if respErr != nil {
		panic(respErr)
	}
	bb, _ := io.ReadAll(resp.Body)
	err := resp.Body.Close()
	if err != nil {
		return nil, err
	}
	var c *Categories
	rb := json.Unmarshal(bb, &c)
	if rb != nil {
		panic(rb)
	}
	var total int
	for _, v := range c.Items {
		total += v.Total
	}
	fmt.Printf("Found %v categories with total amount of items: %v\n", c.Total, total)
	return c, nil
}

func (s *SilpoScraper) GetCategoriesTitles(cts *Categories) {
	for k, v := range cts.Items {
		ctUrl := fmt.Sprintf("%s%s", SilpoCategoryDetailsURL, v.Slug)
		resp, err := s.makeRequest(ctUrl, nil)
		if err != nil {
			panic(err)
		}
		bb, _ := io.ReadAll(resp.Body)
		bcErr := resp.Body.Close()
		if bcErr != nil {
			return
		}
		var ci CategoryItem
		rb := json.Unmarshal(bb, &ci)
		if rb != nil {
			panic(rb)
		}
		cts.Items[k].Title = ci.Title
	}
}
func (s *SilpoScraper) GetProducts(cti []CategoryItem) ([][]string, error) {
	querySize := 100
	mu := sync.Mutex{}
	var result [][]string
	var wg sync.WaitGroup
	var httpSemaphore = make(chan struct{}, 35)
	resultsChan := make(chan []string)

	for _, ci := range cti {
		wg.Add(1)
		go func(ci CategoryItem) {
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
					httpSemaphore <- struct{}{} // Acquire
					defer func() { <-httpSemaphore }()
					defer offsetWg.Done()
					mu.Lock()
					params["offset"] = strconv.Itoa(offset)
					p := url.Values{}
					for k, v := range params {
						p.Add(k, v)
					}
					queryString := p.Encode()
					mu.Unlock()

					reqUrl := fmt.Sprintf("%s?%s", SilpoProductsURL, queryString)
					resp, respErr := s.makeRequest(reqUrl, nil)
					if respErr != nil {
						panic(respErr)
					}
					respBody, respBodyErr := io.ReadAll(resp.Body)
					if respBodyErr != nil {
						panic(respBodyErr)
					}
					err := resp.Body.Close()
					if err != nil {
						return
					}
					var prd Product
					rbJson := json.Unmarshal(respBody, &prd)
					if rbJson != nil {
						panic(rbJson)
					}
					for _, v := range prd.Items {
						resultsChan <- []string{
							v.Title,
							fmt.Sprintf("https://silpo.ua/product/%s", v.SectionSlug),
							fmt.Sprintf("%.2f грн/%s", v.DisplayPrice, v.DisplayRatio),
							ci.Title,
							"scrappers",
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

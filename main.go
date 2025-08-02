package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"
)

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

var client = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     75,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableKeepAlives:   false,
	},
}

func makeRequest(cli *http.Client, reqUrl string, params map[string]string) (resp *http.Response, err error) {
	// TODO: create client as a type method
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
	req.Host = "sf-ecom-api.silpo.ua"
	req.Header.Add("Origin", "https://silpo.ua")
	req.Header.Add("Referer", "https://silpo.ua/")
	req.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:140.0) Gecko/20100101 Firefox/140.0")
	req.Header.Add("Sec-Fetch-Mode", "cors")
	req.Header.Add("Sec-Fetch-Site", "same-site")
	req.Header.Add("Sec-Fetch-Dest", "empty")
	req.Header.Add("Sec-GPC", "1")
	req.Header.Add("TE", "trailers")
	req.Header.Add("Accept-Language", "en-GB,en;q=0.5")
	fmt.Printf("reqUrl: %s\n", reqUrl)
	resp, respErr := cli.Do(req)
	if respErr != nil {
		return nil, respErr
	}

	return resp, nil
}

func (c *Categories) getCategories() {
	categoriesUrl := "https://sf-ecom-api.silpo.ua/v1/branches/00000000-0000-0000-0000-000000000000/categories/tree"
	params := map[string]string{
		"deliveryType": "DeliveryHome",
		"depth":        "1",
	}
	resp, respErr := makeRequest(client, categoriesUrl, params)
	if respErr != nil {
		panic(respErr)
	}
	bb, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	rb := json.Unmarshal(bb, &c)
	if rb != nil {
		panic(rb)
	}
	var total int
	for _, v := range c.Items {
		total += v.Total
	}
	fmt.Printf("Found %v categories with total amount of items: %v\n", c.Total, total)
}

func (c *Categories) getCategoriesTitles() {
	categoryDetailsUrl := "https://sf-ecom-api.silpo.ua/v1/uk/branches/00000000-0000-0000-0000-000000000000/categories/"
	for k, v := range c.Items {
		ctUrl := fmt.Sprintf("%s%s", categoryDetailsUrl, v.Slug)
		resp, err := makeRequest(client, ctUrl, nil)
		if err != nil {
			panic(err)
		}
		bb, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var ci CategoryItem
		rb := json.Unmarshal(bb, &ci)
		if rb != nil {
			panic(rb)
		}
		c.Items[k].Title = ci.Title
	}
}
func getProducts(cti []CategoryItem) ([][]string, error) {
	productsUrl := "https://sf-ecom-api.silpo.ua/v1/uk/branches/00000000-0000-0000-0000-000000000000/products"
	querySize := 100
	mu := sync.Mutex{}
	var result [][]string
	var wg sync.WaitGroup
	var httpSemaphore = make(chan struct{}, 25)
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

					reqUrl := fmt.Sprintf("%s?%s", productsUrl, queryString)
					resp, respErr := makeRequest(client, reqUrl, nil)
					if respErr != nil {
						panic(respErr)
					}
					respBody, respBodyErr := io.ReadAll(resp.Body)
					if respBodyErr != nil {
						panic(respBodyErr)
					}
					resp.Body.Close()
					var prd Product
					rbjson := json.Unmarshal(respBody, &prd)
					if rbjson != nil {
						panic(rbjson)
					}
					for _, v := range prd.Items {
						resultsChan <- []string{
							v.Title,
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

func main() {
	log.Println("Starting main program")
	cts := Categories{}
	cts.getCategories()
	cts.getCategoriesTitles()

	file, err := os.Create("output.csv")
	if err != nil {
		log.Fatal("Error creating file:", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{"Name", "Ref", "Price", "Category", "Shop"}

	if err = writer.Write(header); err != nil {
		log.Fatal("Error writing record to CSV:", err)
	}

	products, prErr := getProducts(cts.Items)
	if prErr != nil {
		log.Fatal(prErr)
	}

	if err := writer.WriteAll(products); err != nil {
		log.Fatal("Error writing record to CSV:", err)
	}

	log.Println("Data successfully written to output.csv")
}

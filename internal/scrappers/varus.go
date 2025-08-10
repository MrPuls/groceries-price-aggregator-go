package scrappers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/MrPuls/groceries-price-aggregator-go/internal/utils"
)

type VarusScraper struct {
	Client    *http.Client
	CSVHeader []string
	Headers   map[string]string
}

type VarusCategoryItem struct {
	Slug        string `json:"link"`
	CategoryIds []int  `json:"category_ids"`
	Total       int
}

type VarusCategories struct {
	Items []VarusCategoryItem `json:"hits"`
}

type VarusProductPriceDetails struct {
	Price float64 `json:"price"`
}

type VarusProductTotalDetails struct {
	Total int `json:"value"`
}

type VarusProductsTotal struct {
	Total VarusProductTotalDetails `json:"total"`
}

type VarusProduct struct {
	Name  string                   `json:"name"`
	Ref   string                   `json:"url_key"`
	Price VarusProductPriceDetails `json:"sqpp_data_region_default"`
}

type VarusProducts struct {
	Items []VarusProduct `json:"hits"`
}

func NewVarusClient() *VarusScraper {
	return &VarusScraper{
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
			"Host":            "varus.ua",
			"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:141.0) Gecko/20100101 Firefox/141.0",
			"Accept":          "application/json",
			"Accept-Language": "en-GB,en;q=0.5",
			"Accept-Encoding": "utf-8",
			"Referer":         "https://varus.ua/",
			"Content-Type":    "application/json",
			"Sec-GPC":         "1",
			"Connection":      "keep-alive",
			"Sec-Fetch-Dest":  "empty",
			"Sec-Fetch-Mode":  "cors",
			"Sec-Fetch-Site":  "same-origin",
			"Priority":        "u=4",
			"TE":              "trailers",
		},
	}
}

func (v *VarusScraper) GetCategories() (*VarusCategories, error) {
	requestData := map[string]interface{}{
		"_availableFilters": []string{},
		"_appliedFilters": []map[string]interface{}{
			{"attribute": "datetime_from", "value": map[string]string{"lt": "now"}, "scope": "default"},
			{"attribute": "datetime_to", "value": map[string]string{"gt": "now-1d"}, "scope": "default"},
			{"attribute": "status", "value": map[string]string{"eq": "1"}, "scope": "default"},
			{"attribute": "position", "value": map[string]string{"eq": "3"}, "scope": "default"},
		},
		"_appliedSort": []string{},
		"_searchText":  "",
	}
	marshaledData, mErr := json.Marshal(requestData)
	if mErr != nil {
		return nil, mErr
	}

	categoriesParams := map[string]string{
		"_source_exclude": "tms,tsk,sgn,paths,created_time,update_time",
		"from":            "0",
		"request":         string(marshaledData),
		"request_format":  "search-query",
		"response_format": "compact",
		"size":            "50",
		"sort":            "",
	}

	p := utils.PrepareURLParams(categoriesParams)
	req, reqErr := utils.MakeGetRequest(VarusCategoriesURL, v.Headers, p)
	if reqErr != nil {
		return nil, reqErr
	}

	resp, respErr := v.Client.Do(req)
	if respErr != nil {
		return nil, respErr
	}

	bb, _ := io.ReadAll(resp.Body)
	err := resp.Body.Close()
	if err != nil {
		return nil, err
	}
	var c *VarusCategories
	rb := json.Unmarshal(bb, &c)
	if rb != nil {
		return nil, rb
	}
	return c, nil
}

func (v *VarusScraper) GetProducts(cts *VarusCategories) ([][]string, error) {
	var result [][]string
	var wg sync.WaitGroup
	var httpSemaphore = make(chan struct{}, 25)
	resultsChan := make(chan []string)
	querySize := 100

	for _, ci := range cts.Items {
		wg.Add(1)
		// TODO: My brother in Christ, this is truly diabolical. this must be refactored for real alongside with other scrapers
		p := utils.PrepareURLParams(map[string]string{
			"_source_exclude": "",
			"_source_include": "brand_data.name,description,category,category_ids,stock.is_in_stock,forNewPost,stock.qty," +
				"stock.max,stock.manage_stock,stock.is_qty_decimal,sku,id,name,image,regular_price," +
				"special_price_discount,special_price_to_date,slug,url_key,url_path,product_label," +
				"type_id,volume,weight,wghweigh,packingtype,is_new,is_18_plus,news_from_date,news_to_date," +
				"varus_perfect,productquantityunit,productquantityunitstep,productminsalablequantity," +
				"productquantitysteprecommended,markdown_id,markdown_title,markdown_discount," +
				"markdown_description,online_promotion_in_stores,boardProduct,fv_image_timestamp,sqpp_data_region_default",
			"from":            "0",
			"request_format":  "search-query",
			"response_format": "compact",
			"shop_id":         "3",
			"size":            strconv.Itoa(querySize),
			"sort":            "",
		})

		foo := map[string]any{
			"_availableFilters": []map[string]any{
				{"field": "pim_brand_id", "scope": "catalog", "options": map[string]any{}},
				{"field": "countrymanufacturerforsite", "scope": "catalog", "options": map[string]any{}},
				{"field": "promotion_banner_ids", "scope": "catalog", "options": map[string]any{}},
				{"field": "price", "scope": "catalog", "options": map[string]any{"shop_id": 3, "version": "2"}},
				{"field": "has_promotion_in_stores", "scope": "catalog", "options": map[string]any{"size": 10000}},
				{"field": "markdown_id", "scope": "catalog", "options": map[string]any{}}},
			"_appliedFilters": []map[string]any{{"attribute": "visibility",
				"value": map[string]any{"in": []int{2, 4}}, "scope": "default"},
				{"attribute": "status", "value": map[string]any{"in": []int{0, 1}},
					"scope": "default"}, {"attribute": "category_ids",
					"value": map[string]any{"in": ci.CategoryIds}, "scope": "default"},
				{"attribute": "markdown_id", "value": map[string]any{"or": nil}, "scope": "default"},
				{"attribute": "sqpp_data_3.in_stock", "value": map[string]any{"or": true}, "scope": "default"},
				{"attribute": "markdown_id", "value": map[string]any{"nin": nil}, "scope": "default"}},
			"_appliedSort": []map[string]any{
				{"field": "_script",
					"options": map[string]any{"type": "number",
						"order": "desc",
						"script": map[string]any{
							"lang":   "painless",
							"source": "\nint score = 0;\n\nscore = doc['sqpp_data_region_default.availability.shipping'].value ? 2 : score;\nscore = doc['sqpp_data_region_default.availability.other_regions'].value ? 2 : score;\nscore = doc['sqpp_data_region_default.availability.pickup'].value ? 2 : score;\nscore = doc['sqpp_data_region_default.availability.other_market'].value ? 2 : score;\nscore = doc['sqpp_data_region_default.availability.delivery'].value ? 4: score;\n\nscore += doc['sqpp_data_region_default.in_stock'].value ? 1 : 0;\n\nif (doc.containsKey('markdown_id') && !doc['markdown_id'].empty && score > 2) {\n    score = 3;\n}\n\nreturn score;\n"}}}, {"field": "category_position_2", "options": map[string]any{"order": "desc"}}, {"field": "sqpp_score", "options": map[string]any{"order": "desc"}}}, "_searchText": ""}
		bar, _ := json.Marshal(foo)
		enc := url.QueryEscape(string(bar))

		reqUr := fmt.Sprintf("%s?%s&request=%s", VarusProductsURL, p.Encode(), enc)
		req, err := http.NewRequest("GET", reqUr, nil)
		if err != nil {
			panic(err)
		}
		resp, respErr := v.Client.Do(req)
		if respErr != nil {
			log.Fatal(respErr)
		}
		respBody, respBodyErr := io.ReadAll(resp.Body)
		if respBodyErr != nil {
			log.Fatal(respBodyErr)
		}
		bcErr := resp.Body.Close()
		if bcErr != nil {
			log.Fatal(bcErr)
		}
		var cti VarusProductsTotal
		rbJson := json.Unmarshal(respBody, &cti)
		ci.Total = cti.Total.Total
		if rbJson != nil {
			log.Fatal(rbJson)
		}
		go func(ci VarusCategoryItem) {
			defer wg.Done()
			fmt.Println("Fetching products for", ci.Slug)
			var offsetWg sync.WaitGroup
			for offset := 0; offset <= ci.Total; offset += querySize {
				offsetWg.Add(1)
				go func(offset int) {
					httpSemaphore <- struct{}{}
					defer func() { <-httpSemaphore }()
					defer offsetWg.Done()
					gp := utils.PrepareURLParams(map[string]string{
						"_source_exclude": "",
						"_source_include": "brand_data.name,description,category,category_ids,stock.is_in_stock,forNewPost,stock.qty," +
							"stock.max,stock.manage_stock,stock.is_qty_decimal,sku,id,name,image,regular_price," +
							"special_price_discount,special_price_to_date,slug,url_key,url_path,product_label," +
							"type_id,volume,weight,wghweigh,packingtype,is_new,is_18_plus,news_from_date,news_to_date," +
							"varus_perfect,productquantityunit,productquantityunitstep,productminsalablequantity," +
							"productquantitysteprecommended,markdown_id,markdown_title,markdown_discount," +
							"markdown_description,online_promotion_in_stores,boardProduct,fv_image_timestamp,sqpp_data_region_default",
						"from":            strconv.Itoa(offset),
						"request_format":  "search-query",
						"response_format": "compact",
						"shop_id":         "3",
						"size":            strconv.Itoa(querySize),
						"sort":            "",
					})
					gbar, _ := json.Marshal(foo)
					genc := url.QueryEscape(string(gbar))

					greqUr := fmt.Sprintf("%s?%s&request=%s", VarusProductsURL, gp.Encode(), genc)
					greq, gerr := http.NewRequest("GET", greqUr, nil)
					if gerr != nil {
						panic(gerr)
					}

					gresp, grespErr := v.Client.Do(greq)
					if grespErr != nil {
						log.Fatal(grespErr)
					}
					grespBody, grespBodyErr := io.ReadAll(gresp.Body)
					if grespBodyErr != nil {
						log.Fatal(grespBodyErr)
					}
					gbcErr := gresp.Body.Close()
					if gbcErr != nil {
						log.Fatal(gbcErr)
					}
					var prd VarusProducts
					grbJson := json.Unmarshal(grespBody, &prd)
					if grbJson != nil {
						log.Fatal(grbJson)
					}
					for _, i := range prd.Items {
						resultsChan <- []string{
							i.Name,
							fmt.Sprintf("https://varus.ua/%s", i.Ref),
							fmt.Sprintf("%.2f грн", i.Price.Price),
							ci.Slug,
							"varus",
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

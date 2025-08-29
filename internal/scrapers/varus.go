package scrapers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MrPuls/groceries-price-aggregator-go/internal/utils"
)

const (
	varusCategoriesURL = "https://varus.ua/api/catalog/vue_storefront_catalog_2/banner/_search"
	varusProductsURL   = "https://varus.ua/api/catalog/vue_storefront_catalog_2/product_v2/_search"
	varusQuerySize     = 100
	varusSemaphoreSize = 35
)

var requestParams = map[string]string{
	"_source_exclude": "",
	"_source_include": "brand_data.name,description,category,category_ids,stock.is_in_stock,forNewPost,stock.qty," +
		"stock.max,stock.manage_stock,stock.is_qty_decimal,sku,id,name,image,regular_price," +
		"special_price_discount,special_price_to_date,slug,url_key,url_path,product_label," +
		"type_id,volume,weight,wghweigh,packingtype,is_new,is_18_plus,news_from_date,news_to_date," +
		"varus_perfect,productquantityunit,productquantityunitstep,productminsalablequantity," +
		"productquantitysteprecommended,markdown_id,markdown_title,markdown_discount," +
		"markdown_description,online_promotion_in_stores,boardProduct,fv_image_timestamp,sqpp_data_region_default",
	"from":            "",
	"request_format":  "search-query",
	"response_format": "compact",
	"shop_id":         "3",
	"size":            "",
	"sort":            "",
}

type VarusScraper struct {
	Client  *http.Client
	Headers map[string]string
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
	Value int `json:"value"`
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

func NewVarusScraper() *VarusScraper {
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

func (v *VarusScraper) GetCategories(ctx context.Context) (*VarusCategories, error) {
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
	marshaledData, err := json.Marshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("[Varus] error marshalling request data: %v", err)
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
	req, err := utils.MakeGetRequest(ctx, varusCategoriesURL, v.Headers, p)
	if err != nil {
		return nil, fmt.Errorf("[Varus] error making GET Request: %v", err)
	}

	resp, err := v.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("[Varus] error getting response from Varus: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("[Varus] getting categories: status code %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("[Varus] error reading response from Varus: %v", err)
	}
	var c *VarusCategories
	jsonErr := json.Unmarshal(body, &c)
	if jsonErr != nil {
		return nil, fmt.Errorf("[Varus] error unmarshalling response from Varus: %v", jsonErr)
	}
	return c, nil
}

func (v *VarusScraper) GetProductsTotalValues(ctx context.Context, cts *VarusCategories) error {
	var wg sync.WaitGroup
	totalProducts := 0
	for k, ci := range cts.Items {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			default:
			}
			req, err := v.buildProductsRequest(ctx, ci.CategoryIds, 0)
			if err != nil {
				fmt.Printf("[Varus] error building products request: %v", err)
			}
			productsTotalErr := v.getProductsTotal(req, &cts.Items[k])
			if productsTotalErr != nil {
				fmt.Printf("[Varus] error getting products total: %v", productsTotalErr)
			}
			totalProducts += cts.Items[k].Total
		}()
	}
	wg.Wait()
	fmt.Printf("[Varus] Total products: %d\n", totalProducts)
	return nil
}

func (v *VarusScraper) GetProducts(ctx context.Context, cts *VarusCategories) ([][]string, error) {
	var result [][]string
	var wg sync.WaitGroup
	var httpSemaphore = make(chan struct{}, varusSemaphoreSize)
	resultsChan := make(chan []string)
	for _, ci := range cts.Items {
		wg.Add(1)
		go func(ci VarusCategoryItem) {
			defer wg.Done()
			var offsetWg sync.WaitGroup
			fmt.Printf("Fetching {%v} products for: %s\n", ci.Total, ci.Slug)
			for offset := 0; offset <= ci.Total; offset += varusQuerySize {
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
					request, err := v.buildProductsRequest(ctx, ci.CategoryIds, offset)
					if err != nil {
						fmt.Printf("[Varus] error building products request: %v", err)
					}
					resp, err := v.Client.Do(request)
					if err != nil {
						fmt.Printf("[Varus] error getting resp from Varus: %v", err)
					}
					defer func() { _ = resp.Body.Close() }()
					if resp.StatusCode != http.StatusOK {
						fmt.Printf("[Varus] getting categories: status code %d", resp.StatusCode)
					}
					body, err := io.ReadAll(resp.Body)
					if err != nil {
						fmt.Printf("[Varus] error reading resp from Varus: %v", err)
					}
					var prd VarusProducts
					jsonErr := json.Unmarshal(body, &prd)
					if jsonErr != nil {
						fmt.Printf("[Varus] error unmarshalling resp from Varus: %v", jsonErr)
					}
					for _, i := range prd.Items {
						resultsChan <- []string{
							strings.ReplaceAll(i.Name, ",", "."),
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

func (v *VarusScraper) getProductsTotal(req *http.Request, category *VarusCategoryItem) error {
	resp, err := v.Client.Do(req)
	if err != nil {
		return fmt.Errorf("[Varus] error getting response from Varus: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("[Varus] getting categories: status code %d", resp.StatusCode)
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("[Varus] error reading response from Varus: %v", err)
	}
	var cti VarusProductsTotal
	jsonErr := json.Unmarshal(respBody, &cti)
	category.Total = cti.Total.Value
	if jsonErr != nil {
		return fmt.Errorf("[Varus] error unmarshalling response from Varus: %v", jsonErr)
	}
	return nil
}

func (v *VarusScraper) buildProductsRequest(ctx context.Context, categories []int, offset int) (*http.Request, error) {
	params := maps.Clone(requestParams)
	params["from"] = strconv.Itoa(offset)
	params["size"] = strconv.Itoa(varusQuerySize)
	urlParams := utils.PrepareURLParams(params)

	requestQuery := map[string]any{
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
				"value": map[string]any{"in": categories}, "scope": "default"},
			{"attribute": "markdown_id", "value": map[string]any{"or": nil}, "scope": "default"},
			{"attribute": "sqpp_data_3.in_stock", "value": map[string]any{"or": true}, "scope": "default"},
			{"attribute": "markdown_id", "value": map[string]any{"nin": nil}, "scope": "default"}},
		"_appliedSort": []map[string]any{
			{"field": "_script",
				"options": map[string]any{"type": "number",
					"order": "desc",
					"script": map[string]any{
						"lang": "painless",
						"source": `
							int score = 0;
							score = doc['sqpp_data_region_default.availability.shipping'].value ? 2 : score;
							score = doc['sqpp_data_region_default.availability.other_regions'].value ? 2 : score;
							score = doc['sqpp_data_region_default.availability.pickup'].value ? 2 : score;
							score = doc['sqpp_data_region_default.availability.other_market'].value ? 2 : score;
							score = doc['sqpp_data_region_default.availability.delivery'].value ? 4: score;
							
							score += doc['sqpp_data_region_default.in_stock'].value ? 1 : 0;
							
							if (doc.containsKey('markdown_id') && !doc['markdown_id'].empty && score > 2) {
								score = 3;
							}
							
							return score;
						`,
					},
				},
			},
			{"field": "category_position_2", "options": map[string]any{"order": "desc"}},
			{"field": "sqpp_score", "options": map[string]any{"order": "desc"}},
		},
		"_searchText": "",
	}
	rqJson, err := json.Marshal(requestQuery)
	if err != nil {
		return nil, fmt.Errorf("[Varus] error marshalling request data: %v", err)
	}
	escapedQuery := url.QueryEscape(string(rqJson))
	reqURL := fmt.Sprintf("%s?%s&request=%s", varusProductsURL, urlParams.Encode(), escapedQuery)
	req, err := utils.MakeGetRequest(ctx, reqURL, v.Headers, nil)
	if err != nil {
		return nil, fmt.Errorf("[Varus] error making GET Request: %v", err)
	}
	return req, nil
}

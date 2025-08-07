package scrappers

import (
	"encoding/json"
	"io"
	"net/http"
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
}

type VarusCategories struct {
	Items []VarusCategoryItem `json:"hits"`
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
			"Accept":       "application/json",
			"Content-Type": "application/json",
			"User-Agent":   "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:140.0) Gecko/20100101 Firefox/140.0",
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

func (v *VarusScraper) GetProducts(cts VarusCategories) ([][]string, error) {
	var result [][]string
	pageSize := 30
	var wg sync.WaitGroup
	mu := &sync.Mutex{}
	var httpSemaphore = make(chan struct{}, 20)
	resultsChan := make(chan []string)
	querySize := 100
	offset := 0

	// My brother in Christ, this is truly diabolical

	productsRequestData := map[string]interface{}{
		"_availableFilters": []map[string]interface{}{
			{"field": "pim_brand_id", "scope": "catalog", "options": map[string]string{}},
			{"field": "countrymanufacturerforsite", "scope": "catalog", "options": map[string]string{}},
			{"field": "promotion_banner_ids", "scope": "catalog", "options": map[string]string{}},
			{"field": "price", "scope": "catalog", "options": map[string]interface{}{"shop_id": 3, "version": "2"}},
			{"field": "has_promotion_in_stores", "scope": "catalog", "options": map[string]int{"size": 10000}},
			{"field": "markdown_id", "scope": "catalog", "options": map[string]string{}},
		},
		"_appliedFilters": []map[string]interface{}{
			{"attribute": "visibility", "value": map[string]interface{}{"in": []int{2, 4}}, "scope": "default"},
			{"attribute": "status", "value": map[string]interface{}{"in": []int{0, 1}}, "scope": "default"},
			{
				"attribute": "category_ids", "value": map[string]interface{}{
					"in": category_ids,
				},
				"scope": "default"},
			{"attribute": "markdown_id", "value": map[string]interface{}{"or": nil}, "scope": "default"},
			{"attribute": "sqpp_data_3.in_stock", "value": map[string]interface{}{"or": true}, "scope": "default"},
			{"attribute": "markdown_id", "value": map[string]interface{}{"nin": true}, "scope": "default"},
		},
		"_appliedSort": []map[string]interface{}{
			{
				"field": "_script",
				"options": map[string]interface{}{
					"type":  "number",
					"order": "desc",
					"script": map[string]string{
						"lang": "painless",
						"source": "\nint score = 0;\n\nscore = doc['sqpp_data_region_default.availability.shipping'].value ?" +
							" 2 : score;\nscore = doc['sqpp_data_region_default.availability.other_regions'].value ?" +
							" 2 : score;\nscore = doc['sqpp_data_region_default.availability.pickup'].value ?" +
							" 2 : score;\nscore = doc['sqpp_data_region_default.availability.other_market'].value ?" +
							" 2 : score;\nscore = doc['sqpp_data_region_default.availability.delivery'].value ?" +
							" 4: score;\n\nscore += doc['sqpp_data_region_default.in_stock'].value ? 1 : 0;" +
							"\n\nif (doc.containsKey('markdown_id') && !doc['markdown_id'].empty && score > 2) " +
							"{\n score = 3;\n}\n\nreturn score;\n",
					},
				},
			},
			{"field": "category_position_2", "options": map[string]string{"order": "desc"}},
			{"field": "sqpp_score", "options": map[string]string{"order": "desc"}},
		},
		"_searchText": "",
	}
	marshaledData, mErr := json.Marshal(productsRequestData)
	if mErr != nil {
		return nil, mErr
	}

	productsParams := map[string]string{
		"_source_exclude": "",
		"_source_include": "brand_data.name,description,category,category_ids,stock.is_in_stock,forNewPost,stock.qty," +
			"stock.max,stock.manage_stock,stock.is_qty_decimal,sku,id,name,image,regular_price," +
			"special_price_discount,special_price_to_date,slug,url_key,url_path,product_label," +
			"type_id,volume,weight,wghweigh,packingtype,is_new,is_18_plus,news_from_date,news_to_date," +
			"varus_perfect,productquantityunit,productquantityunitstep,productminsalablequantity," +
			"productquantitysteprecommended,markdown_id,markdown_title,markdown_discount," +
			"markdown_description,online_promotion_in_stores,boardProduct,fv_image_timestamp,sqpp_data_region_default",
		"from":            strconv.Itoa(offset),
		"request":         string(marshaledData),
		"request_format":  "search-query",
		"response_format": "compact",
		"shop_id":         "3",
		"size":            strconv.Itoa(querySize),
		"sort":            "",
	}
}

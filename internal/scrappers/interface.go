package scrappers

import (
	"net/http"
)

type Scrapper interface {
	makeRequest(reqUrl string, params map[string]string) (resp *http.Response, err error)
	GetProducts(cti []CategoryItem) ([][]string, error)
	GetCategories() (*Categories, error)
}

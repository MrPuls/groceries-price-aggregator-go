package scrapers

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MrPuls/groceries-price-aggregator-go/internal/utils"
	"golang.org/x/net/html"
)

const (
	atbBaseUrl       = "https://www.atbmarket.com"
	atbSemaphoreSize = 35
)

type AtbScraper struct {
	Client  *http.Client
	Headers map[string]string
}

func NewAtbScraper() *AtbScraper {
	return &AtbScraper{
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
			"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:140.0) Gecko/20100101 Firefox/140.0",
			"Accept":          "*/*",
			"Accept-Encoding": "utf-8",
			"Sec-Fetch-Mode":  "cors",
			"Sec-Fetch-Site":  "same-site",
			"Sec-Fetch-Dest":  "empty",
			"Sec-GPC":         "1",
			"TE":              "trailers",
			"Accept-Language": "en-GB,en;q=0.5",
			"Connection":      "keep-alive",
			"Host":            "www.atbmarket.com",
			"Referer":         "https://www.atbmarket.com/",
		},
	}
}

func (a *AtbScraper) getHTML(ctx context.Context, url string) (*html.Node, error) {
	req, err := utils.MakeGetRequest(ctx, url, a.Headers, nil)
	if err != nil {
		return nil, err
	}

	resp, err := a.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("[Metro] getting categories: status code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	return doc, nil
}

func findNodeByClass(n *html.Node, tag, class string) *html.Node {
	if n.Type == html.ElementNode && n.Data == tag {
		for _, attr := range n.Attr {
			if attr.Key == "class" && strings.Contains(attr.Val, class) {
				return n
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if result := findNodeByClass(c, tag, class); result != nil {
			return result
		}
	}
	return nil
}

func findAllNodesByClass(n *html.Node, tag, class string) []*html.Node {
	var results []*html.Node

	var traverse func(*html.Node)
	traverse = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == tag {
			for _, attr := range node.Attr {
				if attr.Key == "class" && strings.Contains(attr.Val, class) {
					results = append(results, node)
					break
				}
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}

	traverse(n)
	return results
}

func getTextContent(n *html.Node) string {
	if n == nil {
		return ""
	}

	if n.Type == html.TextNode {
		return strings.TrimSpace(n.Data)
	}

	var text strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		text.WriteString(getTextContent(c))
	}
	return strings.TrimSpace(text.String())
}

func findHref(n *html.Node) string {
	if n == nil {
		return ""
	}

	if n.Type == html.ElementNode && n.Data == "a" {
		for _, attr := range n.Attr {
			if attr.Key == "href" {
				return attr.Val
			}
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if href := findHref(c); href != "" {
			return href
		}
	}
	return ""
}

func findAttrValue(n *html.Node, tag, class, attr string) string {
	if n == nil {
		return ""
	}

	if n.Type == html.ElementNode && n.Data == tag {
		hasClass := false
		var attrValue string

		for _, a := range n.Attr {
			if a.Key == "class" && strings.Contains(a.Val, class) {
				hasClass = true
			}
			if a.Key == attr {
				attrValue = a.Val
			}
		}

		if hasClass && attrValue != "" {
			return attrValue
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if result := findAttrValue(c, tag, class, attr); result != "" {
			return result
		}
	}
	return ""
}

func (a *AtbScraper) GetCategories(ctx context.Context) ([]string, error) {
	doc, err := a.getHTML(ctx, atbBaseUrl)
	if err != nil {
		return nil, err
	}
	menu := findNodeByClass(doc, "ul", "category-menu")
	if menu == nil {
		return nil, fmt.Errorf("category menu not found")
	}
	menuItems := findAllNodesByClass(menu, "li", "category-menu__item")
	var categories []string
	for _, item := range menuItems {
		href := findHref(item)
		if href != "" {
			categories = append(categories, atbBaseUrl+href)
		}
	}
	return categories, nil
}

func (a *AtbScraper) GetProducts(ctx context.Context, cts []string) ([][]string, error) {

	var result [][]string
	var wg sync.WaitGroup
	httpSemaphore := make(chan struct{}, atbSemaphoreSize)
	resultsChan := make(chan []string)

	for _, category := range cts {
		wg.Add(1)
		go func(category string) {
			httpSemaphore <- struct{}{}
			defer func() { <-httpSemaphore }()
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			default:
			}
			a.fetchProducts(ctx, category, nil, resultsChan)
		}(category)
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

func (a *AtbScraper) fetchProducts(ctx context.Context, urlStr string, page *int, resultChan chan []string) {
	requestURL := urlStr
	var wg sync.WaitGroup
	if page != nil {
		requestURL = fmt.Sprintf("%s?page=%d", urlStr, *page)
	}

	fmt.Printf("getting products from: %s\n", requestURL)

	doc, err := a.getHTML(ctx, requestURL)
	if err != nil {
		log.Printf("error getting products from %s: %v", requestURL, err)
	}
	catalog := findNodeByClass(doc, "div", "catalog-list")
	if catalog == nil {
		log.Printf("catalog not found in %s", requestURL)
	}

	catalogItems := findAllNodesByClass(catalog, "article", "catalog-item")

	pageTitle := findNodeByClass(doc, "h1", "page-title")
	categoryName := getTextContent(pageTitle)

	for _, item := range catalogItems {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			default:
			}
			titleDiv := findNodeByClass(item, "div", "catalog-item__title")
			name := getTextContent(titleDiv)

			priceValue := findAttrValue(item, "data", "product-price__top", "value")
			currencyAbbr := findNodeByClass(item, "abbr", "product-price__currency-abbr")
			currency := getTextContent(currencyAbbr)
			price := priceValue + " " + currency

			href := findHref(titleDiv)
			ref := requestURL + href

			resultChan <- []string{
				name,
				ref,
				price,
				categoryName,
				"ATB",
			}
		}()
	}
	wg.Wait()

	nextPage, err := a.getNextPage(doc)
	if err != nil {
		return
	}
	a.fetchProducts(ctx, urlStr, nextPage, resultChan)
}

func (a *AtbScraper) getNextPage(doc *html.Node) (*int, error) {
	pagination := findNodeByClass(doc, "ul", "product-pagination__list")
	if pagination != nil {
		paginationItems := findAllNodesByClass(pagination, "li", "product-pagination__item")
		if len(paginationItems) >= 2 {
			maxPagesText := getTextContent(paginationItems[len(paginationItems)-2])
			maxPages, err := strconv.Atoi(maxPagesText)
			if err == nil {
				activeItem := findNodeByClass(pagination, "li", "product-pagination__item active")
				if activeItem != nil {
					currentPageText := getTextContent(activeItem)
					currentPage, err := strconv.Atoi(currentPageText)
					if err == nil && currentPage < maxPages {
						nextPage := currentPage + 1
						return &nextPage, nil
					}
					if currentPage == maxPages {
						return nil, fmt.Errorf("no more pages")
					}
				}
			}
		}
	}
	return nil, fmt.Errorf("no pages")
}

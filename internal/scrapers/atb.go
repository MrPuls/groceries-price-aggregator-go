package scrapers

import (
	"fmt"
	"log"
	"os"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func ParseHTML() {
	content, err := os.ReadFile("atb.htm")
	if err != nil {
		fmt.Printf("error reading file: %v", err)
	}
	doc, err := html.Parse(strings.NewReader(string(content)))
	if err != nil {
		log.Fatal(err)
	}
	var menuItems []html.Attribute

	for n := range doc.Descendants() {
		if n.Type == html.ElementNode && n.DataAtom == atom.A {
			for _, v := range n.Attr {
				if v.Key == "class" && v.Val == "category-menu__link-wrap js-dropdown-show" {
					menuItems = append(menuItems, n.Attr...)
				}
			}
		}
	}
	var categories []string
	for _, v := range menuItems {
		if v.Key == "href" {
			categories = append(categories, v.Val)
		}
	}
	fmt.Println(categories)
}

package scrappers

var (
	CSVHeader               = []string{"Name", "Ref", "Price", "Category", "Shop"}
	SilpoCategoriesURL      = "https://sf-ecom-api.silpo.ua/v1/branches/00000000-0000-0000-0000-000000000000/categories/tree"
	SilpoCategoryDetailsURL = "https://sf-ecom-api.silpo.ua/v1/uk/branches/00000000-0000-0000-0000-000000000000/categories"
	SilpoProductsURL        = "https://sf-ecom-api.silpo.ua/v1/uk/branches/00000000-0000-0000-0000-000000000000/products"
	MetroCategoriesURL      = "https://stores-api.zakaz.ua/stores/48215614/categories"
	MetroProductsURL        = "https://stores-api.zakaz.ua/stores/48215614/categories"
	VarusCategoriesURL      = "https://varus.ua/api/catalog/vue_storefront_catalog_2/banner/_search"
	VarusProductsURL        = "https://varus.ua/api/catalog/vue_storefront_catalog_2/product_v2/_search"
)

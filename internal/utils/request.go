package utils

import (
	"fmt"
	"net/http"
	"net/url"
)

func PrepareURLParams(p map[string]string) url.Values {
	v := url.Values{}
	for key, value := range p {
		v.Add(key, value)
	}
	return v
}

func MakeGetRequest(reqUrl string, headers map[string]string, params url.Values) (resp *http.Request, err error) {
	if params != nil {
		queryString := params.Encode()
		reqUrl = fmt.Sprintf("%s?%s", reqUrl, queryString)
	}

	req, err := http.NewRequest("GET", reqUrl, nil)
	if err != nil {
		panic(err)
	}

	if headers != nil {
		for k, v := range headers {
			if k == "Host" {
				req.Host = v
			}
			req.Header.Add(k, v)
		}
	}
	return req, nil
}

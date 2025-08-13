package utils

import (
	"context"
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

func MakeGetRequest(ctx context.Context, reqUrl string, headers map[string]string, params url.Values) (resp *http.Request, err error) {
	if params != nil {
		queryString := params.Encode()
		reqUrl = fmt.Sprintf("%s?%s", reqUrl, queryString)
	}

	if _, urlParseErr := url.Parse(reqUrl); urlParseErr != nil {
		return nil, fmt.Errorf("invalid URL: %v", urlParseErr)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", reqUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("[Util] error creating a request: %s", err)
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

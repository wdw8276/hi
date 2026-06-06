package main

import (
	"net/http"
	"time"
)

// httpResponse wraps http.Response for the main package.
type httpResponse = http.Response

var httpClient = &http.Client{Timeout: 2 * time.Second}

func httpGetImpl(url string) (*httpResponse, error) {
	return httpClient.Get(url)
}

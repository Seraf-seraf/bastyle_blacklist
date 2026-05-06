package httpclient

import (
	"net/http"
	"net/url"
)

type Options struct {
	ProxyURL string
}

func New(options Options) (*http.Client, error) {
	proxy := http.ProxyFromEnvironment
	if options.ProxyURL != "" {
		parsedProxyURL, err := url.Parse(options.ProxyURL)
		if err != nil {
			return nil, err
		}
		proxy = http.ProxyURL(parsedProxyURL)
	}

	return &http.Client{
		Transport: &http.Transport{
			Proxy: proxy,
		},
	}, nil
}

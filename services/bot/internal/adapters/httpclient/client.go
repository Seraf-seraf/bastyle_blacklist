package httpclient

import (
	"net/http"
	"net/url"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
)

type Options struct {
	ProxyURL string
}

func New(options Options) (*http.Client, error) {
	const methodCtx = "httpclient/New"

	proxy := http.ProxyFromEnvironment
	if options.ProxyURL != "" {
		parsedProxyURL, err := url.Parse(options.ProxyURL)
		if err != nil {
			return nil, apperrors.Wrap(methodCtx, err)
		}
		proxy = http.ProxyURL(parsedProxyURL)
	}

	return &http.Client{
		Transport: &http.Transport{
			Proxy: proxy,
		},
	}, nil
}

package httpclient

import (
	"net/http"
	"testing"
)

func TestNewUsesConfiguredProxy(t *testing.T) {
	client, err := New(Options{
		ProxyURL: "http://127.0.0.1:8080",
	})
	if err != nil {
		t.Fatal(err)
	}

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T, want *http.Transport", client.Transport)
	}

	request, err := http.NewRequest(http.MethodGet, "https://api.telegram.org", nil)
	if err != nil {
		t.Fatal(err)
	}

	proxyURL, err := transport.Proxy(request)
	if err != nil {
		t.Fatal(err)
	}
	if proxyURL.String() != "http://127.0.0.1:8080" {
		t.Fatalf("proxy url = %q, want configured proxy", proxyURL.String())
	}
}

func TestNewRejectsInvalidProxyURL(t *testing.T) {
	if _, err := New(Options{ProxyURL: "://bad"}); err == nil {
		t.Fatal("expected error")
	}
}

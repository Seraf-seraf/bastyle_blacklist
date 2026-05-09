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
		t.Fatalf("транспорт = %T, ожидался *http.Transport", client.Transport)
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
		t.Fatalf("URL прокси = %q, ожидалось настроенный прокси", proxyURL.String())
	}
}

func TestNewRejectsInvalidProxyURL(t *testing.T) {
	if _, err := New(Options{ProxyURL: "://bad"}); err == nil {
		t.Fatal("ожидалось: ошибка")
	}
}

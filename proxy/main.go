// DeepSeek TLS Proxy — tiny Go proxy that bridges Hermes (Python) to DeepSeek.
// Go's crypto/tls works on this VPS; system OpenSSL 3.0.13 does not.
// Build: go build -o dsproxy .
// Run:   ./dsproxy (listens on :8650, proxies to api.deepseek.com)
package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

func main() {
	target := "https://api.deepseek.com"
	port := "8650"
	if v := os.Getenv("PROXY_PORT"); v != "" {
		port = v
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS13,
			},
		},
	}

	handler := func(w http.ResponseWriter, r *http.Request) {
		proxyURL := target + r.URL.Path
		if r.URL.RawQuery != "" {
			proxyURL += "?" + r.URL.RawQuery
		}

		req, err := http.NewRequestWithContext(r.Context(), r.Method, proxyURL, r.Body)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		copyHeader(req.Header, r.Header)
		req.Header.Set("Host", "api.deepseek.com")

		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		defer resp.Body.Close()

		copyHeader(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}

	fmt.Printf("🔄 [Proxy] http://localhost:%s -> %s\n", port, target)
	fmt.Println("   Hermes dapat menggunakan http://localhost:8650/v1")
	log.Fatal(http.ListenAndServe(":"+port, http.HandlerFunc(handler)))
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

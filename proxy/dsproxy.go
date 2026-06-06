package proxy

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

// StartTLSProxy runs a local HTTP-to-HTTPS proxy for DeepSeek.
// Hermes connects to http://localhost:8650/v1, proxy forwards to https://api.deepseek.com.
// Needed because VPS OpenSSL 3.0.13 is rejected by DeepSeek's CDN.
func StartTLSProxy() {
	target := "https://api.deepseek.com" // DeepSeek endpoint: /chat/completions (no /v1)
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
		for k, vv := range r.Header {
			for _, v := range vv {
				req.Header.Add(k, v)
			}
		}
		req.Header.Set("Host", "api.deepseek.com")

		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		defer resp.Body.Close()

		for k, vv := range resp.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}

	addr := "127.0.0.1:" + port
	fmt.Printf("🔄 [TLS Proxy] http://%s -> %s\n", addr, target)
	go func() {
		log.Fatal(http.ListenAndServe(addr, http.HandlerFunc(handler)))
	}()
}

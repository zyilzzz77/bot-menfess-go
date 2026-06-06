package proxy

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

// StartTLSProxy runs a local HTTP-to-HTTPS proxy for DeepSeek.
// Hermes connects to http://127.0.0.1:8650, proxy forwards to https://api.deepseek.com.
// Uses the same transport config as the working downloader.
func StartTLSProxy() {
	target := "https://api.deepseek.com"
	port := "8650"
	if v := os.Getenv("PROXY_PORT"); v != "" {
		port = v
	}

	client := &http.Client{
		Timeout: 120 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          10,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   15 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
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

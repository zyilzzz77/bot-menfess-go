package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"bot-wa/internal/downloader"
)

func main() {
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		apiKey = "OXlJB9"
	}

	url := "https://vt.tiktok.com/ZSQjerA3a/"
	if len(os.Args) > 1 {
		url = os.Args[1]
	}

	dl := downloader.NewDownloader(apiKey, "./downloads")

	fmt.Println("⏳ Downloading...")
	fmt.Printf("   URL: %s\n", url)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	start := time.Now()
	result, err := dl.DownloadTikTok(ctx, url)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Downloaded in %.1fs\n", time.Since(start).Seconds())
	fmt.Printf("   Title: %s\n", result.Title)
	for i, item := range result.Items {
		fmt.Printf("   [%d] %s | %s | %.1f MB\n",
			i+1, item.FileName, item.FilePath,
			float64(item.FileSize)/1024/1024)
	}
	fmt.Println("\n📁 File disimpan di: downloads/")
}

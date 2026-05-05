package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// SpotifyPlaylistTrack represents a single track in a Spotify playlist
type SpotifyPlaylistTrack struct {
	Cover    string `json:"cover"`
	Title    string `json:"title"`
	Artists  string `json:"artists"`
	Album    string `json:"album"`
	Duration string `json:"duration"`
	URL      string `json:"url"`
}

// SpotifyPlaylistResult holds the parsed playlist data
type SpotifyPlaylistResult struct {
	Cover  string
	Title  string
	Tracks []SpotifyPlaylistTrack
}

// spotifyPlaylistResponse represents the API response for a Spotify playlist
type spotifyPlaylistResponse struct {
	Creator string `json:"creator"`
	Status  bool   `json:"status"`
	Data    struct {
		Cover string `json:"cover"`
		Title string `json:"title"`
	} `json:"data"`
	Tracks []SpotifyPlaylistTrack `json:"tracks"`
}

// FetchSpotifyPlaylist fetches playlist info and track list from the Spotify API
func (d *Downloader) FetchSpotifyPlaylist(ctx context.Context, playlistURL string) (*SpotifyPlaylistResult, error) {
	apiURL := fmt.Sprintf("%s/spotify?url=%s&apikey=%s",
		d.BaseURL,
		url.QueryEscape(playlistURL),
		d.APIKey,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	var apiResp spotifyPlaylistResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	if !apiResp.Status {
		return nil, fmt.Errorf("API returned error")
	}

	if len(apiResp.Tracks) == 0 {
		return nil, fmt.Errorf("playlist is empty or not found")
	}

	return &SpotifyPlaylistResult{
		Cover:  apiResp.Data.Cover,
		Title:  apiResp.Data.Title,
		Tracks: apiResp.Tracks,
	}, nil
}

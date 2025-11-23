package spotify

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"time"
)

// ========================================================== //
// Types

type Auth struct {
	AccessToken string `json:"access_token"`
	Type        string `json:"token_type"`
	Refresh     string `json:"refresh_token"`
	Expires     string `json:"expiry"`
}

type Playback struct {
	Progress float64     `json:"progress_ms"`
	Item     interface{} `json:"item"`
}

type Queue struct {
	Progress, Duration             float64
	TrackName, TrackPhoto, TrackID string
}

type PaginatedItems struct {
	Items  []interface{} `json:"items"`
	Next   *string       `json:"next"`
	Total  int           `json:"total"`
	Limit  int           `json:"limit"`
	Offset int           `json:"offset"`
}

// ========================================================== //
// HTTP client and retry logic

var httpClient = &http.Client{Timeout: 15 * time.Second}

func doRequest(method, endpoint string, header, query map[string]string) ([]byte, int, error) {
	req, err := http.NewRequest(method, endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	if len(query) > 0 {
		u, _ := url.Parse(req.URL.String())
		q := u.Query()
		for k, v := range query {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
		req.URL = u
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	return fetchWithRetry(req, 5)
}

func fetchWithRetry(req *http.Request, maxRetries int) ([]byte, int, error) {
	var lastErr error
	var status int
	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(backoffDuration(attempt))
			continue
		}
		status = resp.StatusCode
		body, _ := ioReadAll(resp.Body)
		resp.Body.Close()

		if status >= 200 && status < 300 {
			return body, status, nil
		}
		if status == 429 {
			fmt.Println("HTTP STATUS: 429")
		}
		if status == http.StatusTooManyRequests {
			time.Sleep(retryAfterDelay(resp))
			continue
		}
		if status >= 5 {
			time.Sleep(backoffDuration(attempt))
			continue
		}
		return body, status, nil
	}
	return nil, status, lastErr
}

func retryAfterDelay(resp *http.Response) time.Duration {
	// ra := resp.Header.Get("Retry-After")
	// if ra == "" {
	// 	return time.Second
	// }

	// if secs, err := strconv.Atoi(ra); err == nil {
	// 	return time.Duration(secs) * time.Second
	// }
	return time.Second
}

func backoffDuration(attempt int) time.Duration {
	base := 5 * time.Millisecond
	factor := math.Pow(2, float64(attempt))
	jitter := time.Duration(rand.Intn(300)) * time.Millisecond
	return time.Duration(float64(base)*factor) + jitter
}

// ========================================================== //
// Auth utilities

func getHeader() map[string]string {
	tok, err := loadOrObtainToken()
	if err != nil {
		log.Fatalf("Spotify token error: %v", err)
	}
	return map[string]string{"Authorization": "Bearer " + tok.AccessToken}
}

func loadOrObtainToken() (*Auth, error) {
	path := "./main/authToken.txt"

	// wd, _ := os.Getwd()
	// tokenPath := filepath.Join(wd, "authToken.txt")
	// f, err := os.Open(tokenPath)
	b, err := os.ReadFile(path)
	if err == nil {
		var t Auth
		if json.Unmarshal(b, &t) == nil && t.AccessToken != "" && !isExpired(&t) {
			return &t, nil
		}
		log.Println("Existing Spotify token expired or invalid.")
	} else {
		log.Println("No Spotify token found; starting authorization flow...", err)
	}

	if err := launchAuthFlow(); err != nil {
		return nil, err
	}

	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil {
			var t Auth
			if json.Unmarshal(b, &t) == nil && t.AccessToken != "" && !isExpired(&t) {
				return &t, nil
			}
		}
		time.Sleep(1 * time.Second)
	}
	return nil, errors.New("timeout waiting for Spotify auth token")
}

func isExpired(t *Auth) bool {
	if t == nil || t.Expires == "" {
		return false
	}
	exp, err := time.Parse(time.RFC3339, t.Expires)
	if err != nil {
		return false
	}
	return time.Now().After(exp.Add(-1 * time.Minute))
}

func launchAuthFlow() error {
	// Load redirect_url from authConfig
	data, err := os.ReadFile("./main/authConfig.txt")
	if err != nil {
		return fmt.Errorf("failed to read authConfig.txt: %w", err)
	}

	var cfg struct {
		RedirectURL string `json:"redirect_url"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("failed to parse redirect_url: %w", err)
	}

	if cfg.RedirectURL == "" {
		return fmt.Errorf("redirect_url missing in authConfig.txt")
	}

	log.Println("Opening browser to:", cfg.RedirectURL)

	// Open browser to the redirect URL
	if err := exec.Command("xdg-open", cfg.RedirectURL).Start(); err != nil {
		log.Printf("Please open %s manually: %v", cfg.RedirectURL, err)
	}

	return nil
}

// ========================================================== //
// Spotify API: search, albums, tracks

func SearchArtist(artist string) ([]byte, error) {
	header := getHeader()
	header["Accept"] = "application/json"
	header["Content-Type"] = "application/json"

	q := map[string]string{
		"q":    artist,
		"type": "artist",
	}
	body, _, err := doRequest("GET", "https://api.spotify.com/v1/search", header, q)
	return body, err
}

// ArtistAlbumsAppearsOn fetches albums where the artist appears as a featured contributor ("appears_on").
// It complements ArtistAlbums, which only fetches albums where the artist is the primary owner.
func ArtistAlbumsAppearsOn(id string, limit int) ([]byte, error) {
	base := fmt.Sprintf("https://api.spotify.com/v1/artists/%s/albums", id)
	header := getHeader()
	header["Accept"], header["Content-Type"] = "application/json", "application/json"

	pageSize := 50
	totalLimit := limit
	if limit < 0 {
		totalLimit = math.MaxInt32
	}

	var agg []interface{}
	offset := 0
	for {
		params := map[string]string{
			"include_groups": "appears_on",
			"market":         "US",
			"limit":          strconv.Itoa(pageSize),
			"offset":         strconv.Itoa(offset),
		}
		body, status, err := doRequest("GET", base, header, params)
		if err != nil {
			return nil, err
		}
		if status < 200 || status >= 300 {
			log.Printf("warning: ArtistAlbumsAppearsOn non-2xx status=%d for id=%s", status, id)
		}
		var page PaginatedItems
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, err
		}
		agg = append(agg, page.Items...)
		if len(agg) >= totalLimit || page.Next == nil || *page.Next == "" || len(page.Items) == 0 {
			break
		}
		offset += pageSize
	}
	out, _ := json.Marshal(struct {
		Items []interface{} `json:"items"`
	}{agg})
	return out, nil
}

func ArtistAlbums(id string, limit int) ([]byte, int, error) {
	base := fmt.Sprintf("https://api.spotify.com/v1/artists/%s/albums", id)
	header := getHeader()
	header["Accept"], header["Content-Type"] = "application/json", "application/json"

	pageSize := 50
	totalLimit := limit
	if limit < 0 {
		totalLimit = math.MaxInt32
	}

	var agg []interface{}
	offset := 0
	for {
		params := map[string]string{
			"include_groups": "album,single",
			"market":         "US",
			"limit":          strconv.Itoa(pageSize),
			"offset":         strconv.Itoa(offset),
		}
		body, status, err := doRequest("GET", base, header, params)
		if err != nil {
			return nil, status, err
		}
		log.Println("Status code for ArtistAlbums:", status)
		if status == 429 {
			fmt.Println("HTTP STATUS: 429")
			time.Sleep(300 * time.Millisecond)
		}
		if status < 200 || status >= 300 {
			log.Printf("warning: ArtistAlbums non-2xx status=%d for id=%s", status, id)
		}
		var page PaginatedItems
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, status, err
		}
		agg = append(agg, page.Items...)
		if len(agg) >= totalLimit || page.Next == nil || *page.Next == "" || len(page.Items) == 0 {
			break
		}
		offset += pageSize
	}
	out, _ := json.Marshal(struct {
		Items []interface{} `json:"items"`
	}{agg})
	return out, 200, nil
}

func GetAlbumTracks(id string) ([]byte, error) {
	base := fmt.Sprintf("https://api.spotify.com/v1/albums/%s/tracks", id)
	header := getHeader()
	header["Accept"], header["Content-Type"] = "application/json", "application/json"

	pageSize := 50
	var agg []interface{}
	offset := 0
	for {
		params := map[string]string{
			"limit":  strconv.Itoa(pageSize),
			"offset": strconv.Itoa(offset),
		}
		body, status, err := doRequest("GET", base, header, params)
		if err != nil {
			return nil, err
		}
		if status < 200 || status >= 300 {
			log.Printf("warning: GetAlbumTracks non-2xx status=%d for id=%s", status, id)
		}
		var page PaginatedItems
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, err
		}
		agg = append(agg, page.Items...)
		if page.Next == nil || *page.Next == "" || len(page.Items) == 0 {
			break
		}
		offset += pageSize
	}
	out, _ := json.Marshal(struct {
		Items []interface{} `json:"items"`
	}{agg})
	return out, nil
}

// ========================================================== //
// Playlist utilities

// CreatePlaylist creates a new Spotify playlist for the current user and adds the given tracks.
func CreatePlaylist(name string, trackIDs []string) (string, error) {
	header := getHeader()
	header["Accept"], header["Content-Type"] = "application/json", "application/json"

	// Get current user
	userBody, status, err := doRequest("GET", "https://api.spotify.com/v1/me", header, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get current user: %w", err)
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("spotify returned status %d for /me", status)
	}
	var user struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(userBody, &user); err != nil || user.ID == "" {
		return "", fmt.Errorf("could not parse user ID")
	}

	// Create playlist for user
	createURL := fmt.Sprintf("https://api.spotify.com/v1/users/%s/playlists", user.ID)
	reqBody := map[string]interface{}{
		"name":        name,
		"description": "Generated by SixDegreeSpotify",
		"public":      false,
	}
	bodyJSON, _ := json.Marshal(reqBody)

	req, err := http.NewRequest("POST", createURL, io.NopCloser(bytes.NewReader(bodyJSON)))
	if err != nil {
		return "", err
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("playlist creation failed: %s", string(b))
	}

	var playlist struct {
		ID  string            `json:"id"`
		URL map[string]string `json:"external_urls"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&playlist); err != nil {
		return "", fmt.Errorf("decode playlist response: %w", err)
	}

	// Add tracks
	if len(trackIDs) > 0 {
		addURL := fmt.Sprintf("https://api.spotify.com/v1/playlists/%s/tracks", playlist.ID)
		uris := make([]string, len(trackIDs))
		for i, id := range trackIDs {
			uris[i] = "spotify:track:" + id
		}
		addBody := map[string]interface{}{"uris": uris}
		addJSON, _ := json.Marshal(addBody)

		addReq, _ := http.NewRequest("POST", addURL, io.NopCloser(bytes.NewReader(addJSON)))
		for k, v := range header {
			addReq.Header.Set(k, v)
		}
		addResp, err := httpClient.Do(addReq)
		if err != nil {
			return "", err
		}
		defer addResp.Body.Close()
		if addResp.StatusCode < 200 || addResp.StatusCode >= 300 {
			b, _ := io.ReadAll(addResp.Body)
			return "", fmt.Errorf("add tracks failed: %s", string(b))
		}
	}

	return playlist.URL["spotify"], nil
}

// ========================================================== //
// Playback utilities

func reqPlayback() (Playback, []byte) {
	ep := "https://api.spotify.com/v1/me/player/currently-playing?market=US"
	var p Playback
	h := getHeader()
	h["Accept"], h["Content-Type"] = "application/json", "application/json"
	body, _, err := doRequest("GET", ep, h, nil)
	if err != nil {
		log.Println("reqPlayback error:", err)
		return p, nil
	}
	_ = json.Unmarshal(body, &p)
	return p, body
}

func postSpotify(endpoint string, header, query map[string]string) {
	_, status, err := doRequest("POST", endpoint, header, query)
	if err != nil {
		log.Printf("POST %s error: %v", endpoint, err)
		return
	}
	log.Printf("POST %s status %d", endpoint, status)
}

func AddQueue(tracks []string) {
	log.Printf("Adding %d tracks to queue", len(tracks))
	headers := getHeader()
	headers["Content-Type"], headers["Accept"] = "application/json", "application/json"
	uri := "spotify:track:"
	for _, t := range tracks {
		postSpotify("https://api.spotify.com/v1/me/player/queue", headers, map[string]string{"uri": uri + t})
	}
	postSpotify("https://api.spotify.com/v1/me/player/next", headers, nil)
}

func Controller(endpoint string) {
	log.Println("Invoking controller:", endpoint)
	headers := getHeader()
	headers["Content-Type"], headers["Accept"] = "application/json", "application/json"
	postSpotify(endpoint, headers, nil)
}

func GetPlayback() Queue {
	pb, _ := reqPlayback()
	var q Queue
	q.Progress = pb.Progress

	itemMap, ok := pb.Item.(map[string]interface{})
	if !ok {
		return q
	}
	if dur, ok := itemMap["duration_ms"].(float64); ok {
		q.Duration = dur
	}
	if name, ok := itemMap["name"].(string); ok {
		q.TrackName = name
	}
	if id, ok := itemMap["id"].(string); ok {
		q.TrackID = id
	}
	if album, ok := itemMap["album"].(map[string]interface{}); ok {
		if imgs, ok := album["images"].([]interface{}); ok && len(imgs) > 1 {
			if m, ok := imgs[1].(map[string]interface{}); ok {
				if u, ok := m["url"].(string); ok {
					q.TrackPhoto = u
				}
			}
		}
	}
	return q
}

// ========================================================== //
// internal small util

func ioReadAll(r io.Reader) ([]byte, error) {
	const max = 10 << 20 // 10 MB safeguard
	b, err := io.ReadAll(io.LimitReader(r, max))
	return b, err
}

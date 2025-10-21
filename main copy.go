package main

// import (
// 	"context"
// 	"encoding/json"
// 	"fmt"
// 	"log"
// 	"math/rand"
// 	"net/http"
// 	"net/url"
// 	"os"
// 	"os/exec"
// 	"time"

// 	"github.com/Jonnymurillo288/SixDegreesSpotify/db"
// 	"github.com/Jonnymurillo288/SixDegreesSpotify/spotify"
// )

// const (
// 	tokenURL             = "https://accounts.spotify.com/api/token"
// 	genreSeedsURL        = "https://api.spotify.com/v1/recommendations/available-genre-seeds"
// 	recommendationsURL   = "https://api.spotify.com/v1/recommendations"
// 	defaultArtistSamples = 10
// )

// type genreSeedsResp struct {
// 	Genres []string `json:"genres"`
// }

// type recommendationsResp struct {
// 	Tracks []struct {
// 		ID      string `json:"id"`
// 		Name    string `json:"name"`
// 		Artists []struct {
// 			ID   string `json:"id"`
// 			Name string `json:"name"`
// 		} `json:"artists"`
// 	} `json:"tracks"`
// }

// var auth *spotify.Auth

// // ===== Main =====

// func main() {
// 	store, err := db.Open("")
// 	if err != nil {
// 		log.Fatalf("failed to connect: %v", err)
// 	}
// 	defer store.Close()

// 	if err := store.Migrate(context.Background()); err != nil {
// 		log.Fatalf("migration failed: %v", err)
// 	}

// 	log.Println("Database ready!")
// 	var name1, name2 string
// 	// Ensure Spotify authorization before making any API calls
// 	if err := ensureSpotifyAuth(); err != nil {
// 		log.Fatalf("Spotify authorization failed: %v", err)
// 	}

// 	genres, err := getGenreSeeds(auth.AccessToken)
// 	if err != nil {
// 		log.Fatalf("getGenreSeeds error: %v", err)
// 	}
// 	if len(genres) == 0 {
// 		log.Fatal("Spotify returned no genre seeds.")
// 	}

// 	// Pull artists from a few random genres until we have enough unique names.
// 	want := defaultArtistSamples
// 	unique := make(map[string]struct{})
// 	safety := 0

// 	for len(unique) < want && safety < 10 {
// 		safety++
// 		genre := genres[rand.Intn(len(genres))]
// 		names, err := getArtistsFromRecommendations(auth.AccessToken, genre, 100)
// 		if err != nil {
// 			log.Printf("recommendations error for genre %q: %v", genre, err)
// 			continue
// 		}
// 		for _, n := range names {
// 			unique[n] = struct{}{}
// 			if len(unique) >= want {
// 				break
// 			}
// 		}
// 	}

// 	if len(unique) == 0 {
// 		log.Fatal("Failed to retrieve any artists from recommendations.")
// 	}
// 	// Convert map to slice for indexed access
// 	artists := make([]string, 0, len(unique))
// 	for name := range unique {
// 		artists = append(artists, name)
// 		if len(artists) >= want {
// 			break
// 		}
// 	}

// 	// Print results
// 	fmt.Println("\n=== Sample Artists ===")

// 	for i := 0; i < len(artists)-1; i += 2 {
// 		name1 = artists[i]
// 		name2 = artists[i+1]

// 		if i >= want {
// 			break
// 		}

// 		// Example of running the other Go command for your start/find
// 		fmt.Printf("\nRunning search command: go run main.go -start %q -find %q\n", name1, name2)

// 		cmd := exec.Command("go", "run", "main.go", "-start", name1, "-find", name2)
// 		cmd.Stdout = os.Stdout
// 		cmd.Stderr = os.Stderr
// 		if err := cmd.Run(); err != nil {
// 			log.Printf("Failed to run command: %v", err)
// 		}
// 	}
// }

// func authGET(token, endpoint string) (*http.Response, error) {
// 	req, err := http.NewRequest("GET", endpoint, nil)
// 	if err != nil {
// 		return nil, err
// 	}
// 	req.Header.Set("Authorization", "Bearer "+token)
// 	return http.DefaultClient.Do(req)
// }

// func getGenreSeeds(token string) ([]string, error) {
// 	resp, err := authGET(token, genreSeedsURL)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer resp.Body.Close()
// 	if resp.StatusCode/100 != 2 {
// 		return nil, fmt.Errorf("genre seeds status: %s", resp.Status)
// 	}
// 	var gr genreSeedsResp
// 	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
// 		return nil, err
// 	}
// 	return gr.Genres, nil
// }

// func getArtistsFromRecommendations(token, genre string, limit int) ([]string, error) {
// 	if limit <= 0 || limit > 100 {
// 		limit = 50
// 	}
// 	// Add mild randomness to audio features to vary results.
// 	params := url.Values{}
// 	params.Set("limit", fmt.Sprint(limit))
// 	params.Set("seed_genres", genre)
// 	params.Set("min_popularity", fmt.Sprint(rand.Intn(50))) // 0..49
// 	params.Set("target_energy", fmt.Sprintf("%.2f", rand.Float64()))
// 	params.Set("market", "US") // helps ensure playable tracks

// 	endpoint := recommendationsURL + "?" + params.Encode()
// 	resp, err := authGET(token, endpoint)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer resp.Body.Close()
// 	if resp.StatusCode/100 != 2 {
// 		return nil, fmt.Errorf("recommendations status: %s", resp.Status)
// 	}

// 	var rr recommendationsResp
// 	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
// 		return nil, err
// 	}

// 	seen := make(map[string]struct{})
// 	var names []string
// 	for _, t := range rr.Tracks {
// 		for _, a := range t.Artists {
// 			if a.Name == "" {
// 				continue
// 			}
// 			if _, ok := seen[a.Name]; !ok {
// 				names = append(names, a.Name)
// 				seen[a.Name] = struct{}{}
// 			}
// 		}
// 	}
// 	return names, nil
// }

// // ensureSpotifyAuth verifies valid token exists or triggers auth flow.
// func ensureSpotifyAuth() error {
// 	// Ensure auth configuration exists (bootstrap from sample if needed)
// 	cfg := "./main/authConfig.txt"
// 	if _, err := os.Stat(cfg); err != nil {
// 		if os.IsNotExist(err) {
// 			sample := "./main/authConfig.sample.json"
// 			if b, rerr := os.ReadFile(sample); rerr == nil {
// 				_ = os.WriteFile(cfg, b, 0o600)
// 				return fmt.Errorf("created %s from sample; edit it with your Spotify credentials and re-run", cfg)
// 			}
// 			return fmt.Errorf("missing %s; create it with your Spotify app credentials", cfg)
// 		}
// 		return fmt.Errorf("failed to check authConfig.txt: %w", err)
// 	}
// 	// If token already valid, load it
// 	if t, ok := tokenValid("./main/authToken.txt"); ok {
// 		auth = t
// 		return nil
// 	}

// 	// Start the local auth server
// 	cmd := exec.Command("go", "run", "./main/auth.go")
// 	cmd.Stdout = os.Stdout
// 	cmd.Stderr = os.Stderr
// 	if err := cmd.Start(); err != nil {
// 		return fmt.Errorf("failed to start auth server: %w", err)
// 	}

// 	// Wait for the server to become reachable on localhost:8392
// 	client := &http.Client{Timeout: 2 * time.Second}
// 	readyDeadline := time.Now().Add(10 * time.Second)
// 	var up bool
// 	for time.Now().Before(readyDeadline) {
// 		resp, err := client.Get("http://localhost:8392/")
// 		if err == nil {
// 			if resp.Body != nil {
// 				resp.Body.Close()
// 			}
// 			up = true
// 			break
// 		}
// 		time.Sleep(300 * time.Millisecond)
// 	}
// 	if !up {
// 		return fmt.Errorf("authorization server did not start on http://localhost:8392; run `go run ./main/auth.go` manually to inspect errors")
// 	}

// 	fmt.Println("Spotify auth server is running on http://localhost:8392/")
// 	fmt.Println("If your browser does not open automatically, visit the URL above to authorize.")

// 	// Attempt to open the browser for user authorization (best-effort)
// 	_ = exec.Command("xdg-open", "http://localhost:8392/").Start()

// 	// Wait for token to be created and become valid
// 	deadline := time.Now().Add(2 * time.Minute)
// 	for time.Now().Before(deadline) {
// 		if _, ok := tokenValid("./main/authToken.txt"); ok {
// 			return nil
// 		}
// 		time.Sleep(1 * time.Second)
// 	}
// 	return fmt.Errorf("timeout waiting for Spotify token; complete the authorization in your browser and retry")
// }

// // tokenValid parses the stored token and checks expiry safety window.
// func tokenValid(path string) (*spotify.Auth, bool) {
// 	b, err := os.ReadFile(path)
// 	if err != nil {
// 		return nil, false
// 	}
// 	var t spotify.Auth
// 	if err := json.Unmarshal(b, &t); err != nil {
// 		return nil, false
// 	}
// 	if t.AccessToken == "" {
// 		return nil, false
// 	}
// 	if t.Expires == "" {
// 		return nil, false
// 	}
// 	exp, err := time.Parse(time.RFC3339Nano, t.Expires)
// 	if err != nil {
// 		exp, err = time.Parse(time.RFC3339, t.Expires)
// 	}
// 	if err != nil {
// 		return nil, false
// 	}
// 	if time.Now().After(exp.Add(-1 * time.Minute)) {
// 		return nil, false
// 	}
// 	return &t, true
// }

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"

	"github.com/Jonnymurillo288/SixDegreesSpotify/spotify"
)

const (
	tokenURL             = "https://accounts.spotify.com/api/token"
	genreSeedsURL        = "https://api.spotify.com/v1/recommendations/available-genre-seeds"
	recommendationsURL   = "https://api.spotify.com/v1/recommendations"
	defaultArtistSamples = 10
)

type genreSeedsResp struct {
	Genres []string `json:"genres"`
}

type recommendationsResp struct {
	Tracks []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Artists []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"artists"`
	} `json:"tracks"`
}

var auth *spotify.Auth

// ===== Main =====

func Gather() {
	var name1, name2 string
	// Ensure Spotify authorization before making any API calls
	if err := ensureSpotifyAuth(); err != nil {
		log.Fatalf("Spotify authorization failed: %v", err)
	}

	genres, err := getGenreSeeds(auth.AccessToken)
	if err != nil {
		log.Fatalf("getGenreSeeds error: %v", err)
	}
	if len(genres) == 0 {
		log.Fatal("Spotify returned no genre seeds.")
	}

	// Pull artists from a few random genres until we have enough unique names.
	want := defaultArtistSamples
	unique := make(map[string]struct{})
	safety := 0

	for len(unique) < want && safety < 10 {
		safety++
		genre := genres[rand.Intn(len(genres))]
		names, err := getArtistsFromRecommendations(auth.AccessToken, genre, 100)
		if err != nil {
			log.Printf("recommendations error for genre %q: %v", genre, err)
			continue
		}
		for _, n := range names {
			unique[n] = struct{}{}
			if len(unique) >= want {
				break
			}
		}
	}

	if len(unique) == 0 {
		log.Fatal("Failed to retrieve any artists from recommendations.")
	}
	// Convert map to slice for indexed access
	artists := make([]string, 0, len(unique))
	for name := range unique {
		artists = append(artists, name)
		if len(artists) >= want {
			break
		}
	}

	// Print results
	fmt.Println("\n=== Sample Artists ===")

	for i := 0; i < len(artists)-1; i += 2 {
		name1 = artists[i]
		name2 = artists[i+1]

		if i >= want {
			break
		}

		// Example of running the other Go command for your start/find
		fmt.Printf("\nRunning search command: go run main.go -start %q -find %q\n", name1, name2)

		cmd := exec.Command("go", "run", "main.go", "-start", name1, "-find", name2)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			log.Printf("Failed to run command: %v", err)
		}
	}
}

func authGET(token, endpoint string) (*http.Response, error) {
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return http.DefaultClient.Do(req)
}

func getGenreSeeds(token string) ([]string, error) {
	resp, err := authGET(token, genreSeedsURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("genre seeds status: %s", resp.Status)
	}
	var gr genreSeedsResp
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		return nil, err
	}
	return gr.Genres, nil
}

func getArtistsFromRecommendations(token, genre string, limit int) ([]string, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	// Add mild randomness to audio features to vary results.
	params := url.Values{}
	params.Set("limit", fmt.Sprint(limit))
	params.Set("seed_genres", genre)
	params.Set("min_popularity", fmt.Sprint(rand.Intn(50))) // 0..49
	params.Set("target_energy", fmt.Sprintf("%.2f", rand.Float64()))
	params.Set("market", "US") // helps ensure playable tracks

	endpoint := recommendationsURL + "?" + params.Encode()
	resp, err := authGET(token, endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("recommendations status: %s", resp.Status)
	}

	var rr recommendationsResp
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	var names []string
	for _, t := range rr.Tracks {
		for _, a := range t.Artists {
			if a.Name == "" {
				continue
			}
			if _, ok := seen[a.Name]; !ok {
				names = append(names, a.Name)
				seen[a.Name] = struct{}{}
			}
		}
	}
	return names, nil
}

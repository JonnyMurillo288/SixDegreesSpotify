package sixdegrees

import (
	"encoding/json"
	"log"

	"github.com/Jonnymurillo288/SixDegreesSpotify/spotify"
)

// Artists represents one artist node with tracks and metadata.
type Artists struct {
	Name                        string
	ID                          string
	Tracks                      []Track
	Popularity                  float64
	PopularityKeys, NumFeatKeys []int
	Genres                      map[string]int
}

// searchResponse matches Spotify /v1/search
type searchResponse struct {
	Artists struct {
		Items []struct {
			ID         string   `json:"id"`
			Name       string   `json:"name"`
			Popularity float64  `json:"popularity"`
			Genres     []string `json:"genres"`
		} `json:"items"`
	} `json:"artists"`
}

// InputArtist queries Spotify and returns an initialized Artists struct.
// It returns a placeholder with Name set if lookup fails (so callers can continue gracefully).
func InputArtist(name string) *Artists {
	a := &Artists{
		Name:           name,
		Tracks:         make([]Track, 0),
		PopularityKeys: []int{},
		NumFeatKeys:    []int{},
		Genres:         make(map[string]int),
	}

	body, err := spotify.SearchArtist(name)
	if err != nil {
		log.Printf("SearchArtist error for %q: %v", name, err)
		return a
	}
	if !json.Valid(body) {
		log.Printf("Spotify returned invalid JSON for %q", name)
		return a
	}

	var resp searchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		log.Printf("JSON unmarshal error for %q: %v", name, err)
		return a
	}

	if len(resp.Artists.Items) == 0 {
		log.Printf("No artist found for query %q", name)
		return a
	}

	item := resp.Artists.Items[0]
	a.Name = item.Name
	a.ID = item.ID
	a.Popularity = item.Popularity
	for _, g := range item.Genres {
		a.Genres[g]++
	}

	log.Printf("Loaded artist: %s (ID %s)", a.Name, a.ID)
	return a
}

// CreateArtists creates a lightweight Artists struct manually.
func CreateArtists(name, id string) *Artists {
	return &Artists{
		Name:   name,
		ID:     id,
		Tracks: make([]Track, 0),
		Genres: make(map[string]int),
	}
}

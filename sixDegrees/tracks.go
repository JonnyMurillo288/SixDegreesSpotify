package sixdegrees

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	_ "github.com/go-sql-driver/mysql"
)

type Track struct {
	Artist   *Artists // Primary artist
	Name     string
	PhotoURL string
	ID       string
	Featured []*Artists // Featured artists
}

type trackResponse struct {
	Href  string `json:"href"`
	Items []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Artists []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"artists"`
	} `json:"items"`
	Limit    int         `json:"limit"`
	Next     interface{} `json:"next"`
	Offset   int         `json:"offset"`
	Previous interface{} `json:"previous"`
	Total    int         `json:"total"`
}

type albumResponse struct {
	Items []struct {
		ID      string `json:"id"`
		Artists []struct {
			Name string `json:"name"`
		} `json:"artists"`
	} `json:"items"`
}

// Helper tracks visited artists, distances, predecessor chain, and edge evidence.
type Helper struct {
	ArtistMap map[string]*Artists // visited artists by name
	DistTo    map[string]int      // distance (hops)
	Prev      map[string]string   // predecessor chain
	Evidence  map[string]string   // track name connecting Prev[x] -> x
}

// NewHelper initializes an empty BFS helper
func NewHelper() *Helper {
	return &Helper{
		ArtistMap: make(map[string]*Artists),
		DistTo:    make(map[string]int),
		Prev:      make(map[string]string),
		Evidence:  make(map[string]string),
	}
}

func (h *Helper) ReconstructPath(start, target string) ([]string, []string) {
	fmt.Println("Reconstructing Path")
	for k, d := range h.DistTo { // This tests the DistTo and Prev maps, should be non decreasing order of discovery
		fmt.Printf("%s depth %d prev %s\n", k, d, h.Prev[k])
	}

	if start == "" || target == "" {
		return nil, nil
	}
	fmt.Println("Helper looks like:", h)

	cur := target
	var path []string
	var songs []string

	for cur != "" {
		path = append(path, cur)

		// Only add a song if it exists (and not for the start node)
		if song, ok := h.Evidence[cur]; ok && cur != start {
			songs = append(songs, song)
		}

		if cur == start {
			break
		}

		cur = h.Prev[cur]
		if cur == "" {
			return nil, nil // path not found
		}
	}

	// reverse both slices
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	for i, j := 0, len(songs)-1; i < j; i, j = i+1, j-1 {
		songs[i], songs[j] = songs[j], songs[i]
	}

	return path, songs
}

// newTrack builds a Track safely
func newTrack(art *Artists, name, photo, id string, feat []*Artists) Track {
	return Track{
		Artist:   art,
		Name:     name,
		PhotoURL: photo,
		ID:       id,
		Featured: feat,
	}
}

// CreateTracks converts raw Spotify album-track JSON into Track structs.
func (a *Artists) CreateTracks(data []byte, h *Helper) ([]Track, *Helper) {
	if h == nil {
		h = NewHelper()
	}

	var parsed trackResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		log.Printf("CreateTracks: failed to parse tracks for %s: %v", a.Name, err)
		return nil, h
	}
	log.Printf("Parsed %d items for %s", len(parsed.Items), a.Name)
	fmt.Println("Parsed", len(parsed.Items), "items for", a.Name)
	if len(parsed.Items) == 0 {
		log.Printf("CreateTracks: no tracks found for %s", a.Name)
		return nil, h
	}

	var tracks []Track
	for _, item := range parsed.Items {
		var feat []*Artists
		for _, art := range item.Artists {
			if art.Name == a.Name {
				continue
			}
			if existing, ok := h.ArtistMap[art.Name]; ok {
				feat = append(feat, existing)
			} else {
				if newA := InputArtist(art.Name); newA != nil {
					h.ArtistMap[newA.Name] = newA
					feat = append(feat, newA)
				}
			}
		}
		tracks = append(tracks, newTrack(a, item.Name, "", item.ID, feat))
	}
	log.Printf("Created %d tracks for %s", len(tracks), a.Name)
	return tracks, h
}

// ParseAlbums extracts album IDs from Spotify's artist-albums JSON response.
func (a *Artists) ParseAlbums(data []byte) []string {
	var parsed albumResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		log.Printf("ParseAlbums: failed for %s: %v", a.Name, err)
		return nil
	}
	if len(parsed.Items) == 0 {
		log.Printf("ParseAlbums: no albums found for %s", a.Name)
		return nil
	}

	var ids []string
	for _, item := range parsed.Items {
		skip := false
		for _, art := range item.Artists {
			if art.Name == "Various Artists" {
				skip = true
				break
			}
		}
		if !skip {
			ids = append(ids, item.ID)
		}
	}
	log.Printf("%s: parsed %d album IDs", a.Name, len(ids))
	return ids
}

// CheckTracks returns the number of track rows in a database.
func (art *Artists) CheckTracks(db *sql.DB) (int, error) {
	if db == nil {
		return 0, errors.New("nil database connection")
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM Tracks").Scan(&count); err != nil {
		return 0, err
	}
	log.Printf("Database contains %d tracks total", count)
	return count, nil
}

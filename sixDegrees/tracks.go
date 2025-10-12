package sixdegrees

import (
	"database/sql"
	"encoding/json"
	"errors"
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
	Items []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Artists []struct {
			Name string `json:"name"`
			ID   string `json:"id"`
		} `json:"artists"`
	} `json:"items"`
}

type albumResponse struct {
	Items []struct {
		ID      string `json:"id"`
		Artists []struct {
			Name string `json:"name"`
		} `json:"artists"`
	} `json:"items"`
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

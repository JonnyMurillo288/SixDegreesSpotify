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
	Artist    *Artists // Primary artist
	Name      string
	PhotoURL  string
	ID        string
	Recording string
	Featured  []*Artists // Featured artists
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

// in sixdegrees package or same place you keep Helper types
type EdgeSnap struct {
	FromID    string
	FromName  string
	ToID      string
	ToName    string
	TrackID   string
	TrackName string
	PhotoURL  string
}

type Helper struct {
	ArtistByID map[string]*Artists
	IDByName   map[string]string

	DistToID map[string]int      // by artist ID
	PrevID   map[string]string   // toID -> fromID
	Evidence map[string]EdgeSnap // toID -> immutable edge

}

func NewHelper() *Helper {
	return &Helper{
		ArtistByID: make(map[string]*Artists),
		IDByName:   make(map[string]string),

		DistToID: make(map[string]int),
		PrevID:   make(map[string]string),
		Evidence: make(map[string]EdgeSnap),
	}
}

// Function to reconstruct the path from start to target artist.
// THis function operates by backtracking from the target to the start using the Prev map.
// At the end the path and songs slices are reversed to present them in the correct order.
func (h *Helper) ReconstructPathIDs(startID, targetID string) ([]string, []EdgeSnap) {
	fmt.Println("Reconstructing path from", startID, "to", targetID)
	if startID == "" || targetID == "" {
		return nil, nil
	}

	cur := targetID
	var ids []string
	var edges []EdgeSnap

	for cur != "" {
		ids = append(ids, cur)
		if cur != startID {
			if e, ok := h.Evidence[cur]; ok {
				edges = append(edges, e)
			}
		}
		fmt.Print("Edge evidence so far:", edges, "\n")
		if cur == startID {
			break
		}
		cur = h.PrevID[cur]
		if cur == "" {
			return nil, nil
		} // not found
	}

	// reverse
	for i, j := 0, len(ids)-1; i < j; i, j = i+1, j-1 {
		ids[i], ids[j] = ids[j], ids[i]
	}
	for i, j := 0, len(edges)-1; i < j; i, j = i+1, j-1 {
		edges[i], edges[j] = edges[j], edges[i]
	}

	return ids, edges
}

func (h *Helper) PrintPath(ids []string, edges []EdgeSnap) {
	for i, id := range ids {
		name := ""
		if a := h.ArtistByID[id]; a != nil {
			name = a.Name
		}
		fmt.Printf("%d. %s (%s)\n", i+1, name, id)
		if i < len(edges) {
			e := edges[i]
			fmt.Printf("   └─ [%s] %s → %s\n", e.TrackID, e.TrackName, e.ToName)
		}
	}
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
	_ = json.Unmarshal(data, &parsed)

	// log.Printf("Parsed %d items for %s", len(parsed.Items), a.Name)
	// fmt.Println("Parsed", len(parsed.Items), "items for", a.Name)
	if len(parsed.Items) == 0 {
		log.Printf("CreateTracks: no tracks found for %s", a.Name)
		return nil, h
	}

	var tracks []Track
	for _, item := range parsed.Items {

		// --- Step 1: check if this artist is on this track ---
		hasSelf := false
		for _, art := range item.Artists {
			if art.ID == a.ID || art.Name == a.Name {
				hasSelf = true
				break
			}
		}
		if !hasSelf {
			// This track does not include artist 'a', skip it.
			continue
		}

		// --- Step 2: build list of featured artists (others on the same track) ---
		var feat []*Artists
		for _, art := range item.Artists {
			if art.ID == "" || art.Name == "" {
				continue // ignore incomplete data
			}
			if art.ID == a.ID || art.Name == a.Name {
				continue // skip self
			}

			// Reuse or create artist reference
			if existing, ok := h.ArtistByID[art.ID]; ok {
				feat = append(feat, existing)
			} else {
				if newA := InputArtist(art.Name); newA != nil {
					h.ArtistByID[newA.ID] = newA
					feat = append(feat, newA)
				}
			}
		}

		if len(feat) == 0 {
			continue
		}

		// --- Step 3: record the valid collaborative track ---
		// Will only get to this step if 1. The track contains a.Name
		// 2. There are other featured artists
		tracks = append(tracks, newTrack(a, item.Name, "", item.ID, feat))
	}
	// log.Printf("Created %d tracks for %s", len(tracks), a.Name)
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

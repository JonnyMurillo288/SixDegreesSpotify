// Shared search helpers: BFS/Dijkstra enrichment, caching

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	sixdegrees "github.com/Jonnymurillo288/SixDegreesSpotify/sixDegrees"
	"github.com/Jonnymurillo288/SixDegreesSpotify/spotify"
	lru "github.com/hashicorp/golang-lru"
)

// ---------------------------
// Caching
// ---------------------------

type Cache struct {
	mu           sync.RWMutex
	albumTracks  *lru.Cache // albumID -> []byte (tracks JSON)
	artistAlbums *lru.Cache // artistID -> []byte (albums JSON: primary + appears_on merged)
}

func NewCache() *Cache {
	at, _ := lru.New(1000) // album tracks cache
	aa, _ := lru.New(1000) // artist albums cache
	return &Cache{
		albumTracks:  at,
		artistAlbums: aa,
	}
}

var cache *Cache

func init() {
	cache = NewCache()
}

// getArtistAlbumsMergedCached returns a merged albums JSON (primary + appears_on),
// using per-artist cache; when offline=false and miss, it fetches from Spotify and caches.
func getArtistAlbumsMergedCached(artistID string, limit int, offline bool, verbose bool) ([]byte, error) {
	// 1) cache hit
	cache.mu.RLock()
	if v, ok := cache.artistAlbums.Get(artistID); ok {
		cache.mu.RUnlock()
		if verbose {
			log.Printf("    Using cached album list for artist %s", artistID)
		}
		return v.([]byte), nil
	}
	cache.mu.RUnlock()

	// 2) offline -> no API
	if offline {
		if verbose {
			log.Printf("    Offline: no album fetch for artist %s", artistID)
		}
		return nil, nil
	}

	// 3) fetch both: primary + appears_on
	if verbose {
		log.Printf("    Fetching albums (primary + appears_on) for artist %s", artistID)
	}
	primaryBody, err1 := spotify.ArtistAlbums(artistID, limit)
	if err1 != nil {
		return nil, fmt.Errorf("ArtistAlbums failed for %s: %w", artistID, err1)
	}

	appearsBody, err2 := spotify.ArtistAlbumsAppearsOn(artistID, limit)

	// Merge into a single {"items":[...]} array
	var primary struct {
		Items []interface{} `json:"items"`
	}
	_ = json.Unmarshal(primaryBody, &primary)

	if err2 == nil && len(appearsBody) > 0 {
		var appears struct {
			Items []interface{} `json:"items"`
		}
		_ = json.Unmarshal(appearsBody, &appears)
		primary.Items = append(primary.Items, appears.Items...)
	} else if err2 != nil && verbose {
		log.Printf("warning: appears_on fetch failed for %s: %v", artistID, err2)
	}

	merged, _ := json.Marshal(primary)

	// 4) cache and return
	cache.mu.Lock()
	cache.artistAlbums.Add(artistID, merged)
	cache.mu.Unlock()

	return merged, nil
}

// fetchAlbumTracksCached returns album tracks JSON, cached by albumID.
func fetchAlbumTracksCached(a *sixdegrees.Artists, h *sixdegrees.Helper, albumID string) ([]byte, error) {
	cache.mu.RLock()
	if data, ok := cache.albumTracks.Get(albumID); ok {
		cache.mu.RUnlock()
		return data.([]byte), nil
	}
	cache.mu.RUnlock()

	tracks, err := spotify.GetAlbumTracks(albumID)
	if err != nil {
		return nil, err
	}
	cache.mu.Lock()
	cache.albumTracks.Add(albumID, tracks)
	cache.mu.Unlock()
	return tracks, nil
}

// ---------------------------
// Priority queue types (unchanged API)
// ---------------------------

type ArtistNode struct {
	artist     *sixdegrees.Artists
	depth      int
	popularity int
}

type ArtistQueue []*ArtistNode

func (aq ArtistQueue) LessBFS(i, j int) bool {
	if aq[i].depth == aq[j].depth {
		return aq[i].popularity > aq[j].popularity
	}
	return aq[i].depth < aq[j].depth
}

func (aq ArtistQueue) Less(i, j int) bool {
	if aq[i].depth == aq[j].depth {
		return aq[i].popularity > aq[j].popularity
	}
	return aq[i].depth < aq[j].depth
}

func (aq ArtistQueue) Len() int            { return len(aq) }
func (aq ArtistQueue) Swap(i, j int)       { aq[i], aq[j] = aq[j], aq[i] }
func (aq *ArtistQueue) Push(x interface{}) { *aq = append(*aq, x.(*ArtistNode)) }
func (aq *ArtistQueue) Pop() interface{} {
	old := *aq
	n := len(old)
	item := old[n-1]
	*aq = old[0 : n-1]
	return item
}

// ---------------------------
// BFS helper (local copy to keep file self-contained)
// ---------------------------

type Helper struct {
	ArtistMap map[string]*sixdegrees.Artists
	DistTo    map[string]int
	Prev      map[string]string
	Evidence  map[string]string
}

func NewHelper() *Helper {
	return &Helper{
		ArtistMap: make(map[string]*sixdegrees.Artists),
		DistTo:    make(map[string]int),
		Prev:      make(map[string]string),
		Evidence:  make(map[string]string),
	}
}

func hasTarget(a *sixdegrees.Artists, target string) bool {
	fmt.Print("hasTarget checking artist ", a.Name, " for target ", target, "\n")
	for _, t := range a.Tracks {
		if t.Artist != nil && t.Artist.Name == target {
			return true
		}
		for _, f := range t.Featured {
			if f != nil && f.Name == target {
				return true
			}
		}
	}
	return false
}

func hasFeatured(tracks []sixdegrees.Track) bool {
	for _, t := range tracks {
		if len(t.Featured) > 0 {
			return true
		}
	}
	return false
}

// ---------------------------
// Enrichment (fixed)
// ---------------------------

func enrichArtist(
	a *sixdegrees.Artists,
	h *sixdegrees.Helper,
	target string,
	found *bool,
	verbose bool,
	limit *int,
	offline bool,
) error {
	fmt.Println("Enriching artist:", a.Name)
	// If offline and we already have *featured* neighbors, skip
	if offline && len(a.Tracks) > 0 && hasFeatured(a.Tracks) {
		if verbose {
			log.Printf("    Offline & artist %s already has featured neighbors; skipping enrichment", a.Name)
		}
		return nil
	}

	if verbose {
		log.Printf("    Fetching albums/tracks for %s...", a.Name)
	}

	albumLimit := 15
	if limit != nil && *limit > 0 {
		albumLimit = *limit
	}
	var err error

	// // 1) Pull any existing tracks from DB
	// dbTracks, err := storeConn2.ListTracksByArtistID(context.Background(), a.ID, 1e6)
	// if err != nil && verbose {
	// 	log.Printf("ListTracksByArtistID error: %v", err)
	// }
	// if T, convErr := storeConn2.DBTracksToTracks(dbTracks); convErr == nil && len(T) > 0 {
	// 	a.Tracks = append(a.Tracks, T...)
	// }

	// 2) Gather albums (primary + appears_on) via cache/API (online), or skip (offline)
	var albumsBody []byte
	if !offline {
		albumsBody, err = getArtistAlbumsMergedCached(a.ID, albumLimit, false, verbose)
		if err != nil && verbose {
			log.Printf("    warning: albums fetch failed for %s: %v", a.Name, err)
		}
	}

	// 3) Parse albums and create tracks; always upsert album before tracks (FK safety)
	if albumsBody != nil {
		for i, al := range a.ParseAlbums(albumsBody) {
			if i >= albumLimit {
				break
			}

			// // Ensure album exists for FK
			// _ = storeConn2.UpsertAlbum(context.Background(), DBAlbum{
			// 	ID:              al,
			// 	PrimaryArtistID: sqlNullString(a.ID),
			// })

			tracksBytes, err := fetchAlbumTracksCached(a, nil, al)
			if err != nil {
				continue
			}
			fmt.Println("Fetched tracks for album", al, "for artist", a.Name)
			T, _ := a.CreateTracks(tracksBytes, nil)
			if len(T) == 0 {
				continue
			}
			fmt.Println("Created", len(T), "tracks for artist", a.Name)

			for _, t := range T {
				if len(t.Featured) > 0 {
					a.Tracks = append(a.Tracks, t)
				}
			}

			// Persist tracks & relationships
			// for _, t := range T {
			// 	// dba := createDBTrack(t, al)
			// 	// if err := storeConn2.UpsertTrack(context.Background(), dba); err != nil && verbose {
			// 	// 	log.Printf("warning: upsert track %s: %v", t.Name, err)
			// 	// }
			// 	// if t.Artist != nil && t.Artist.ID != "" {
			// 	// 	_ = storeConn2.UpsertArtist(context.Background(), createDBArtist(*t.Artist))
			// 	// 	_ = storeConn2.AddTrackArtist(context.Background(), t.ID, t.Artist.ID, "primary")
			// 	// }
			// 	for _, f := range t.Featured {
			// 		// if f == nil || f.ID == "" {
			// 		// 	continue
			// 		// }
			// 		// _ = storeConn2.UpsertArtist(context.Background(), createDBArtist(*f))
			// 		// _ = storeConn2.AddTrackArtist(context.Background(), t.ID, f.ID, "featured")
			// 	}
			// }
		}
	}

	time.Sleep(10 * time.Millisecond) // gentle throttle
	return nil
}

// small helper to avoid noisy sql.NullString construction
func sqlNullString(s string) (ns sql.NullString) {
	if s != "" {
		ns.String, ns.Valid = s, true
	}
	return
}

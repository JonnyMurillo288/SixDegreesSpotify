// Shared search helpers: BFS/Dijkstra enrichment, caching
// Database-first enrichment: checks DB first, then fetches remainder from Spotify API.

package main

import (
	"context"
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
	artistAlbums *lru.Cache // artistID -> []byte (albums JSON)
}

func NewCache() *Cache {
	at, _ := lru.New(1000)
	aa, _ := lru.New(1000)
	return &Cache{albumTracks: at, artistAlbums: aa}
}

var cache *Cache

func init() {
	cache = NewCache()
}

// ---------------------------
// DB-first album fetch with partial remainder API call
// ---------------------------

func (s *Store) getArtistAlbumsMergedCached(artistID string, limit int, offline, verbose bool) ([]byte, error) {
	// 1) Check cache
	cache.mu.RLock()
	if v, ok := cache.artistAlbums.Get(artistID); ok {
		cache.mu.RUnlock()
		if verbose {
			log.Printf("    Using cached album list for artist %s", artistID)
		}
		return v.([]byte), nil
	}
	cache.mu.RUnlock()

	// 2) DB query
	dbAlbums, err := s.ListAlbumsByArtistID(context.Background(), artistID, 1e6)
	numFromDB := 0
	if err == nil && len(dbAlbums) > 0 {
		numFromDB = len(dbAlbums)
		if verbose {
			log.Printf("    Found %d albums in DB for %s", numFromDB, artistID)
		}
	}

	// 3) If DB already has enough or offline, skip API
	if numFromDB >= limit || offline {
		j, _ := json.Marshal(map[string]interface{}{"items": dbAlbums})
		cache.mu.Lock()
		cache.artistAlbums.Add(artistID, j)
		cache.mu.Unlock()
		if verbose && offline {
			log.Printf("    Offline or satisfied from DB; skipping API for %s", artistID)
		}
		return j, nil
	}

	// 4) Fetch remainder from Spotify API
	apiLimit := limit - numFromDB
	if apiLimit < 0 {
		apiLimit = 0
	}
	var apiAlbums []map[string]interface{}

	if apiLimit > 0 {
		if verbose {
			log.Printf("    Fetching %d additional albums from API for %s", apiLimit, artistID)
		}
		primary, err1 := spotify.ArtistAlbums(artistID, apiLimit)
		if err1 == nil {
			var p struct {
				Items []map[string]interface{} `json:"items"`
			}
			_ = json.Unmarshal(primary, &p)
			apiAlbums = append(apiAlbums, p.Items...)
		}
		appears, err2 := spotify.ArtistAlbumsAppearsOn(artistID, apiLimit)
		if err2 == nil {
			var a struct {
				Items []map[string]interface{} `json:"items"`
			}
			_ = json.Unmarshal(appears, &a)
			apiAlbums = append(apiAlbums, a.Items...)
		}
	}

	// 5) Merge DB + API results
	all := make([]interface{}, 0, numFromDB+len(apiAlbums))
	for _, d := range dbAlbums {
		all = append(all, d)
	}
	for _, a := range apiAlbums {
		all = append(all, a)
	}

	j, _ := json.Marshal(map[string]interface{}{"items": all})
	cache.mu.Lock()
	cache.artistAlbums.Add(artistID, j)
	cache.mu.Unlock()
	return j, nil
}

// ---------------------------
// DB-first track fetch with remainder logic
// ---------------------------

func (s *Store) fetchAlbumTracksCached(a *sixdegrees.Artists, albumID string, limit int, offline, verbose bool) ([]byte, error) {
	cache.mu.RLock()
	if v, ok := cache.albumTracks.Get(albumID); ok {
		cache.mu.RUnlock()
		if verbose {
			log.Printf("    Using cached tracks for album %s", albumID)
		}
		return v.([]byte), nil
	}
	cache.mu.RUnlock()

	// Check DB
	dbTracks, err := s.ListTracksByAlbumID(context.Background(), albumID)
	numFromDB := 0
	if err == nil && len(dbTracks) > 0 {
		numFromDB = len(dbTracks)
		if verbose {
			log.Printf("    Found %d tracks in DB for album %s", numFromDB, albumID)
		}
	}

	// If DB has all or offline
	if numFromDB >= limit || offline {
		j, _ := json.Marshal(map[string]interface{}{"items": dbTracks})
		cache.mu.Lock()
		cache.albumTracks.Add(albumID, j)
		cache.mu.Unlock()
		return j, nil
	}

	// Otherwise fetch the remainder
	apiLimit := limit - numFromDB
	if apiLimit < 0 {
		apiLimit = 0
	}

	if verbose {
		log.Printf("    Fetching %d additional tracks from API for album %s", apiLimit, albumID)
	}
	tracks, err := spotify.GetAlbumTracks(albumID)
	if err != nil {
		return nil, err
	}

	// Merge DB + API
	var apiTracks struct {
		Items []map[string]interface{} `json:"items"`
	}
	_ = json.Unmarshal(tracks, &apiTracks)

	all := make([]interface{}, 0, numFromDB+len(apiTracks.Items))
	for _, d := range dbTracks {
		all = append(all, d)
	}
	for _, a := range apiTracks.Items {
		all = append(all, a)
	}
	j, _ := json.Marshal(map[string]interface{}{"items": all})
	cache.mu.Lock()
	cache.albumTracks.Add(albumID, j)
	cache.mu.Unlock()
	return j, nil
}

// ---------------------------
// Priority queue types
// ---------------------------

type ArtistNode struct {
	artist     *sixdegrees.Artists
	depth      int
	popularity int
}

type ArtistQueue []*ArtistNode

func (aq ArtistQueue) Len() int      { return len(aq) }
func (aq ArtistQueue) Swap(i, j int) { aq[i], aq[j] = aq[j], aq[i] }
func (aq ArtistQueue) Less(i, j int) bool {
	if aq[i].depth == aq[j].depth {
		return aq[i].popularity > aq[j].popularity
	}
	return aq[i].depth < aq[j].depth
}
func (aq *ArtistQueue) Push(x interface{}) { *aq = append(*aq, x.(*ArtistNode)) }
func (aq *ArtistQueue) Pop() interface{} {
	old := *aq
	n := len(old)
	item := old[n-1]
	*aq = old[:n-1]
	return item
}

// ---------------------------
// Helper struct
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

// ---------------------------
// Enrichment function
// ---------------------------

func (s *Store) enrichArtist(
	a *sixdegrees.Artists,
	h *sixdegrees.Helper,
	target string,
	found *bool,
	verbose bool,
	limit *int,
	offline bool,
) error {
	fmt.Println("Enriching artist:", a.Name)
	albumLimit := 15
	if limit != nil && *limit > 0 {
		albumLimit = *limit
	}

	// 1) Load tracks from DB first
	dbTracks, err := s.ListTracksByArtistID(context.Background(), a.ID, 1e6)
	if err == nil && len(dbTracks) > 0 {
		T, _ := s.DBTracksToTracks(dbTracks)
		if len(T) > 0 {
			if verbose {
				log.Printf("    Loaded %d tracks from DB for %s", len(T), a.Name)
			}
			a.Tracks = append(a.Tracks, T...)
			if hasFeatured(a.Tracks) {
				return nil // already enriched
			}
		}
	}

	// 2) Get albums (DB + API remainder)
	albumsBody, err := s.getArtistAlbumsMergedCached(a.ID, albumLimit, offline, verbose)
	if err != nil || albumsBody == nil {
		return err
	}

	// 3) Parse albums and build track relationships
	for i, al := range a.ParseAlbums(albumsBody) {
		if i >= albumLimit {
			break
		}

		_ = s.UpsertAlbum(context.Background(), DBAlbum{
			ID:              al,
			PrimaryArtistID: sqlNullString(a.ID),
		})

		tracksBody, err := s.fetchAlbumTracksCached(a, al, albumLimit, offline, verbose)
		if err != nil {
			continue
		}

		T, _ := a.CreateTracks(tracksBody, nil)
		if len(T) == 0 {
			continue
		}

		for _, t := range T {
			if len(t.Featured) > 0 {
				a.Tracks = append(a.Tracks, t)
			}
			dba := createDBTrack(t, al)
			_ = s.UpsertTrack(context.Background(), dba)

			if t.Artist != nil && t.Artist.ID != "" {
				_ = s.UpsertArtist(context.Background(), createDBArtist(*t.Artist))
				_ = s.AddTrackArtist(context.Background(), t.ID, t.Artist.ID, "primary")
			}
			for _, f := range t.Featured {
				if f == nil || f.ID == "" {
					continue
				}
				_ = s.UpsertArtist(context.Background(), createDBArtist(*f))
				_ = s.AddTrackArtist(context.Background(), t.ID, f.ID, "featured")
			}
		}
	}

	time.Sleep(10 * time.Millisecond)
	return nil
}

// ---------------------------
// Small helpers
// ---------------------------

func hasFeatured(tracks []sixdegrees.Track) bool {
	for _, t := range tracks {
		if len(t.Featured) > 0 {
			return true
		}
	}
	return false
}

func sqlNullString(s string) (ns sql.NullString) {
	if s != "" {
		ns.String, ns.Valid = s, true
	}
	return
}

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

func dedupeAlbums(resp *albumResponse) {
	seen := make(map[string]bool)
	out := make([]struct {
		ID      string `json:"id"`
		Artists []struct {
			Name string `json:"name"`
		} `json:"artists"`
	}, 0, len(resp.Items))

	for _, it := range resp.Items {
		if seen[it.ID] {
			continue
		}
		seen[it.ID] = true
		out = append(out, it)
	}

	resp.Items = out
}

// ---------------------------
// DB-first album fetch with partial remainder API call
// ---------------------------

func (s *Store) getArtistAlbumsMergedCached(artistID string, limit int, offline, verbose bool) (albumResponse, error) {
	// 1) Check cache
	// cache.mu.RLock()
	// if v, ok := cache.artistAlbums.Get(artistID); ok {
	// 	cache.mu.RUnlock()
	// 	if verbose {
	// 		log.Printf("    Using cached album list for artist %s", artistID)
	// 	}
	// 	// return v.([]byte), nil
	// }
	// cache.mu.RUnlock()

	// 2) DB query
	dbAlbums, err := s.ListAlbumsByArtistID(context.Background(), artistID, limit)
	if err != nil {
		fmt.Println("There is an error shared_search_funcs.go Line 87: ", err)
	}
	apiAlbums, err := s.ConvertDBAlbumsToResponse(dbAlbums, "http://localhost/album", limit, 0)
	fmt.Printf("Number of apiAlbums from ConvertDBAlbumsToResponse: %d\n", len(apiAlbums.Items))
	numFromDB := 0
	if err == nil && len(dbAlbums) > 0 {
		numFromDB = len(apiAlbums.Items)
		if verbose {
			log.Printf("    Found %d albums in DB for %s", numFromDB, artistID)
		}
	}

	// 3) If DB already has enough or offline, skip API
	// if numFromDB >= limit || offline {
	// 	j, _ := json.Marshal(map[string]interface{}{"items": dbAlbumsConv})
	// 	// cache.mu.Lock()
	// 	// cache.artistAlbums.Add(artistID, j)
	// 	// cache.mu.Unlock()
	// 	// if verbose && offline {
	// 	// 	log.Printf("    Offline or satisfied from DB; skipping API for %s", artistID)
	// 	// }
	// 	// return j, nil
	// }

	// 4) Fetch remainder from Spotify API
	apiLimit := limit - len(apiAlbums.Items)
	if apiLimit <= 0 {
		apiLimit = 0
	}

	if apiLimit > 0 {
		if verbose {
			log.Printf("    Fetching %d additional albums from API for %s\n", apiLimit, artistID)
		}
		primary, err1 := spotify.ArtistAlbums(artistID, apiLimit)
		if err1 == nil {
			var p albumResponse
			_ = json.Unmarshal(primary, &p)
			apiAlbums.Items = append(apiAlbums.Items, p.Items...)
			fmt.Println("Number of primary albums fetched from API:", len(p.Items))
		}
		appears, err2 := spotify.ArtistAlbumsAppearsOn(artistID, apiLimit)
		if err2 == nil {
			var a albumResponse
			_ = json.Unmarshal(appears, &a)
			apiAlbums.Items = append(apiAlbums.Items, a.Items...)
		}
	}
	// Drop apiAlbums.Items that are duplicate Items.ID
	dedupeAlbums(&apiAlbums)
	return apiAlbums, nil
}

// ---------------------------
// DB-first track fetch with remainder logic
// ---------------------------
func dedupeTracks(resp *trackResponse) {
	seen := make(map[string]bool)
	out := make([]struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Artists []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"artists"`
	}, 0, len(resp.Items))

	for _, it := range resp.Items {
		if seen[it.ID] {
			continue
		}
		seen[it.ID] = true
		out = append(out, it)
	}

	resp.Items = out
}

func (s *Store) fetchAlbumTracksCached(a *sixdegrees.Artists, albumID string, limit int, offline, verbose bool) (trackResponse, error) {
	// cache.mu.RLock()
	if _, ok := cache.albumTracks.Get(albumID); ok {
		cache.mu.RUnlock()
		if verbose {
			log.Printf("    Using cached tracks for album %s", albumID)
		}
		// Don't return
		// return v.([]byte), nil
	}
	// cache.mu.RUnlock()

	// Check DB
	dbTracks, err := s.ListTracksByAlbumID(context.Background(), albumID)
	numFromDB := 0
	if err == nil && len(dbTracks) > 0 {
		numFromDB = len(dbTracks)
		if verbose {
			log.Printf("    Found %d tracks in DB for album %s", numFromDB, albumID)
		}
	}

	// convert the dbTracks object to type similar to trackResponse that API calls
	apiTracks, err := s.ConvertDBTracksToResponse(dbTracks, "http://localhost/tracks", a.Name, 10, 0)
	if err != nil {
		panic(err)
	}

	// If DB has all or offline
	// if numFromDB >= limit || offline {
	// 	j, _ := json.Marshal(map[string]interface{}{"items": dbTracksConv.Items})
	// 	cache.mu.Lock()
	// 	cache.albumTracks.Add(albumID, j)
	// 	cache.mu.Unlock()
	// 	return j, nil
	// }

	// Otherwise fetch the remainder
	apiLimit := limit - numFromDB
	if apiLimit <= 0 {
		apiLimit = 0
	}

	if verbose {
		log.Printf("    Fetching %d additional tracks from API for album %s -> (%d - %d = %d)", apiLimit, albumID, limit, numFromDB, apiLimit)
	}

	tracks, err := spotify.GetAlbumTracks(albumID)
	if err != nil {
		return trackResponse{}, err
	}
	var dbT trackResponse
	_ = json.Unmarshal(tracks, &dbT)

	apiTracks.Items = append(apiTracks.Items, dbT.Items...)

	dedupeTracks(&apiTracks)
	return apiTracks, nil
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
			// Only append tracks that have featured artists
			if hasFeatured(a.Tracks) {
				a.Tracks = append(a.Tracks, T...)
			}
		}
	}

	// 2) Get albums (DB + API remainder)
	albumsBody, err := s.getArtistAlbumsMergedCached(a.ID, albumLimit, offline, verbose)
	if err != nil {
		return err
	}

	// 3) Parse albums and build track relationships
	// for album in albumsBody.Items
	for i, al := range albumsBody.Items {
		if i == 0 {
			fmt.Printf("Number of albums for %d:%s\n", len(albumsBody.Items), a.Name)
		}
		if i >= albumLimit {
			break
		}

		_ = s.UpsertAlbum(context.Background(), DBAlbum{
			ID:              al.ID,
			PrimaryArtistID: sqlNullString(a.ID),
		})
		for _, art := range al.Artists {
			_ = s.UpsertArtist(context.Background(), DBArtist{
				Name: art.Name,
			})
			_ = s.AddAlbumArtist(context.Background(), al.ID, art.Name)

			tracksBody, err := s.fetchAlbumTracksCached(a, al.ID, albumLimit, offline, verbose)
			if err != nil {
				continue
			}
			j, _ := json.Marshal(tracksBody)
			T, _ := a.CreateTracks(j, nil)
			if len(T) == 0 {
				continue
			}

			for _, t := range T {
				if len(t.Featured) > 0 {
					a.Tracks = append(a.Tracks, t)
				}
				dba := createDBTrack(t, al.ID)
				fmt.Println(dba.AlbumID)
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

func (s *Store) ConvertDBTracksToResponse(dbTracks []DBTrack, href, artist_name string, limit, offset int) (trackResponse, error) {
	var resp trackResponse
	resp.Href = href
	resp.Limit = limit
	resp.Offset = offset
	resp.Total = len(dbTracks)

	var featured_artists []*sixdegrees.Artists

	for _, dbt := range dbTracks {
		featured_artists = nil
		art, err := s.ListFeaturedArtistsForTrack(context.Background(), dbt.ID)
		if err != nil {
			fmt.Println("Error ", err)
		}
		for _, a := range art {
			artist, _ := s.DBArtistsToArtists(a)
			featured_artists = append(featured_artists, artist)
		}
		item := struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Artists []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"artists"`
		}{
			ID:   dbt.ID,
			Name: dbt.Name,
		}

		//

		// Add artist info if available
		if dbt.PrimaryArtistID.Valid {
			item.Artists = append(item.Artists, struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			}{
				ID:   dbt.PrimaryArtistID.String,
				Name: artist_name, // optional — fill in if you can look up artist name
			})
		}
		// Get the featured artists in each track
		for _, art := range featured_artists {
			item.Artists = append(item.Artists, struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			}{
				ID:   art.ID,
				Name: art.Name,
			})
		}

		resp.Items = append(resp.Items, item)
	}

	return resp, nil
}

func (s *Store) ConvertDBAlbumsToResponse(dbAlbums []DBAlbum, href string, limit, offset int) (albumResponse, error) {
	var resp albumResponse

	for _, dba := range dbAlbums {
		item := struct {
			ID      string `json:"id"`
			Artists []struct {
				Name string `json:"name"`
			} `json:"artists"`
		}{
			ID: dba.ID,
		}

		// Look up artist name if possible
		var artistName string
		if dba.PrimaryArtistID.Valid {
			artist, err := s.GetArtistByID(context.Background(), dba.PrimaryArtistID.String)
			if err == nil && artist.Name != "" {
				artistName = artist.Name
			}
		}

		// Append at least one artist entry
		item.Artists = append(item.Artists, struct {
			Name string `json:"name"`
		}{
			Name: artistName,
		})

		resp.Items = append(resp.Items, item)
	}

	return resp, nil
}

func dedupeAlbumItems(resp *trackResponse) {
	seen := make(map[string]bool)
	out := make([]struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Artists []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"artists"`
	}, 0, len(resp.Items))

	for _, it := range resp.Items {
		if seen[it.ID] {
			continue
		}
		seen[it.ID] = true
		out = append(out, it)
	}

	resp.Items = out
}

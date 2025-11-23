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
	"golang.org/x/sync/errgroup"
)

// ---------------------------
// API response types
// ---------------------------

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

// type albumResponse struct {
// 	Items []struct {
// 		ID      string `json:"id"`
// 		Artists []struct {
// 			Name string `json:"name"`
// 		} `json:"artists"`
// 	} `json:"items"`
// }

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
// Dedup helpers
// ---------------------------

func dedupeAlbums(resp *albumResponse) {
	if len(resp.Items) == 0 {
		return
	}
	seen := make(map[string]struct{}, len(resp.Items))
	out := resp.Items[:0]

	for _, it := range resp.Items {
		if _, ok := seen[it.ID]; ok {
			continue
		}
		seen[it.ID] = struct{}{}
		out = append(out, it)
	}

	resp.Items = out
}

func dedupeTracks(resp *trackResponse) {
	if len(resp.Items) == 0 {
		return
	}
	seen := make(map[string]struct{}, len(resp.Items))
	out := resp.Items[:0]

	for _, it := range resp.Items {
		if _, ok := seen[it.ID]; ok {
			continue
		}
		seen[it.ID] = struct{}{}
		out = append(out, it)
	}

	resp.Items = out
}

type albumResponse struct {
	Items []struct {
		ID      string `json:"id"`
		Artists []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"artists"`
	} `json:"items"`
}

// ---------------------------
// DB-first album fetch with cache + partial API call
// ---------------------------

func (s *Store) getArtistAlbumsMergedCached(
	artistID string,
	limit int,
	offline, verbose bool,
) (albumResponse, int, error) {
	start := time.Now()

	// 1) Cache lookup
	cache.mu.RLock()
	if v, ok := cache.artistAlbums.Get(artistID); ok {
		cache.mu.RUnlock()
		if verbose {
			log.Printf("    [albums] cache hit for artist %s", artistID)
		}
		var resp albumResponse
		if err := json.Unmarshal(v.([]byte), &resp); err != nil {
			return albumResponse{}, 500, err
		}
		if limit > 0 && len(resp.Items) > limit {
			resp.Items = resp.Items[:limit]
		}
		if verbose {
			log.Printf("    getArtistAlbumsMergedCached (cache) for %s took %s", artistID, time.Since(start))
		}
		return resp, 200, nil
	}
	cache.mu.RUnlock()

	if verbose {
		log.Printf("    [albums] cache miss for artist %s; querying DB/API", artistID)
	}

	// 2) DB query (DB-first)
	dbAlbums, err := s.ListAlbumsByArtistID(context.Background(), artistID, limit)
	if err != nil {
		log.Printf("    ListAlbumsByArtistID error for %s: %v", artistID, err)
	}

	apiAlbums, err := s.ConvertDBAlbumsToResponse(dbAlbums, "http://localhost/album", limit, 0)
	if err != nil {
		return albumResponse{}, 500, err
	}
	numFromDB := len(apiAlbums.Items)
	if verbose {
		log.Printf("    DB returned %d albums for %s", numFromDB, artistID)
	}

	// 3) If DB has enough or we're offline
	if numFromDB >= limit || offline {
		dedupeAlbums(&apiAlbums)
		if !offline {
			if j, err := json.Marshal(apiAlbums); err == nil {
				cache.mu.Lock()
				cache.artistAlbums.Add(artistID, j)
				cache.mu.Unlock()
			}
		}
		if verbose {
			log.Printf("    Satisfied albums from DB for %s (offline=%v); took %s",
				artistID, offline, time.Since(start))
		}
		return apiAlbums, 200, nil
	}

	// 4) Fetch remainder from Spotify API (concurrent primary + appears-on)
	apiLimit := limit - numFromDB
	if apiLimit < 0 {
		apiLimit = 0
	}

	var statusCode int
	if apiLimit > 0 && !offline {
		if verbose {
			log.Printf("    Fetching %d additional albums from API for %s", apiLimit, artistID)
		}

		var primaryRaw, appearsRaw []byte
		var primaryErr, appearsErr error

		g, _ := errgroup.WithContext(context.Background())

		g.Go(func() error {
			var status int
			primaryRaw, status, primaryErr = spotify.ArtistAlbums(artistID, apiLimit)
			if status == 429 {
				statusCode = 429
				return fmt.Errorf("spotify api rate limit exceeded")
			}
			return primaryErr
		})

		g.Go(func() error {
			appearsRaw, appearsErr = spotify.ArtistAlbumsAppearsOn(artistID, apiLimit)
			return appearsErr
		})

		if err := g.Wait(); err != nil {
			// if rate limited, propagate; otherwise continue with DB-only
			if statusCode == 429 {
				return albumResponse{}, 429, err
			}
			log.Printf("    Error fetching albums from Spotify for %s: %v", artistID, err)
		}

		if primaryErr == nil && len(primaryRaw) > 0 {
			var p albumResponse
			_ = json.Unmarshal(primaryRaw, &p)
			apiAlbums.Items = append(apiAlbums.Items, p.Items...)
		}
		if appearsErr == nil && len(appearsRaw) > 0 {
			var a albumResponse
			_ = json.Unmarshal(appearsRaw, &a)
			apiAlbums.Items = append(apiAlbums.Items, a.Items...)
		}
	}

	dedupeAlbums(&apiAlbums)

	// cache final result
	if !offline {
		if j, err := json.Marshal(apiAlbums); err == nil {
			cache.mu.Lock()
			cache.artistAlbums.Add(artistID, j)
			cache.mu.Unlock()
		}
	}

	if verbose {
		log.Printf("    getArtistAlbumsMergedCached for %s took %s", artistID, time.Since(start))
	}
	return apiAlbums, 200, nil
}

// ---------------------------
// DB-first track fetch with cache + remainder logic
// ---------------------------

func (s *Store) fetchAlbumTracksCached(
	a *sixdegrees.Artists,
	albumID string,
	limit int,
	offline, verbose bool,
) (trackResponse, error) {
	start := time.Now()

	// 1) Cache lookup
	cache.mu.RLock()
	if v, ok := cache.albumTracks.Get(albumID); ok {
		cache.mu.RUnlock()
		if verbose {
			log.Printf("    [tracks] cache hit for album %s", albumID)
		}
		var resp trackResponse
		if err := json.Unmarshal(v.([]byte), &resp); err != nil {
			return trackResponse{}, err
		}
		if limit > 0 && len(resp.Items) > limit {
			resp.Items = resp.Items[:limit]
		}
		if verbose {
			log.Printf("    fetchAlbumTracksCached (cache) for %s took %s", albumID, time.Since(start))
		}
		return resp, nil
	}
	cache.mu.RUnlock()

	if verbose {
		log.Printf("    [tracks] cache miss for album %s; querying DB/API", albumID)
	}

	// 2) DB
	dbTracks, err := s.ListTracksByAlbumID(context.Background(), albumID)
	if err != nil {
		log.Printf("    ListTracksByAlbumID error for album %s: %v", albumID, err)
	}
	numFromDB := len(dbTracks)
	if verbose {
		log.Printf("    DB returned %d tracks for album %s", numFromDB, albumID)
	}

	apiTracks, err := s.ConvertDBTracksToResponse(dbTracks, "http://localhost/tracks", a.Name, limit, 0)
	if err != nil {
		return trackResponse{}, err
	}

	// 3) DB-only or offline
	if numFromDB >= limit || offline {
		dedupeTracks(&apiTracks)
		if !offline {
			if j, err := json.Marshal(apiTracks); err == nil {
				cache.mu.Lock()
				cache.albumTracks.Add(albumID, j)
				cache.mu.Unlock()
			}
		}
		if verbose {
			log.Printf("    Satisfied tracks from DB for album %s (offline=%v); took %s",
				albumID, offline, time.Since(start))
		}
		return apiTracks, nil
	}

	// 4) Fetch remainder from Spotify
	apiLimit := limit - numFromDB
	if apiLimit < 0 {
		apiLimit = 0
	}

	if apiLimit > 0 && !offline {
		if verbose {
			log.Printf("    Fetching additional tracks from API for album %s (need %d more)", albumID, apiLimit)
		}

		tracksRaw, err := spotify.GetAlbumTracks(albumID)
		if err != nil {
			log.Printf("    GetAlbumTracks error for %s: %v", albumID, err)
		} else {
			var tResp trackResponse
			_ = json.Unmarshal(tracksRaw, &tResp)
			apiTracks.Items = append(apiTracks.Items, tResp.Items...)
		}
	}

	dedupeTracks(&apiTracks)

	// Cache final
	if !offline {
		if j, err := json.Marshal(apiTracks); err == nil {
			cache.mu.Lock()
			cache.albumTracks.Add(albumID, j)
			cache.mu.Unlock()
		}
	}

	if verbose {
		log.Printf("    fetchAlbumTracksCached for album %s took %s", albumID, time.Since(start))
	}
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
//
// NOTE: this is where most time used to vanish due to:
//  - massive redundant SQL upserts inside nested loops
//  - no protection around a.Tracks when running concurrently
//  - lots of Spotify calls per artist
//

func (s *Store) enrichArtist(
	a *sixdegrees.Artists,
	h *sixdegrees.Helper,
	target string,
	found *bool,
	verbose bool,
	limit *int,
	offline bool,
) error {
	if verbose {
		log.Printf("Enriching artist: %s", a.Name)
	}
	start := time.Now()

	albumLimit := 15
	if limit != nil && *limit > 0 {
		albumLimit = *limit
	}

	// 1) Get albums (DB + API remainder, cached)
	albumsBody, status, err := s.getArtistAlbumsMergedCached(a.ID, albumLimit, offline, verbose)
	if err != nil {
		if status == 429 {
			// Bubble up rate limits so caller can backoff
			return err
		}
		log.Printf("    getArtistAlbumsMergedCached error for %s: %v", a.Name, err)
		return err
	}

	if verbose {
		log.Printf("    %s has %d albums after merge/dedupe", a.Name, len(albumsBody.Items))
	}

	// 2) Process albums in parallel
	g, _ := errgroup.WithContext(context.Background())

	// protect a.Tracks against concurrent appends
	var tracksMu sync.Mutex

	for i, al := range albumsBody.Items {
		if i >= albumLimit {
			break
		}
		alCopy := al // capture loop variable

		g.Go(func() error {
			ctx := context.Background()

			// Ensure album in DB
			if err := s.UpsertAlbum(ctx, DBAlbum{
				ID:              alCopy.ID,
				PrimaryArtistID: sqlNullString(a.ID),
			}); err != nil && verbose {
				log.Printf("    UpsertAlbum error for %s: %v", alCopy.ID, err)
			}

			// Upsert album artists + relationship
			// We could batch this too, but this is less hot than tracks.
			for _, art := range alCopy.Artists {
				name := art.Name
				if name == "" {
					continue
				}
				if err := s.UpsertArtist(ctx, DBArtist{Name: name}); err != nil && verbose {
					log.Printf("    UpsertArtist error for %s: %v", name, err)
				}
				if err := s.AddAlbumArtist(ctx, alCopy.ID, name); err != nil && verbose {
					log.Printf("    AddAlbumArtist error album=%s artist=%s: %v", alCopy.ID, name, err)
				}
			}

			// Fetch album tracks (DB + API, cached)
			tracksBody, err := s.fetchAlbumTracksCached(a, alCopy.ID, albumLimit, offline, verbose)
			if err != nil {
				if verbose {
					log.Printf("    fetchAlbumTracksCached error for album %s: %v", alCopy.ID, err)
				}
				return nil // non-fatal; continue with other albums
			}

			// Convert to sixdegrees.Track
			j, _ := json.Marshal(tracksBody)
			T, _ := a.CreateTracks(j, nil)
			if len(T) == 0 {
				return nil
			}

			// BATCH upserts for tracks + track-artist edges to avoid 1000s of SQL calls
			tracksToUpsert := make([]DBTrack, 0, len(T))
			trackArtistsPrimary := make([]TrackArtistEdge, 0, len(T))
			trackArtistsFeatured := make([]TrackArtistEdge, 0, len(T))

			for _, t := range T {
				// Add to in-memory graph only if featured artists exist
				if len(t.Featured) > 0 {
					tracksMu.Lock()
					a.Tracks = append(a.Tracks, t)
					tracksMu.Unlock()
				}

				dbTrack := createDBTrack(t, alCopy.ID)
				tracksToUpsert = append(tracksToUpsert, dbTrack)

				// Primary artist
				if t.Artist != nil && t.Artist.ID != "" {
					if err := s.UpsertArtist(ctx, createDBArtist(*t.Artist)); err != nil && verbose {
						log.Printf("    UpsertArtist (primary) error for %s: %v", t.Artist.Name, err)
					}
					trackArtistsPrimary = append(trackArtistsPrimary, TrackArtistEdge{
						TrackID:  t.ID,
						ArtistID: t.Artist.ID,
						Role:     "primary",
					})
				}

				// Featured artists
				for _, f := range t.Featured {
					if f == nil || f.ID == "" {
						continue
					}
					if err := s.UpsertArtist(ctx, createDBArtist(*f)); err != nil && verbose {
						log.Printf("    UpsertArtist (featured) error for %s: %v", f.Name, err)
					}
					trackArtistsFeatured = append(trackArtistsFeatured, TrackArtistEdge{
						TrackID:  t.ID,
						ArtistID: f.ID,
						Role:     "featured",
					})
				}
			}

			// Do bulk upserts for tracks & track-artist edges
			if err := s.UpsertTracksBulk(ctx, tracksToUpsert); err != nil && verbose {
				log.Printf("    UpsertTracksBulk error for album %s: %v", alCopy.ID, err)
			}

			if err := s.AddTrackArtistsBulk(ctx, append(trackArtistsPrimary, trackArtistsFeatured...)); err != nil && verbose {
				log.Printf("    AddTrackArtistsBulk error for album %s: %v", alCopy.ID, err)
			}

			return nil
		})
	}

	if err := g.Wait(); err != nil && verbose {
		log.Printf("    enrichArtist parallel processing error for %s: %v", a.Name, err)
	}

	if verbose {
		log.Printf("    enrichArtist for %s took %s", a.Name, time.Since(start))
	}

	return nil
}

// ---------------------------
// Small helpers
// ---------------------------

func sqlNullString(s string) (ns sql.NullString) {
	if s != "" {
		ns.String, ns.Valid = s, true
	}
	return
}

// ---------------------------
// Converters
// ---------------------------

func (s *Store) ConvertDBTracksToResponse(
	dbTracks []DBTrack,
	href, artistName string,
	limit, offset int,
) (trackResponse, error) {
	var resp trackResponse
	resp.Href = href
	resp.Limit = limit
	resp.Offset = offset
	resp.Total = len(dbTracks)

	resp.Items = make([]struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Artists []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"artists"`
	}, 0, len(dbTracks))

	for _, dbt := range dbTracks {
		// Get featured artists for this track
		art, err := s.ListFeaturedArtistsForTrack(context.Background(), dbt.ID)
		if err != nil {
			log.Printf("    ListFeaturedArtistsForTrack error for %s: %v", dbt.ID, err)
		}

		var featuredArtists []*sixdegrees.Artists
		for _, aDB := range art {
			artist, err := s.DBArtistsToArtists(aDB)
			if err != nil {
				log.Printf("    DBArtistsToArtists error: %v", err)
				continue
			}
			featuredArtists = append(featuredArtists, artist)
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

		// Primary artist from DBTrack
		if dbt.PrimaryArtistID.Valid {
			item.Artists = append(item.Artists, struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			}{
				ID:   dbt.PrimaryArtistID.String,
				Name: artistName,
			})
		}

		// Featured artists
		for _, art := range featuredArtists {
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

func (s *Store) ConvertDBAlbumsToResponse(
	dbAlbums []DBAlbum,
	href string,
	limit, offset int,
) (albumResponse, error) {

	var resp albumResponse

	// Prepare Items slice with capacity
	resp.Items = make([]struct {
		ID      string `json:"id"`
		Artists []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"artists"`
	}, 0, len(dbAlbums))

	for _, dba := range dbAlbums {

		// Prepare album item
		item := struct {
			ID      string `json:"id"`
			Artists []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"artists"`
		}{
			ID: dba.ID,
		}

		// Look up primary artist if present
		if dba.PrimaryArtistID.Valid {
			art, err := s.GetArtistByID(context.Background(), dba.PrimaryArtistID.String)
			if err == nil && art.ID != "" {
				item.Artists = append(item.Artists, struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				}{
					ID:   art.ID,
					Name: art.Name,
				})
			}
		}

		resp.Items = append(resp.Items, item)
	}

	return resp, nil
}

// ---------------------------
// Batch helpers (you must implement these on Store)
// ---------------------------

// TrackArtistEdge represents a (track, artist, role) row.
type TrackArtistEdge struct {
	TrackID  string
	ArtistID string
	Role     string // "primary" or "featured"
}

// UpsertTracksBulk should insert or update all tracks in a single statement or transaction.
// Implement this on your Store (e.g. using COPY, INSERT ... ON CONFLICT, etc.).
func (s *Store) UpsertTracksBulk(ctx context.Context, tracks []DBTrack) error {
	if len(tracks) == 0 {
		return nil
	}

	// TODO: implement real batch logic.
	// For now, fall back to per-track upsert as a compatibility shim.
	for _, t := range tracks {
		if err := s.UpsertTrack(ctx, t); err != nil {
			return err
		}
	}
	return nil
}

// AddTrackArtistsBulk should insert track-artist edges in bulk.
// Again, implement as a single DB operation if possible.
func (s *Store) AddTrackArtistsBulk(ctx context.Context, edges []TrackArtistEdge) error {
	if len(edges) == 0 {
		return nil
	}

	// TODO: implement real batch logic.
	// Compatibility fallback:
	for _, e := range edges {
		if err := s.AddTrackArtist(ctx, e.TrackID, e.ArtistID, e.Role); err != nil {
			return err
		}
	}
	return nil
}

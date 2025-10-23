package main

import (
	"container/heap"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	sixdegrees "github.com/Jonnymurillo288/SixDegreesSpotify/sixDegrees"
	"github.com/Jonnymurillo288/SixDegreesSpotify/spotify"
)

// Priority queue for artists based on popularity
type ArtistQueue []*sixdegrees.Artists

func (aq ArtistQueue) Len() int { return len(aq) }
func (aq ArtistQueue) Less(i, j int) bool {
	// Change < to > if you want more popular first
	return aq[i].Popularity > aq[j].Popularity // More popular first
}
func (aq ArtistQueue) Swap(i, j int) { aq[i], aq[j] = aq[j], aq[i] }

func (aq *ArtistQueue) Push(x interface{}) {
	*aq = append(*aq, x.(*sixdegrees.Artists))
}

func (aq *ArtistQueue) Pop() interface{} {
	old := *aq
	n := len(old)
	item := old[n-1]
	*aq = old[0 : n-1]
	return item
}

var albumCache = make(map[string][]byte)

// global store pointer is defined in this package and set in main.go
var store *Store

// This function checks if we have any cached albums and their respective tracks
func fetchAlbumTracksCached(a *sixdegrees.Artists, h *sixdegrees.Helper, albumID string) ([]byte, error) {
	// 1. check memory cache
	if data, ok := albumCache[albumID]; ok {
		fmt.Printf("Got a cached Album for %s", a.Name)
		return data, nil
	}

	// 2. call API
	tracks, err := spotify.GetAlbumTracks(albumID)
	if err != nil {
		return nil, err
	}

	// 3. store to cache as bytes (for reuse)
	if data, err := json.Marshal(tracks); err == nil {
		albumCache[albumID] = data
	}

	return tracks, nil
}

// RunSearchOpts performs a bounded/unbounded BFS search between artists.
func RunSearchOpts(start, target *sixdegrees.Artists, maxDepth int, verbose bool, limit *int) (*sixdegrees.Helper, []string, bool) {
	h := sixdegrees.NewHelper()
	h.ArtistMap[start.Name] = start
	h.DistTo[start.Name] = 0

	queue := &ArtistQueue{}
	heap.Init(queue)
	heap.Push(queue, start)
	visited := map[string]bool{start.Name: true}
	found := false

	// Functions for adding
	// UpsertArtist, UpsertAlbum, UpsertTrack, AddTrackArtist, SaveArtistWithTracks
	for queue.Len() > 0 && !found {
		current := heap.Pop(queue).(*sixdegrees.Artists)

		if verbose {
			log.Printf("[Depth %d] Exploring %s (%d tracks)", h.DistTo[current.Name], current.Name, len(current.Tracks))
		}

		// Depth guard
		if maxDepth >= 0 && h.DistTo[current.Name] >= maxDepth {
			continue
		}

		for _, tr := range current.Tracks {
			if tr.Artist.Name == target.Name {
				h.Prev[target.Name] = current.Name
				h.Evidence[target.Name] = tr.Name
				found = true
				break
			}

			for _, feat := range tr.Featured {
				if feat.Name == "" || feat.Name == current.Name {
					continue
				}
				if visited[feat.Name] {
					continue
				}
				visited[feat.Name] = true

				h.Prev[feat.Name] = current.Name
				h.Evidence[feat.Name] = tr.Name
				h.DistTo[feat.Name] = h.DistTo[current.Name] + 1
				h.ArtistMap[feat.Name] = feat

				if verbose {
					log.Printf("  ↳ Found feature: %s (via %s)", feat.Name, tr.Name)
				}

				// Fetch this feature’s albums/tracks only once
				if err := enrichArtist(feat, h, target.Name, &found, verbose, limit); err != nil && verbose {
					log.Printf("    (warning: %v)", err)
				}
				if found {
					break
				}

				// Check if target found among features’ tracks
				if hasTarget(feat, target.Name) {
					h.Prev[target.Name] = feat.Name
					found = true
					break
				}

				heap.Push(queue, feat)
			}
			if found {
				break
			}
		}
	}

	if found {
		return h, h.ReconstructPath(start.Name, target.Name), true
	}
	return h, nil, false
}

// Functions for adding
// UpsertArtist, UpsertAlbum, UpsertTrack, AddTrackArtist, SaveArtistWithTracks
// Enrich artist data by fetching albums and tracks if not already populated.
func enrichArtist(a *sixdegrees.Artists, h *sixdegrees.Helper, target string, found *bool, verbose bool, limit *int) error {
	if len(a.Tracks) > 0 || *found {
		return nil
	}
	if verbose {
		log.Printf("    Fetching albums/tracks for %s...", a.Name)
	}
	// honor provided limit if set; default to 15
	albumLimit := 15
	if limit != nil && *limit > 0 {
		albumLimit = *limit
	}
	body, err := spotify.ArtistAlbums(a.ID, albumLimit)
	if err != nil {
		return fmt.Errorf("albums fetch failed for %s: %w", a.Name, err)
	}

	// Upsert the Artist if store is available
	if store != nil {
		dba := createDBArtist(*a)
		if err := store.UpsertArtist(context.Background(), dba); err != nil && verbose {
			log.Printf("    warning: error upserting artist %s: %v", a.Name, err)
		}
	}

	// cap iterations by albumLimit as well
	for i, al := range a.ParseAlbums(body) {
		if i >= albumLimit {
			return nil
		}
		tracks, err := fetchAlbumTracksCached(a, h, al)
		if err != nil {
			continue
		}
		T, _ := a.CreateTracks(tracks, h)
		if store != nil {
			for _, t := range T {
				dba := createDBTrack(t, al) // store the track
				if err := store.UpsertTrack(context.Background(), dba); err != nil && verbose {
					log.Printf("warning: error upserting track %s: %v", t.Name, err)
				}
				// store the main artist
				if err := store.AddTrackArtist(context.Background(), t.ID, t.Artist.ID, "primary"); err != nil && verbose {
					log.Printf("warning: error upserting artist_track for %s: %v", t.Artist.Name, err)
				}
				// if there is a featured, store featured, then link the featured to the main
				if len(t.Featured) > 0 {
					for _, art := range t.Featured {
						dba := createDBArtist(*art)
						if err := store.UpsertArtist(context.Background(), dba); err != nil && verbose {
							log.Printf("warning: error upserting track %s: %v", art.Name, err)
						}
						if err := store.AddTrackArtist(context.Background(), t.ID, art.ID, "featured"); err != nil && verbose {
							log.Printf("warning: error upserting artist_track for %s: %v", art.Name, err)
						}
					}
				}
			}
		}

		a.Tracks = append(a.Tracks, T...)

		// check if any of these tracks hit the target mid-fetch
		if hasTarget(a, target) {
			*found = true
			return nil
		}
	}
	time.Sleep(300 * time.Millisecond) // small delay to respect API rate limits
	return nil
}

// Utility to check if any track by this artist matches the target
func hasTarget(a *sixdegrees.Artists, target string) bool {
	for _, t := range a.Tracks {
		if t.Artist.Name == target {
			return true
		}
		for _, f := range t.Featured {
			if f.Name == target {
				return true
			}
		}
	}
	return false
}

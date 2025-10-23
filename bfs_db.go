package main

import (
	"container/heap"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	sixdegrees "github.com/Jonnymurillo288/SixDegreesSpotify/sixDegrees"
)

// RunSearchOpts performs a bounded/unbounded BFS search between artists.
func RunSearchOptsDB(start, target *sixdegrees.Artists, maxDepth int, verbose bool, limit *int) (*sixdegrees.Helper, []string, bool) {
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
				if err := enrichArtistDB(feat, h, target.Name, &found, verbose, limit); err != nil && verbose {
					log.Printf("(warning: %v)", err)
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

// Functions for adding tracks to an artist item
// UpsertArtist, UpsertAlbum, UpsertTrack, AddTrackArtist, SaveArtistWithTracks
// Enrich artist data by fetching albums and tracks if not already populated.
func enrichArtistDB(a *sixdegrees.Artists, h *sixdegrees.Helper, target string, found *bool, verbose bool, limit *int) error {
	if len(a.Tracks) > 0 || *found {
		return nil
	}
	if verbose {
		log.Printf("    Fetching albums/tracks for %s...", a.Name)
	}
	// return the tracks of the artist
	body, err := store.ListTracksByArtistID(context.Background(), a.ID, 1e6)
	if err != nil {
		return fmt.Errorf("albums fetch failed for %s: %w", a.Name, err)
	}
	tracks, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("error with marshalling tracks")
	}
	// Create the tracks
	T, _ := a.CreateTracks(tracks, h)
	a.Tracks = append(a.Tracks, T...)

	// check if any of these tracks hit the target mid-fetch
	if hasTarget(a, target) {
		*found = true
		return nil
	}
	time.Sleep(300 * time.Millisecond) // small delay to respect API rate limits
	return nil
}

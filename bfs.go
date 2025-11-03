package main

import (
	"container/heap"
	"fmt"
	"log"
	"strings"

	sixdegrees "github.com/Jonnymurillo288/SixDegreesSpotify/sixDegrees"
)

// RunSearchOptsBFS performs a breadth-first (popularity-weighted) traversal between artists.
func RunSearchOptsBFS(store *Store, start, target *sixdegrees.Artists, maxDepth int, verbose bool, limit *int, offline bool) (*sixdegrees.Helper, []string, []string, bool) {
	var err error
	var storeConn *Store
	if store == nil {
		storeConn, err = Open("")
		if err != nil {
			return nil, nil, nil, false
		}
		defer storeConn.Close()
	} else {
		storeConn = store
	}

	h := sixdegrees.NewHelper()
	h.ArtistMap[start.Name] = start
	h.DistTo[start.Name] = 0

	queue := &ArtistQueue{}
	heap.Init(queue)
	heap.Push(queue, &ArtistNode{artist: start, depth: 0, popularity: int(start.Popularity)})

	fmt.Printf("Starting BFS Search from %s to %s\n", start.Name, target.Name)

	visited := make(map[string]bool)
	visitedTracks := map[string]bool{"": true}
	found := false

	for queue.Len() > 0 {
		node := heap.Pop(queue).(*ArtistNode)
		current := node.artist

		// Skip if we've fully processed this artist already
		if visited[current.Name] {
			continue
		}

		if verbose {
			log.Printf("[Depth %d | Queue %d] Exploring %s (%d tracks, pop %d)\n",
				h.DistTo[current.Name], queue.Len(), current.Name, len(current.Tracks), current.Popularity)
		}

		// Don't enrich beyond max depth
		if maxDepth >= 0 && h.DistTo[current.Name] >= maxDepth {
			if verbose {
				fmt.Printf("Reached max depth for %s (%d)\n", current.Name, maxDepth)
			}
			visited[current.Name] = true
			continue
		}

		// Enrich artist (DB → Cache → API)
		if err := storeConn.enrichArtist(current, h, target.Name, &found, verbose, limit, offline); err != nil && verbose {
			log.Printf("enrichArtist error for %s: %v", current.Name, err)
		}
		fmt.Println("Current artist has", len(current.Tracks), "tracks after enrichment.")

		// Early exit if enrichment directly discovered target
		if found {
			break
		}
		fmt.Println("Have not found target yet, continuing BFS...")
		for _, tr := range current.Tracks {
			if visitedTracks[tr.ID] {
				continue
			}
			visitedTracks[tr.ID] = true

			if verbose {
				fmt.Printf("Processing track: %s (%s)\n", tr.Name, current.Name)
			}

			for _, feat := range tr.Featured {
				if feat == nil || feat.Name == "" || feat.Name == current.Name {
					continue
				}

				// Only discover once
				if _, seen := h.DistTo[feat.Name]; seen {
					continue
				}

				// Record how we got here: current.Name --[tr.Name]--> feat.Name
				h.Prev[feat.Name] = current.Name
				h.Evidence[feat.Name] = tr.Name
				h.DistTo[feat.Name] = h.DistTo[current.Name] + 1
				h.ArtistMap[feat.Name] = feat

				// Check for direct target connection
				if strings.EqualFold(feat.Name, target.Name) {
					h.Prev[target.Name] = current.Name
					h.Evidence[target.Name] = tr.Name
					found = true
					break
				}

				heap.Push(queue, &ArtistNode{
					artist:     feat,
					depth:      h.DistTo[feat.Name],
					popularity: int(feat.Popularity),
				})
			}

			if found {
				break
			}
		}

		// Mark artist as fully processed *after* enrichment and feature expansion
		visited[current.Name] = true

		if found {
			break
		}
	}

	// Build and return path
	if found {
		path, songs := h.ReconstructPath(start.Name, target.Name)
		return h, path, songs, true
	}

	fmt.Println("No path found between", start.Name, "and", target.Name)
	return h, nil, nil, false
}

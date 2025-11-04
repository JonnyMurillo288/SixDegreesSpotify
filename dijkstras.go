// This is going to be the dijkstras algorithm implementation for finding the shortest path
// between two artists based on their collaborations
package main

import (
	"container/heap"
	"context"
	"fmt"
	"log"

	sixdegrees "github.com/Jonnymurillo288/SixDegreesSpotify/sixDegrees"
)

// RunSearchOpts performs a best-first (popularity-weighted) traversal through the entire queue.
func RunSearchOptsDIJKSTRA(store *Store, start, target *sixdegrees.Artists, maxDepth int, verbose bool, limit *int, offline bool) (*sixdegrees.Helper, []string, []sixdegrees.Track, bool) {
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
	visited := map[string]bool{start.Name: true}
	visitedTracks := map[string]bool{"": true}
	found := false

	for queue.Len() > 0 {
		node := heap.Pop(queue).(*ArtistNode)
		current := node.artist
		dbTrack, err := storeConn.ListTracksByArtistID(context.Background(), current.ID, 1e6)
		if err != nil {
			fmt.Printf("Error with getting tracks for Artist: %s in RunSearchOpts", current.Name)
		}

		tracks, err := storeConn.DBTracksToTracks(dbTrack)
		if err != nil {
			fmt.Println("Error with DBTracksToTracks. Unable to do so")
		}
		// current.Tracks = append(current.Tracks, tracks...)
		for _, t := range tracks {
			if !visitedTracks[t.ID] {
				current.Tracks = append(current.Tracks, t)
			}
		}

		if verbose {
			log.Printf("[Depth %d | Queue %d] Exploring %s (%d tracks, pop %d)",
				h.DistTo[current.Name], queue.Len(), current.Name, len(current.Tracks), current.Popularity)
		}

		if maxDepth >= 0 && h.DistTo[current.Name] >= maxDepth {
			continue
		}
		if err := store.enrichArtist(current, h, target.Name, &found, verbose, limit, offline); err != nil && verbose {
			log.Printf("enrichArtist error for %s: %v", current.Name, err)
		}
		for _, tr := range current.Tracks {
			if visitedTracks[tr.ID] {
				fmt.Println("Have previously visited", tr.Name)
				continue
			}
			visitedTracks[tr.ID] = true
			fmt.Println("bfs.go Line 99: Upserting a new track", tr.Artist.Name, visitedTracks[tr.ID], tr.Name)

			dba := createDBTrack(tr, "")
			if err := storeConn.UpsertTrack(context.Background(), dba); err != nil && verbose {
				log.Printf("warning: upsert track %s: %v", tr.Name, err)
			}
			// Record path evidence
			if tr.Artist != nil {
				h.Evidence[tr.Artist.Name] = tr
			}

			for _, feat := range tr.Featured {
				if feat == nil || feat.Name == "" || feat.Name == current.Name {
					fmt.Println("Skipping featured artist", feat.Name, "for track", tr.Name)
					continue
				}
				if visited[feat.Name] {
					fmt.Println("Already visited featured artist", feat.Name, "for track", tr.Name)
					fmt.Printf("Evidence so far for %s: %s\n", feat.Name, h.Evidence[feat.Name])
					continue
				}
				// only assign DistTo and Prev on first discovery
				if _, seen := h.DistTo[feat.Name]; seen {
					// already have a shorter path recorded
					continue
				}

				visited[feat.Name] = true
				h.Prev[feat.Name] = current.Name
				h.Evidence[feat.Name] = tr
				h.DistTo[feat.Name] = h.DistTo[current.Name] + 1
				h.ArtistMap[feat.Name] = feat

				if hasTarget(feat, target.Name) {
					h.Prev[target.Name] = feat.Name
					found = true
					break
				}

				heap.Push(queue, feat)
			}
			if found {
				break // optional early exit
			}

		}
	}

	// Always return helper; only build path if target found
	if found {
		path, songs := h.ReconstructPath(start.Name, target.Name)
		return h, path, songs, true
	} else {
		fmt.Println("No path found: returning helper only,")
		return h, nil, nil, false
	}
}

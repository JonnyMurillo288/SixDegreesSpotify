package main

import (
	"context"
	"fmt"
	"log"
	"time"

	sixdegrees "github.com/Jonnymurillo288/SixDegreesSpotify/sixDegrees"
)

func RunSearchOptsBFS(
	start, target *sixdegrees.Artists,
	maxDepth int,
	verbose bool,
	limit *int,
	offline bool,
) (*sixdegrees.Helper, []string, []string, []sixdegrees.Track, int, bool) {
	s, err := Open("")
	if err != nil {
		fmt.Println("Error opening up the database:", err)
		return nil, nil, nil, nil, 0, false
	}
	defer s.Close()

	if start == nil || start.ID == "" || target == nil || target.ID == "" {
		return nil, nil, nil, nil, 400, false
	}

	if start.ID == target.ID {
		h := sixdegrees.NewHelper()
		h.ArtistByID[start.ID] = start
		h.IDByName[start.Name] = start.ID
		return h, []string{start.Name}, []string{start.ID}, []sixdegrees.Track{}, 200, true
	}

	if verbose {
		fmt.Println("=== Starting BFS over MusicBrainz relations ===")
		fmt.Printf("Start:  %s (%s)\n", start.Name, start.ID)
		fmt.Printf("Target: %s (%s)\n", target.Name, target.ID)
	}

	// Limits to prevent runaway searches
	perArtistLimit := 5000
	if limit != nil && *limit > 0 {
		perArtistLimit = *limit
	}
	if perArtistLimit > 20000 {
		perArtistLimit = 20000
	}

	const maxExpandedArtists = 30000
	const maxSearchDuration = 3000 * time.Second

	startTime := time.Now()

	// Helper to store mapping
	h := sixdegrees.NewHelper()
	h.ArtistByID[start.ID] = start
	h.IDByName[start.Name] = start.ID

	type queueItem struct {
		A     *sixdegrees.Artists
		Depth int
	}

	queue := []queueItem{{A: start, Depth: 0}}
	visited := map[string]bool{start.ID: true}

	prev := make(map[string]string)                 // childID → parentID
	prevTracks := make(map[string]sixdegrees.Track) // childID → track used

	expandedCount := 0

	for len(queue) > 0 {
		if time.Since(startTime) > maxSearchDuration {
			if verbose {
				log.Printf("BFS aborted: exceeded max search duration (%s)", maxSearchDuration)
			}
			return h, nil, nil, nil, 504, false
		}

		if expandedCount >= maxExpandedArtists {
			if verbose {
				log.Printf("BFS aborted: expanded too many artists (%d)", maxExpandedArtists)
			}
			return h, nil, nil, nil, 504, false
		}

		item := queue[0]
		queue = queue[1:]

		if maxDepth > 0 && item.Depth > maxDepth {
			continue
		}

		if verbose {
			log.Printf("Expanding %s (%s) at depth %d",
				item.A.Name, item.A.ID, item.Depth)
		}

		expandedCount++
		// fmt.Println("Number of Limit", perArtistLimit)
		neighbors, status, err := s.MusicBrainzNeighborProvider(context.Background(), item.A, perArtistLimit, verbose)
		if status == 429 {
			if verbose {
				log.Printf("NeighborProvider rate limited on %s: %v", item.A.Name, err)
			}
			return h, nil, nil, nil, 429, false
		}
		if err != nil {
			if verbose {
				log.Printf("NeighborProvider error on %s: %v", item.A.Name, err)
			}
			continue
		}

		for _, nb := range neighbors {
			if nb == nil || nb.Artist == nil || nb.Artist.ID == "" {
				continue
			}
			childID := nb.Artist.ID

			// keep helper maps populated
			if _, ok := h.ArtistByID[childID]; !ok {
				h.ArtistByID[childID] = nb.Artist
			}
			if nb.Artist.Name != "" {
				h.IDByName[nb.Artist.Name] = childID
			}

			if !visited[childID] {
				visited[childID] = true
				prev[childID] = item.A.ID
				prevTracks[childID] = nb.Track

				// found target?
				if childID == target.ID {
					if verbose {
						log.Printf("Found target %s (%s) at depth %d",
							nb.Artist.Name, childID, item.Depth+1)
					}

					pathIDs := reconstructIDPath(prev, start.ID, target.ID)

					pathNames := make([]string, 0, len(pathIDs))
					for _, id := range pathIDs {
						if art, ok := h.ArtistByID[id]; ok {
							pathNames = append(pathNames, art.Name)
						} else {
							pathNames = append(pathNames, id)
						}
					}

					// reconstruct track path
					pathTracks := make([]sixdegrees.Track, 0, len(pathIDs)-1)
					for i := 1; i < len(pathIDs); i++ {
						pid := pathIDs[i]
						if t, ok := prevTracks[pid]; ok {
							pathTracks = append(pathTracks, t)
						} else {
							pathTracks = append(pathTracks, sixdegrees.Track{})
						}
					}

					return h, pathNames, pathIDs, pathTracks, 200, true
				}

				queue = append(queue, queueItem{
					A:     nb.Artist,
					Depth: item.Depth + 1,
				})
			}
		}
	}
	s.Close()

	if verbose {
		log.Printf("BFS finished with no path found (expanded %d artists)", expandedCount)
	}
	return h, nil, nil, nil, 404, false
}

// reconstructIDPath rebuilds the path from startID to targetID using prev map.
func reconstructIDPath(prev map[string]string, startID, targetID string) []string {
	var path []string
	for at := targetID; at != ""; at = prev[at] {
		path = append(path, at)
		if at == startID {
			break
		}
	}
	// reverse
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	if len(path) == 0 || path[0] != startID {
		return nil
	}
	return path
}

package main

import (
	"container/heap"
	"fmt"
	"log"

	sixdegrees "github.com/Jonnymurillo288/SixDegreesSpotify/sixDegrees"
)

// RunSearchOptsBidirectional performs a bidirectional BFS search between artists.
// It expands outward from both the start and target artists and stops when the two frontiers meet.
func RunSearchOptsBidirectional(store *Store, start, target *sixdegrees.Artists, maxDepth int, verbose bool, limit *int, offline bool) (*sixdegrees.Helper, []string, []string, bool) {
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

	fmt.Printf("Starting Bidirectional BFS Search from %s to %s\n", start.Name, target.Name)

	// Initialize helpers and queues for both directions
	hStart := sixdegrees.NewHelper()
	hTarget := sixdegrees.NewHelper()
	hStart.ArtistMap[start.Name] = start
	hTarget.ArtistMap[target.Name] = target
	hStart.DistTo[start.Name] = 0
	hTarget.DistTo[target.Name] = 0

	visitedStart := map[string]bool{}
	visitedTarget := map[string]bool{}

	qStart := &ArtistQueue{}
	qTarget := &ArtistQueue{}
	heap.Init(qStart)
	heap.Init(qTarget)

	heap.Push(qStart, &ArtistNode{artist: start, depth: 0, popularity: int(start.Popularity)})
	heap.Push(qTarget, &ArtistNode{artist: target, depth: 0, popularity: int(target.Popularity)})

	// Main alternating expansion loop
	for qStart.Len() > 0 && qTarget.Len() > 0 {
		meet := expandFrontier(qStart, visitedStart, hStart, visitedTarget, hTarget, storeConn, target.Name, verbose, maxDepth, limit, offline)
		if meet != "" {
			path, songs := reconstructBidirectionalPath(hStart, hTarget, start.Name, target.Name, meet)
			return hStart, path, songs, true
		}

		meet = expandFrontier(qTarget, visitedTarget, hTarget, visitedStart, hStart, storeConn, start.Name, verbose, maxDepth, limit, offline)
		if meet != "" {
			path, songs := reconstructBidirectionalPath(hStart, hTarget, start.Name, target.Name, meet)
			return hStart, path, songs, true
		}
	}

	fmt.Println("No path found between", start.Name, "and", target.Name)
	return hStart, nil, nil, false
}

// expandFrontier expands one BFS frontier by a single node and checks for intersection.
func expandFrontier(
	queue *ArtistQueue,
	visited map[string]bool,
	hThis *sixdegrees.Helper,
	visitedOther map[string]bool,
	hOther *sixdegrees.Helper,
	storeConn *Store,
	goalName string,
	verbose bool,
	maxDepth int,
	limit *int,
	offline bool,
) string {

	if queue.Len() == 0 {
		return ""
	}

	node := heap.Pop(queue).(*ArtistNode)
	current := node.artist

	if visited[current.Name] {
		return ""
	}

	depth := hThis.DistTo[current.Name]
	if maxDepth >= 0 && depth >= maxDepth {
		if verbose {
			log.Printf("Reached max depth for %s (%d)\n", current.Name, maxDepth)
		}
		visited[current.Name] = true
		return ""
	}

	if verbose {
		log.Printf("[Depth %d | Queue %d] Expanding %s (pop %d)\n", depth, queue.Len(), current.Name, current.Popularity)
	}

	if err := storeConn.enrichArtist(current, hThis, goalName, new(bool), verbose, limit, offline); err != nil && verbose {
		log.Printf("enrichArtist error for %s: %v", current.Name, err)
	}

	visited[current.Name] = true

	for _, tr := range current.Tracks {
		for _, feat := range tr.Featured {
			if feat == nil || feat.Name == "" || feat.Name == current.Name {
				continue
			}
			if visited[feat.Name] {
				continue
			}
			if _, seen := hThis.DistTo[feat.Name]; seen {
				continue
			}

			hThis.Prev[feat.Name] = current.Name
			hThis.Evidence[feat.Name] = tr.Name
			hThis.ArtistMap[feat.Name] = feat
			hThis.DistTo[feat.Name] = depth + 1

			// Check for intersection with the other frontier
			if visitedOther[feat.Name] || hOther.ArtistMap[feat.Name] != nil {
				if verbose {
					fmt.Printf("Frontiers met at %s!\n", feat.Name)
				}
				return feat.Name
			}

			heap.Push(queue, &ArtistNode{
				artist:     feat,
				depth:      hThis.DistTo[feat.Name],
				popularity: int(feat.Popularity),
			})
		}
	}

	return ""
}

// reconstructBidirectionalPath merges the two BFS trees at the meeting artist.
func reconstructBidirectionalPath(hStart, hTarget *sixdegrees.Helper, startName, targetName, meet string) ([]string, []string) {
	leftPath, leftSongs := hStart.ReconstructPath(startName, meet)
	rightPath, rightSongs := hTarget.ReconstructPath(targetName, meet)

	// Reverse right side (meet → target)
	reverse(rightPath)
	reverse(rightSongs)

	// Merge, omitting duplicate meeting node
	fullPath := append(leftPath, rightPath[1:]...)
	fullSongs := append(leftSongs, rightSongs...)

	fmt.Printf("Meeting artist: %s\n", meet)
	fmt.Printf("Path length: %d\n", len(fullPath))

	return fullPath, fullSongs
}

// reverse reverses a slice of strings in place.
func reverse(s []string) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

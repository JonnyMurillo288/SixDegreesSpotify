package sixdegrees

import (
	"fmt"
	"log"
	"time"

	"github.com/Jonnymurillo288/SixDegreesSpotify/spotify"
)

// Helper tracks visited artists, distances, predecessor chain, and edge evidence.
type Helper struct {
	ArtistMap map[string]*Artists // visited artists by name
	DistTo    map[string]int      // distance (hops)
	Prev      map[string]string   // predecessor chain
	Evidence  map[string]string   // track name connecting Prev[x] -> x
}

// NewHelper initializes an empty BFS helper
func NewHelper() *Helper {
	return &Helper{
		ArtistMap: make(map[string]*Artists),
		DistTo:    make(map[string]int),
		Prev:      make(map[string]string),
		Evidence:  make(map[string]string),
	}
}

// RunSearchOpts performs a bounded/unbounded BFS search between artists.
func RunSearchOpts(start, target *Artists, maxDepth int, verbose bool) (*Helper, []string, bool) {
	h := NewHelper()
	h.ArtistMap[start.Name] = start
	h.DistTo[start.Name] = 0

	queue := []*Artists{start}
	visited := map[string]bool{start.Name: true}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if verbose {
			log.Printf("[Depth %d] Exploring %s (%d tracks)", h.DistTo[current.Name], current.Name, len(current.Tracks))
		}

		// Depth guard
		if maxDepth >= 0 && h.DistTo[current.Name] >= maxDepth {
			continue
		}

		// Expand each track
		for _, tr := range current.Tracks {
			// Direct artist match
			if tr.Artist.Name == target.Name {
				h.Prev[target.Name] = current.Name
				h.Evidence[target.Name] = tr.Name
				return h, h.ReconstructPath(start.Name, target.Name), true
			}

			for _, feat := range tr.Featured {
				if feat.Name == "" || feat.Name == current.Name {
					continue
				}

				// Already visited → skip
				if visited[feat.Name] {
					continue
				}
				visited[feat.Name] = true

				// Record connection
				h.Prev[feat.Name] = current.Name
				h.Evidence[feat.Name] = tr.Name
				h.DistTo[feat.Name] = h.DistTo[current.Name] + 1
				h.ArtistMap[feat.Name] = feat

				if verbose {
					log.Printf("  ↳ Found feature: %s (via %s)", feat.Name, tr.Name)
				}

				// Fetch this feature’s albums/tracks only once
				if err := enrichArtist(feat, h, verbose); err != nil && verbose {
					log.Printf("    (warning: %v)", err)
				}

				// Check if target found among features’ tracks
				if hasTarget(feat, target.Name) {
					h.Prev[target.Name] = feat.Name
					return h, h.ReconstructPath(start.Name, target.Name), true
				}

				queue = append(queue, feat)
			}
		}
	}
	return h, nil, false
}

// Enrich artist data by fetching albums and tracks if not already populated.
func enrichArtist(a *Artists, h *Helper, verbose bool) error {
	if len(a.Tracks) > 0 {
		return nil // already enriched
	}
	if verbose {
		log.Printf("    Fetching albums/tracks for %s...", a.Name)
	}
	body, err := spotify.ArtistAlbums(a.ID, 10)
	if err != nil {
		return fmt.Errorf("albums fetch failed for %s: %w", a.Name, err)
	}
	for _, al := range a.ParseAlbums(body) {
		tracks, err := spotify.GetAlbumTracks(al)
		if err != nil {
			continue
		}
		T, _ := a.CreateTracks(tracks, h)
		a.Tracks = append(a.Tracks, T...)
	}
	time.Sleep(300 * time.Millisecond) // small delay to respect API rate limits
	return nil
}

// Utility to check if any track by this artist matches the target
func hasTarget(a *Artists, target string) bool {
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

// ReconstructPath builds a start→target list using Prev map.
func (h *Helper) ReconstructPath(start, target string) []string {
	if start == "" || target == "" {
		return nil
	}
	cur := target
	var path []string
	for cur != "" {
		path = append(path, cur)
		if cur == start {
			break
		}
		cur = h.Prev[cur]
		if cur == "" {
			return nil
		}
	}
	// reverse
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

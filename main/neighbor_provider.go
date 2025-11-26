package main

import (
	"fmt"

	sixdegrees "github.com/Jonnymurillo288/SixDegreesSpotify/sixDegrees"
)

// NeighborEdge represents a single "collaboration edge":
// the neighbor artist + (optionally) the track connecting them.
//
// In the pure MusicBrainz version, Track is left empty for now because
// we are only using artist relations, not per-recording credits.
type NeighborEdge struct {
	Artist *sixdegrees.Artists
	Track  sixdegrees.Track
}

// MusicBrainzNeighborProvider returns neighbors (related artists) for artist `a`
// using MusicBrainz relations only.
func MusicBrainzNeighborProvider(
	mb *MBClient,
	a *sixdegrees.Artists,
	limit int,
	verbose bool,
) ([]*NeighborEdge, int, error) {

	if a == nil || a.ID == "" {
		return nil, 400, fmt.Errorf("MusicBrainzNeighborProvider: artist is nil or has empty ID")
	}

	if limit <= 0 {
		limit = 50
	}

	if verbose {
		fmt.Printf("\n=== MusicBrainz: expanding neighbors for %s (%s) ===\n", a.Name, a.ID)
	}

	lookup, err := mb.LookupArtist(a.ID)
	if err != nil {
		return nil, 500, fmt.Errorf("MusicBrainz lookup failed for %s: %w", a.ID, err)
	}

	edges := make([]*NeighborEdge, 0, limit)
	seen := make(map[string]bool)

	for _, rel := range lookup.Relations {
		id := rel.Artist.ID
		name := rel.Artist.Name
		if id == "" || name == "" {
			continue
		}
		if seen[id] {
			continue
		}
		seen[id] = true

		nbArtist := &sixdegrees.Artists{
			ID:   id,
			Name: name,
		}

		edges = append(edges, &NeighborEdge{
			Artist: nbArtist,
			Track:  sixdegrees.Track{}, // left empty in MB-only mode
		})

		if len(edges) >= limit {
			break
		}
	}

	return edges, 200, nil
}

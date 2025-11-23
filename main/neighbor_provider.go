package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	sixdegrees "github.com/Jonnymurillo288/SixDegreesSpotify/sixDegrees"
)

type NeighborEdge struct {
	Artist *sixdegrees.Artists
	Track  sixdegrees.Track
}

// NeighborProvider returns neighbors (featured collaborators) and the tracks
// that connect them to artist `a`. No album_artists table is needed.
func (s *Store) NeighborProvider(
	a *sixdegrees.Artists,
	limit int,
	verbose bool,
) ([]*NeighborEdge, int, error) {

	ctx := context.Background()

	if verbose {
		fmt.Printf("\n=== Expanding neighbors for %s (%s) ===\n", a.Name, a.ID)
	}

	if limit <= 0 {
		limit = 15
	}

	edges := make([]*NeighborEdge, 0, limit)
	seen := make(map[string]bool)

	// ------------------------------------------------------
	// 1) FAST PATH: tracks from DB by artist
	// ------------------------------------------------------
	dbTracks, err := s.ListTracksByArtistID(ctx, a.ID, 5000)
	if err == nil && len(dbTracks) > 0 {
		tracks, _ := s.DBTracksToTracks(dbTracks)
		if verbose {
			log.Printf("    Loaded %d tracks from DB for %s", len(tracks), a.Name)
		}

		for _, t := range tracks {
			if !trackHasArtist(t, a) {
				continue
			}
			for _, f := range t.Featured {
				if f == nil || f.ID == "" {
					continue
				}
				if !seen[f.ID] {
					seen[f.ID] = true
					edges = append(edges, &NeighborEdge{
						Artist: f,
						Track:  t,
					})
					if len(edges) >= limit {
						if verbose {
							log.Printf("    Early stop: enough neighbors from DB tracks")
						}
						return dedupeEdges(edges), 200, nil
					}
				}
			}
		}
	}

	// ------------------------------------------------------
	// 2) Albums (DB-first + API remainder, cached)
	// ------------------------------------------------------
	albumLimit := limit
	if albumLimit < 5 {
		albumLimit = 5
	}

	albumsBody, status, err :=
		s.getArtistAlbumsMergedCached(a.ID, albumLimit, false, verbose)
	if status == 429 {
		return nil, 429, fmt.Errorf("Spotify API rate limit exceeded")
	}
	if err != nil {
		return nil, 500, fmt.Errorf("NeighborProvider: load albums: %w", err)
	}

	if verbose {
		log.Printf("    %s: %d merged albums (limit=%d)", a.Name, len(albumsBody.Items), albumLimit)
	}

	// ------------------------------------------------------
	// 3) For each album → tracks → collaborators (EARLY STOP)
	// ------------------------------------------------------
	for _, al := range albumsBody.Items {
		if len(edges) >= limit {
			if verbose {
				log.Printf("    Early stop: neighbor limit reached from albums")
			}
			break
		}

		// Ensure primary artist exists
		_ = s.UpsertArtist(ctx, DBArtist{ID: a.ID, Name: a.Name})

		// Upsert album (no album_artists table involved)
		_ = s.UpsertAlbum(ctx, DBAlbum{
			ID:              al.ID,
			PrimaryArtistID: sqlNullString(a.ID),
		})

		// Upsert album artists (for completeness; no album_artists link)
		for _, art := range al.Artists {
			if art.ID == "" || art.Name == "" {
				continue
			}
			_ = s.UpsertArtist(ctx, DBArtist{ID: art.ID, Name: art.Name})
		}

		// Get tracks for album (DB + API)
		tracksBody, err := s.fetchAlbumTracksCached(a, al.ID, limit, false, verbose)
		if err != nil {
			if verbose {
				log.Printf("    fetchAlbumTracksCached error for album %s: %v", al.ID, err)
			}
			continue
		}

		j, _ := json.Marshal(tracksBody)
		T, _ := a.CreateTracks(j, nil)
		if len(T) == 0 {
			continue
		}

		for _, t := range T {
			if !trackHasArtist(t, a) {
				continue
			}

			// Upsert track
			_ = s.UpsertTrack(ctx, createDBTrack(t, al.ID))

			// Primary artist
			if t.Artist != nil && t.Artist.ID != "" {
				_ = s.UpsertArtist(ctx, createDBArtist(*t.Artist))
				_ = s.AddTrackArtist(ctx, t.ID, t.Artist.ID, "primary")
			}

			// Featured artists → neighbors
			for _, f := range t.Featured {
				if f == nil || f.ID == "" {
					continue
				}

				if !seen[f.ID] {
					seen[f.ID] = true

					edges = append(edges, &NeighborEdge{
						Artist: f,
						Track:  t,
					})

					if len(edges) >= limit {
						if verbose {
							log.Printf("    Early stop: enough neighbors found (%d)", len(edges))
						}

						_ = s.UpsertArtist(ctx, createDBArtist(*f))
						_ = s.AddTrackArtist(ctx, t.ID, f.ID, "featured")

						return dedupeEdges(edges), 200, nil
					}
				}

				_ = s.UpsertArtist(ctx, createDBArtist(*f))
				_ = s.AddTrackArtist(ctx, t.ID, f.ID, "featured")
			}
		}
	}

	time.Sleep(10 * time.Millisecond)
	return dedupeEdges(edges), 200, nil
}

// trackHasArtist: does track `t` contain artist `a` (primary or featured)?
// func trackHasArtist(t sixdegrees.Track, a *sixdegrees.Artists) bool {
// 	if a == nil {
// 		return false
// 	}
// 	if t.Artist != nil && t.Artist.ID == a.ID {
// 		return true
// 	}
// 	for _, f := range t.Featured {
// 		if f != nil && f.ID == a.ID {
// 			return true
// 		}
// 	}
// 	return false
// }

// dedupeEdges: remove duplicate neighbors by Artist.ID
func dedupeEdges(in []*NeighborEdge) []*NeighborEdge {
	out := make([]*NeighborEdge, 0, len(in))
	seen := make(map[string]bool)
	for _, e := range in {
		if e == nil || e.Artist == nil || e.Artist.ID == "" {
			continue
		}
		if !seen[e.Artist.ID] {
			seen[e.Artist.ID] = true
			out = append(out, e)
		}
	}
	return out
}

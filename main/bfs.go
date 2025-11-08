package main

import (
	"container/heap"
	"fmt"
	"log"
	"strings"

	sixdegrees "github.com/Jonnymurillo288/SixDegreesSpotify/sixDegrees"
)

func keyA(a *sixdegrees.Artists) (id, name string) {
	if a == nil {
		return "", ""
	}
	return a.ID, a.Name
}

// RunSearchOptsBFS performs a breadth-first (popularity-weighted) traversal between artists.
func RunSearchOptsBFS(start, target *sixdegrees.Artists, maxDepth int, verbose bool, limit *int, offline bool,
	enrichFn func(*sixdegrees.Artists, *sixdegrees.Helper, string, *bool, bool, *int, bool) error,
) (*sixdegrees.Helper, []string, []sixdegrees.EdgeSnap, bool) {
	// var storeConn *Store
	// if store == nil {
	// 	storeConn, err = Open("")
	// 	if err != nil {
	// 		return nil, nil, nil, false
	// 	}
	// 	defer storeConn.Close()
	// } else {
	// 	storeConn = store
	// }

	h := sixdegrees.NewHelper()
	h.ArtistByID[start.ID] = start
	h.IDByName[start.Name] = start.ID
	h.DistToID[start.ID] = 0
	var found bool
	found = false

	queue := &ArtistQueue{}
	heap.Init(queue)
	heap.Push(queue, &ArtistNode{artist: start, depth: 0, popularity: int(start.Popularity)})

	visitedArtist := make(map[string]bool) // by artist ID
	visitedTracks := map[string]bool{"": true}

	fmt.Println("Starting BFS Search from", start.Name, "to", target.Name)
	for queue.Len() > 0 {
		node := heap.Pop(queue).(*ArtistNode)
		current := node.artist
		curID, curName := keyA(current)

		if visitedArtist[curID] {
			fmt.Println("Already visited artist", curName, "skipping...")
			continue
		}

		// ... depth checks, enrichArtist (unchanged) ...
		if verbose {
			log.Printf("[Depth %d | Queue %d] Exploring %s (%d tracks, pop %d)\n",
				h.DistToID[current.ID], queue.Len(), current.ID, len(current.Tracks), int(current.Popularity))
		}

		// Don't enrich beyond max depth
		if maxDepth >= 0 && h.DistToID[current.ID] >= maxDepth {
			if verbose {
				fmt.Printf("Reached max depth for %s (%d)\n", current.Name, maxDepth)
			}
			visitedArtist[current.ID] = true
			continue
		}

		// Enrich artist (DB → Cache → API)
		// if err := storeConn.enrichArtist(current, h, target.Name, &found, verbose, limit, offline); err != nil && verbose {
		// 	log.Printf("enrichArtist error for %s: %v", current.Name, err)
		// }
		if err := enrichFn(current, h, target.Name, &found, verbose, limit, offline); err != nil && verbose {
			log.Printf("enrichArtist error for %s: %v", current.Name, err)
		}
		fmt.Println("Current artist has", len(current.Tracks), "tracks after enrichment.")

		// Early exit if enrichment directly discovered target
		if found {
			break
		}

		for _, tr := range current.Tracks {
			if verbose {
				log.Printf("[Depth %d | Queue %d] Exploring %s (%d tracks, pop %d)\n",
					h.DistToID[current.ID], queue.Len(), current.ID, len(current.Tracks), int(current.Popularity))
			}

			if visitedTracks[tr.ID] {
				continue
			}
			visitedTracks[tr.ID] = true

			for _, feat := range tr.Featured {
				if feat == nil {
					continue
				}
				toID, toName := keyA(feat)
				if toID == "" || toID == curID {
					continue
				}

				// verify: feat really is in this track (defensive)
				valid := false
				for _, f := range tr.Featured {
					if f != nil && f.ID == toID {
						valid = true
						break
					}
				}
				if !valid {
					continue
				}

				// Only discover once by **ID**
				if _, seen := h.DistToID[toID]; seen {
					continue
				}

				// Record immutable snapshot (DO NOT store the Track with pointers)
				h.PrevID[toID] = curID
				h.DistToID[toID] = h.DistToID[curID] + 1
				h.ArtistByID[toID] = feat
				h.IDByName[toName] = toID

				fmt.Printf("[VERIFY] %s --[%s/%s]--> %s\n",
					curName, tr.Name, tr.ID, toName)

				h.Evidence[toID] = sixdegrees.EdgeSnap{
					FromID: curID, FromName: curName,
					ToID: toID, ToName: toName,
					TrackID: tr.ID, TrackName: tr.Name,
					PhotoURL: tr.PhotoURL,
				}

				if strings.EqualFold(toID, target.ID) || strings.EqualFold(toName, target.Name) {
					// also snapshot the target explicitly
					h.PrevID[target.ID] = curID
					h.Evidence[target.ID] = sixdegrees.EdgeSnap{
						FromID: curID, FromName: curName,
						ToID: target.ID, ToName: target.Name,
						TrackID: tr.ID, TrackName: tr.Name,
					}

					found = true
					break
				}

				heap.Push(queue, &ArtistNode{
					artist:     feat,
					depth:      h.DistToID[toID],
					popularity: int(feat.Popularity),
				})
			}
			if found {
				break
			}
		}

		visitedArtist[curID] = true
		if found {
			break
		}
	}
	// Build and return path
	if found {
		fmt.Println("Path found between", start.Name, "and", target.Name)
		path, songs := h.ReconstructPathIDs(start.ID, target.ID)
		return h, path, songs, true
	}

	fmt.Println("No path found between", start.Name, "and", target.Name)
	return h, nil, nil, false
}

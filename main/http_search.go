package main

import (
	"fmt"
	"strconv"
	"time"

	sixdegrees "github.com/Jonnymurillo288/SixDegreesSpotify/sixDegrees"
)

// SearchArtists runs the full pipeline to compute a collaboration path between two artists,
// using MusicBrainz for both artist resolution and neighbor expansion.
func SearchArtists(
	start, target string,
	depth, limit int,
	offline bool,
) (int, []struct{ From, Track, To, TrackID, TrackURL string }, string, int, error) {

	if start == "" || target == "" {
		return 0, nil, "start or target empty", 400, nil
	}

	mb := NewMBClient()
	verbose := true
	startTime := time.Now().UTC().Unix()

	// -------------------------
	// Resolve START artist
	// -------------------------
	startHits, err := mb.SearchArtist(start)
	if err != nil || len(startHits) == 0 {
		return 0, nil, "start artist not found", 404, nil
	}
	startArtist := &sixdegrees.Artists{
		ID:   startHits[0].ID,
		Name: startHits[0].Name,
	}

	// -------------------------
	// Resolve TARGET artist
	// -------------------------
	targetHits, err := mb.SearchArtist(target)
	if err != nil || len(targetHits) == 0 {
		return 0, nil, "target artist not found", 404, nil
	}
	targetArtist := &sixdegrees.Artists{
		ID:   targetHits[0].ID,
		Name: targetHits[0].Name,
	}

	fmt.Printf("Running MusicBrainz BFS Search from %q to %q...\n",
		startArtist.Name, targetArtist.Name)

	helper, pathNames, pathIDs, tracks, status, ok := RunSearchOptsBFS(
		startArtist,
		targetArtist,
		depth,
		verbose,
		&limit,
		offline,
	)

	if status == 429 {
		return 0, nil, "", 429, fmt.Errorf("error 429: reached external rate limit")
	}
	if !ok || len(pathIDs) == 0 {
		msg := fmt.Sprintf("no path found between %q and %q", startArtist.Name, targetArtist.Name)
		if depth >= 0 {
			msg = fmt.Sprintf("%s within depth %d", msg, depth)
		}
		fmt.Println(msg)
		return 0, nil, msg, 404, nil
	}

	fmt.Printf("Path found between %q and %q (%d hops):\n\n",
		startArtist.Name, targetArtist.Name, len(pathIDs)-1)
	fmt.Println(pathNames, len(pathNames), pathIDs)

	// ---- Build steps (From, Track, To, TrackID, TrackURL) ----
	var steps []struct{ From, Track, To, TrackID, TrackURL string }
	edges := make([]sixdegrees.EdgeSnap, 0, len(pathIDs)-1)

	// safety: tracks should be len(pathIDs)-1, but guard anyway
	if len(tracks) < len(pathIDs)-1 {
		fmt.Printf("WARNING: tracks length (%d) < hops (%d)\n", len(tracks), len(pathIDs)-1)
	}

	for i := 1; i < len(pathIDs); i++ {
		fromID := pathIDs[i-1]
		toID := pathIDs[i]

		from := helper.ArtistByID[fromID]
		to := helper.ArtistByID[toID]

		var t sixdegrees.Track
		if i-1 < len(tracks) {
			t = tracks[i-1]
		}

		trackID := t.ID
		trackName := t.Name
		trackURL := t.PhotoURL // may be empty in MB-only mode

		steps = append(steps, struct {
			From     string
			Track    string
			To       string
			TrackID  string
			TrackURL string
		}{
			From:     from.Name,
			Track:    trackName,
			To:       to.Name,
			TrackID:  trackID,
			TrackURL: trackURL,
		})

		edges = append(edges, sixdegrees.EdgeSnap{
			FromID:    fromID,
			FromName:  from.Name,
			ToID:      toID,
			ToName:    to.Name,
			TrackID:   trackID,
			TrackName: trackName,
			PhotoURL:  trackURL,
		})

		fmt.Printf("%d. %s —[%s]→ %s (TrackID: %s)\n",
			i, from.Name, trackName, to.Name, trackID)
	}

	helper.PrintPath(pathIDs, edges)

	endTime := time.Now().UTC().Unix()
	fmt.Printf("Analysis took %s seconds\n", strconv.FormatInt(endTime-startTime, 10))
	fmt.Println("Done.")

	return len(pathIDs) - 1, steps, "", 200, nil
}

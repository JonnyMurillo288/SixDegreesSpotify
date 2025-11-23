package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	sixdegrees "github.com/Jonnymurillo288/SixDegreesSpotify/sixDegrees"
)

// SearchArtists runs the full pipeline to compute a collaboration path between two artists.
func SearchArtists(
	start, target string,
	depth, limit int,
	offline bool,
) (int, []struct{ From, Track, To, TrackID, TrackURL string }, string, int, error) {

	store, err := Open("")
	if err != nil {
		return 1, nil, "", 500, fmt.Errorf("failed to open store: %w", err)
	}
	defer store.Close()

	if err := store.Migrate(context.Background()); err != nil {
		return 1, nil, "", 500, fmt.Errorf("failed to migrate store: %w", err)
	}

	if start == "" || target == "" {
		return 0, nil, "start or target empty", 400, nil
	}

	verbose := true
	startTime := time.Now().UTC().Unix()

	var (
		startArtist  *sixdegrees.Artists
		targetArtist *sixdegrees.Artists
		gotArtist    bool
	)

	// ---- Resolve START artist (DB first, then Spotify) ----
	gotArtist = false
	if !offline {
		if dba, err := store.FindArtistByName(context.Background(), start); err == nil {
			gotArtist = true
			fmt.Printf("startArtistDBA (DB): %s\n", dba.Name)
			startArtist = sixdegrees.CreateArtists(dba.Name, dba.ID)
		}
	}
	if !gotArtist {
		fmt.Printf("Fetching start artist %q from Spotify API\n", start)
		startArtist = sixdegrees.InputArtist(start)
		if startArtist == nil || startArtist.ID == "" {
			return 0, nil, "start artist not found", 404, nil
		}
		dba := DBArtist{
			ID:         startArtist.ID,
			Name:       startArtist.Name,
			Popularity: sql.NullInt64{Int64: int64(startArtist.Popularity), Valid: startArtist.Popularity > 0},
			Genres:     startArtist.Genres,
		}
		if err := store.UpsertArtist(context.Background(), dba); err != nil {
			return 1, nil, "", 500, fmt.Errorf("upsert start artist failed: %w", err)
		}
	}

	// ---- Resolve TARGET artist (DB first, then Spotify) ----
	gotArtist = false
	if !offline {
		if dba, err := store.FindArtistByName(context.Background(), target); err == nil {
			gotArtist = true
			fmt.Printf("targetArtistDBA (DB): %s\n", dba.Name)
			targetArtist = sixdegrees.CreateArtists(dba.Name, dba.ID)
		}
	}
	if !gotArtist {
		fmt.Printf("Fetching target artist %q from Spotify API\n", target)
		targetArtist = sixdegrees.InputArtist(target)
		if targetArtist == nil || targetArtist.ID == "" {
			return 0, nil, "target artist not found", 404, nil
		}
		dba := DBArtist{
			ID:         targetArtist.ID,
			Name:       targetArtist.Name,
			Popularity: sql.NullInt64{Int64: int64(targetArtist.Popularity), Valid: targetArtist.Popularity > 0},
			Genres:     targetArtist.Genres,
		}
		if err := store.UpsertArtist(context.Background(), dba); err != nil {
			return 1, nil, "", 500, fmt.Errorf("upsert target artist failed: %w", err)
		}
	}

	// Swap if needed (start is more popular than target)
	switchingArtist := false
	if startArtist.Popularity > targetArtist.Popularity {
		switchingArtist = true
		startArtist, targetArtist = targetArtist, startArtist
	}

	fmt.Println("Running BFS Search...")

	helper, pathNames, pathIDs, tracks, status, ok := RunSearchOptsBFS(
		store,
		startArtist,
		targetArtist,
		depth,
		verbose,
		&limit,
		offline,
	)

	if status == 429 {
		return 0, nil, "", 429, fmt.Errorf("error 429: reached spotify rate limit")
	}
	if !ok || len(pathIDs) == 0 {
		msg := fmt.Sprintf("no path found between %q and %q", startArtist.Name, targetArtist.Name)
		if depth >= 0 {
			msg = fmt.Sprintf("%s within depth %d", msg, depth)
		}
		fmt.Println(msg)
		return 0, nil, msg, 404, nil
	}

	// If we swapped start/target, reverse everything.
	if switchingArtist {
		// reverse names
		for i, j := 0, len(pathNames)-1; i < j; i, j = i+1, j-1 {
			pathNames[i], pathNames[j] = pathNames[j], pathNames[i]
		}
		// reverse IDs
		for i, j := 0, len(pathIDs)-1; i < j; i, j = i+1, j-1 {
			pathIDs[i], pathIDs[j] = pathIDs[j], pathIDs[i]
		}
		// reverse tracks
		for i, j := 0, len(tracks)-1; i < j; i, j = i+1, j-1 {
			tracks[i], tracks[j] = tracks[j], tracks[i]
		}
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
		trackURL := t.PhotoURL // adjust if your Track struct uses another field

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

	// optional: write track IDs to file
	var results []string
	for _, t := range tracks {
		if t.ID != "" {
			results = append(results, t.ID)
		}
	}
	if len(results) > 0 {
		if b, err := json.Marshal(results); err == nil {
			_ = os.WriteFile("results/results.json", b, 0o644)
		}
	}

	endTime := time.Now().UTC().Unix()
	fmt.Printf("Analysis took %s seconds\n", strconv.FormatInt(endTime-startTime, 10))
	fmt.Println("Done.")

	return len(pathIDs) - 1, steps, "", 200, nil
}

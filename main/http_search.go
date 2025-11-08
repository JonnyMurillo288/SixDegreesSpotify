package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	sixdegrees "github.com/Jonnymurillo288/SixDegreesSpotify/sixDegrees"
)

// SearchArtists runs the full pipeline to compute a collaboration path between two artists.
// It mirrors the CLI logic but returns structured data for HTTP.
func SearchArtists(start, target string, depth, limit int, offline bool) (int, []struct{ From, Track, To, TrackID, TrackURL string }, string, error) {
	var err error
	var switchingArtist bool
	var find = target       // to match CLI variable naming
	switchingArtist = false // reset switchingArtist for each call
	var verbose = true      // enable verbose logging for HTTP requests
	startTime := time.Now().UTC().Unix()

	if start == "" || target == "" {
		return 0, nil, "", nil
	}

	var (
		startArtist  *sixdegrees.Artists
		targetArtist *sixdegrees.Artists
		gotArtist    bool
	)
	gotArtist = false
	// Start artist
	// If we are offline mode get the Artist from the DB
	// If we are not then get from the Spotify API
	// if offline {
	// 	gotArtist = true
	// 	startArtistDBA, err := store.FindArtistByName(context.Background(), start)
	// 	if err != nil {
	// 		gotArtist = false
	// 		fmt.Printf("Error with finding artist: %s in the database", start)
	// 	}
	// 	fmt.Printf("startArtistDBA: %s\n", startArtistDBA.Name)
	// 	startArtist = sixdegrees.CreateArtists(startArtistDBA.Name, startArtistDBA.ID)
	// }

	// If we are online mode or we did not get the artist, run API
	if !offline || !gotArtist {
		fmt.Printf("Going to get the %s from Spotify API\n", start)
		startArtist = sixdegrees.InputArtist(start)
		// dba := DBArtist{
		// 	ID:         startArtist.ID,
		// 	Name:       startArtist.Name,
		// 	Popularity: sql.NullInt64{Int64: int64(startArtist.Popularity), Valid: startArtist.Popularity > 0},
		// 	Genres:     startArtist.Genres,
		// }

		// if err := store.UpsertArtist(context.Background(), dba); err != nil {
		// 	log.Fatalf("Upsert failed: %v", err)
		// }
	}

	if startArtist.ID == "" {
		log.Fatalf("Start artist %q not found on Spotify.", start)
	}

	// if offline {
	// 	gotArtist = true
	// 	targetArtistDBA, err := store.FindArtistByName(context.Background(), find)
	// 	if err != nil {
	// 		gotArtist = false
	// 		fmt.Printf("Error with finding artist: %s in the database", find)
	// 	}
	// 	fmt.Printf("startArtistDBA: %s\n", targetArtistDBA.Name)
	// 	targetArtist = sixdegrees.CreateArtists(targetArtistDBA.Name, targetArtistDBA.ID)
	// }

	// If we are online mode or we did not get the artist, run API
	if !offline || !gotArtist {
		fmt.Printf("Going to get the %s from Spotify API\n", target)
		targetArtist = sixdegrees.InputArtist(find)
		// dba := DBArtist{
		// 	ID:         targetArtist.ID,
		// 	Name:       targetArtist.Name,
		// 	Popularity: sql.NullInt64{Int64: int64(targetArtist.Popularity), Valid: startArtist.Popularity > 0},
		// 	Genres:     targetArtist.Genres,
		// }

		// if err := store.UpsertArtist(context.Background(), dba); err != nil {
		// 	log.Fatalf("Upsert failed: %v", err)
		// }
	}

	// Ensure startArtist is the *less popular* one
	if startArtist.Popularity > targetArtist.Popularity {
		fmt.Println("Switching start and target artists for BFS search based on popularity")
		fmt.Println("Start Artist:", startArtist.Name, "with popularity", startArtist.Popularity)
		fmt.Println("Target Artist:", targetArtist.Name, "with popularity", targetArtist.Popularity)
		switchingArtist = true
		startArtist, targetArtist = targetArtist, startArtist
	}

	// Check if there is tracks for the artist in the DB, if not then Call Spotify API
	// dbTracks, err := store.ListTracksByArtistID(context.Background(), startArtist.ID, 1e6)
	// if err != nil {
	// 	fmt.Printf("Error List TracksByArtistID %s", err)
	// }
	// t, err := store.DBTracksToTracks(dbTracks)
	// fmt.Printf("main.go Line 148: Got %d tracks from ListTracksByArtist for %s\n", len(t), startArtist.Name)
	// if no DBTracks or error in getting the DB tracks
	// if (!offline && len(dbTracks) == 0) || err != nil {

	// Run the connection search

	fmt.Println("Running BFS Search...")
	helper, path, songs, ok := RunSearchOptsBFS(startArtist, targetArtist, depth, verbose, &limit, offline, enrichArtist)

	if !ok || len(path) == 0 {
		if depth >= 0 {
			fmt.Printf("No path found between %q and %q within depth %d\n", startArtist.Name, targetArtist.Name, depth)
		} else {
			fmt.Printf("No path found between %q and %q\n", startArtist.Name, targetArtist.Name)
			fmt.Println(len(path))
		}
		os.Exit(0)
	}

	// Display the found path
	fmt.Printf("Path found between %q and %q (%d hops):\n\n", startArtist.Name, targetArtist.Name, len(path)-1)
	if switchingArtist {
		for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
			path[i], path[j] = path[j], path[i]
		}
	}
	fmt.Println(path, len((path)), helper.Evidence[start], songs)
	var results []string // results will hold the track ID that we need
	var steps []struct{ From, Track, To, TrackID, TrackURL string }

	for i := 1; i < len(path); i++ {
		from := path[i-1]
		to := path[i]
		track := songs[i-1]
		fmt.Println(track)
		if track.TrackName != "" {
			fmt.Printf("%d. %s —[%s]→ %s w/ TrackID: %s\n", i, from, track.TrackName, to, "")
			steps = append(steps, struct {
				From     string
				Track    string
				To       string
				TrackID  string
				TrackURL string
			}{From: from, Track: track.TrackName, To: to, TrackID: track.TrackID, TrackURL: track.PhotoURL})
		} else {
			fmt.Printf("%d. %s → %s\n", i, from, to)
		}
	}
	helper.PrintPath(path, songs)

	resultsBytes, err := json.Marshal(results)
	if err != nil {
		log.Fatalf("Error marshalling results to JSON: %v", err)
	}

	writeErr := os.WriteFile("results/results.json", resultsBytes, 0644)
	if writeErr != nil {
		log.Fatalf("Error writing results to file: %v", err)
	}
	fmt.Println("Results written to results/results.json")

	endTime := time.Now().UTC().Unix()
	fmt.Printf("Analysis took %s seconds", strconv.FormatInt(endTime-startTime, 10))
	fmt.Println("\nDone.")
	return len(path) - 1, steps, "", nil
}

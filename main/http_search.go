package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	sixdegrees "github.com/Jonnymurillo288/SixDegreesSpotify/sixDegrees"
	"github.com/Jonnymurillo288/SixDegreesSpotify/spotify"
)

// SearchArtists runs the full pipeline to compute a collaboration path between two artists.
// It mirrors the CLI logic but returns structured data for HTTP.
func SearchArtists(ctx context.Context, store *Store, start, target string, depth, limit int, offline bool) (int, []struct{ From, Track, To string }, string, error) {
	var err error
	var switchingArtist bool
	var find = target       // to match CLI variable naming
	switchingArtist = false // reset switchingArtist for each call
	var verbose = true      // enable verbose logging for HTTP requests
	startTime := time.Now().UTC().Unix()

	if start == "" || target == "" {
		return 0, nil, "start and target required", nil
	}

	var (
		startArtist  *sixdegrees.Artists
		targetArtist *sixdegrees.Artists
		gotArtist    bool
	)

	// Start artist
	// If we are offline mode get the Artist from the DB
	// If we are not then get from the Spotify API
	if offline {
		gotArtist = true
		startArtistDBA, err := store.FindArtistByName(context.Background(), start)
		if err != nil {
			gotArtist = false
			fmt.Printf("Error with finding artist: %s in the database", start)
		}
		fmt.Printf("startArtistDBA: %s\n", startArtistDBA.Name)
		startArtist = sixdegrees.CreateArtists(startArtistDBA.Name, startArtistDBA.ID)
	}

	// If we are online mode or we did not get the artist, run API
	if !offline || !gotArtist {
		fmt.Printf("Going to get the Input Artist from Spotify API")
		startArtist = sixdegrees.InputArtist(start)
		dba := DBArtist{
			ID:         startArtist.ID,
			Name:       startArtist.Name,
			Popularity: sql.NullInt64{Int64: int64(startArtist.Popularity), Valid: startArtist.Popularity > 0},
			Genres:     startArtist.Genres,
		}

		if err := store.UpsertArtist(context.Background(), dba); err != nil {
			log.Fatalf("Upsert failed: %v", err)
		}
	}

	if startArtist.ID == "" {
		log.Fatalf("Start artist %q not found on Spotify.", start)
	}

	if offline {
		gotArtist = true
		targetArtistDBA, err := store.FindArtistByName(context.Background(), find)
		if err != nil {
			gotArtist = false
			fmt.Printf("Error with finding artist: %s in the database", find)
		}
		fmt.Printf("startArtistDBA: %s\n", targetArtistDBA.Name)
		targetArtist = sixdegrees.CreateArtists(targetArtistDBA.Name, targetArtistDBA.ID)
	}

	// If we are online mode or we did not get the artist, run API
	if !offline || !gotArtist {
		fmt.Printf("Going to get the Input Artist from Spotify API")
		targetArtist = sixdegrees.InputArtist(find)
		dba := DBArtist{
			ID:         targetArtist.ID,
			Name:       targetArtist.Name,
			Popularity: sql.NullInt64{Int64: int64(targetArtist.Popularity), Valid: startArtist.Popularity > 0},
			Genres:     targetArtist.Genres,
		}

		if err := store.UpsertArtist(context.Background(), dba); err != nil {
			log.Fatalf("Upsert failed: %v", err)
		}
	}

	// Ensure startArtist is the *less popular* one
	if startArtist.Popularity > targetArtist.Popularity {
		switchingArtist = true
		startArtist, targetArtist = targetArtist, startArtist
	}

	// Retrieve albums for the starting artist
	var albums []byte
	if !offline {
		fmt.Println(offline)
		fmt.Println("Made it passed the offline filter main.go: Line 107")
		albums, err = spotify.ArtistAlbums(startArtist.ID, limit)
		if err != nil {
			log.Fatalf("Error fetching albums for %s: %v", startArtist.Name, err)
		}
	}

	h := sixdegrees.NewHelper()

	// Check if there is tracks for the artist in the DB, if not then Call Spotify API
	// dbTracks, err := store.ListTracksByArtistID(context.Background(), startArtist.ID, 1e6)
	// if err != nil {
	// 	fmt.Printf("Error List TracksByArtistID %s", err)
	// }
	// t, err := store.DBTracksToTracks(dbTracks)
	// fmt.Printf("main.go Line 148: Got %d tracks from ListTracksByArtist for %s\n", len(t), startArtist.Name)
	// if no DBTracks or error in getting the DB tracks
	// if (!offline && len(dbTracks) == 0) || err != nil {
	var t []sixdegrees.Track
	if (!offline) || err != nil {
		fmt.Println("Made it passed the offline filter main.go: Line 124")
		// The first layer of the queue will be the startArtist features
		for _, album := range startArtist.ParseAlbums(albums) {
			dba := DBAlbum{
				ID:              album,
				PrimaryArtistID: sql.NullString{String: startArtist.ID, Valid: startArtist.ID != ""},
			}

			if err := store.UpsertAlbum(context.Background(), dba); err != nil {
				log.Fatalf("Upsert failed: %v", err)
			}
			fmt.Println("Upserted Album")
			tracks, err := spotify.GetAlbumTracks(album)
			t, _ := startArtist.CreateTracks(tracks, h)
			if err != nil {
				log.Printf("Warning: failed to fetch tracks for album %s: %v", album, err)
			}
			startArtist.Tracks = append(startArtist.Tracks, t...)
			for _, tr := range t {
				track := DBTrack{
					ID:              tr.ID,
					Name:            tr.Name,
					AlbumID:         sql.NullString{String: album, Valid: album != ""},
					PrimaryArtistID: sql.NullString{String: tr.Artist.ID, Valid: tr.Artist.ID != ""},
				}
				if err := store.UpsertTrack(context.Background(), track); err != nil {
					log.Fatalf("Upsert failed: %v", err)
				}
			}
		}
	} else {
		startArtist.Tracks = append(startArtist.Tracks, t...)
	}

	if !offline {
		fmt.Println("Made it below the offline filter for spotify.ArtistAlbums Line 158")
		albums, err = spotify.ArtistAlbums(targetArtist.ID, limit)
		if err != nil {
			log.Fatalf("Error fetching albums for %s: %v", startArtist.Name, err)
		}
	}

	// Check if there is tracks for the artist in the DB, if not then Call Spotify API
	// dbTracks, err = store.ListTracksByArtistID(context.Background(), startArtist.ID, 1e6)
	// if err != nil {
	// 	fmt.Printf("Error List TracksByArtistID %s", err)
	// }
	// t, err = store.DBTracksToTracks(dbTracks)
	// fmt.Printf("main.go Line 199: Got %d tracks from ListTracksByArtist for %s\n", len(t), targetArtist.Name)
	// if no DBTracks or error in getting the DB tracks
	// if !offline && len(dbTracks) == 0 || err != nil {
	if !offline || err != nil {
		fmt.Println("Made it below the offline filter")
		// The first layer of the queue will be the startArtist features
		for _, album := range targetArtist.ParseAlbums(albums) {
			dba := DBAlbum{
				ID:              album,
				PrimaryArtistID: sql.NullString{String: targetArtist.ID, Valid: startArtist.ID != ""},
			}

			if err := store.UpsertAlbum(context.Background(), dba); err != nil {
				log.Fatalf("Upsert failed: %v", err)
			}
			fmt.Println("Upserted Album")
			tracks, err := spotify.GetAlbumTracks(album)
			t, _ := targetArtist.CreateTracks(tracks, h)
			if err != nil {
				log.Printf("Warning: failed to fetch tracks for album %s: %v", album, err)
			}
			targetArtist.Tracks = append(targetArtist.Tracks, t...)
			for _, tr := range t {
				track := DBTrack{
					ID:              tr.ID,
					Name:            tr.Name,
					AlbumID:         sql.NullString{String: album, Valid: album != ""},
					PrimaryArtistID: sql.NullString{String: tr.Artist.ID, Valid: tr.Artist.ID != ""},
				}
				if err := store.UpsertTrack(context.Background(), track); err != nil {
					log.Fatalf("Upsert failed: %v", err)
				}
			}
		}
	} else {
		targetArtist.Tracks = append(targetArtist.Tracks, t...)
	}

	// Run the connection search
	helper, path, songs, ok := RunSearchOptsBFS(store, startArtist, targetArtist, depth, verbose, &limit, offline)

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
	fmt.Println(startArtist.Name, targetArtist.Name)
	fmt.Printf("Path found between %q and %q (%d hops):\n\n", startArtist.Name, targetArtist.Name, len(path)-1)
	if switchingArtist {
		for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
			path[i], path[j] = path[j], path[i]
		}
	}
	fmt.Println(path, len((path)), h.Evidence[start], songs)
	var results []string // results will hold the track ID that we need
	var steps []struct{ From, Track, To string }
	var trackSteps []sixdegrees.Track

	for i := 1; i < len(path); i++ {
		from := path[i-1]
		to := path[i]
		track := helper.Evidence[from]
		fmt.Println(track)
		fmt.Println("Previous:", h.Prev)
		if track.Name != "" {
			fmt.Printf("%d. %s —[%s]→ %s\n", i, from, track, to)
			steps = append(steps, struct{ From, Track, To string }{From: from, Track: track.Name, To: to})
			trackSteps = append(trackSteps, track)
		} else {
			fmt.Printf("%d. %s → %s\n", i, from, to)
		}
	}
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

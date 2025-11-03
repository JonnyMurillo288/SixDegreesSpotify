package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	sixdegrees "github.com/Jonnymurillo288/SixDegreesSpotify/sixDegrees"
)

// This is a parallel main function that utilizes the database instead of searching
// the Spotify API. For testing and eventually searching only here
func Smain() {

	var store, err = Open("")
	var mh MainHelper
	if err != nil {
		log.Fatalf("failed to connect: %v", err)
	}
	defer store.Close()

	if err := store.Migrate(context.Background()); err != nil {
		log.Fatalf("migration failed: %v", err)
	}

	log.Println("Database ready!")

	startTime := time.Now().UTC().Unix()
	var start, find string
	var depth int
	var verbose bool
	var limit int
	var switchingArtist bool
	switchingArtist = false

	flag.StringVar(&start, "start", "", "Starting artist name")
	flag.StringVar(&find, "find", "", "Target artist name to find connection to")
	flag.IntVar(&depth, "depth", -1, "Maximum BFS depth in hops (-1 for unlimited)")
	flag.BoolVar(&verbose, "verbose", false, "Enable verbose logging")
	flag.IntVar(&limit, "limit", 5, "Max Limit of albums to parse through")
	flag.Parse()

	if start == "" || find == "" {
		fmt.Println("Missing required flags: -start and/or -find.")
		fmt.Println(`Usage: go run main.go -start "Artist A" -find "Artist B" [-depth N] [-verbose]`)
		os.Exit(1)
	}

	// Ensure Spotify authorization before making any API calls
	if err := ensureSpotifyAuth(); err != nil {
		log.Fatalf("Spotify authorization failed: %v", err)
	}

	// Create/Find Target Artist
	// startArtist := FindArtistByName(context.Background(),start)
	startArtist := sixdegrees.InputArtist(start)
	if startArtist == nil || startArtist.ID == "" {
		log.Fatalf("Start artist %q not found on Spotify.", start)
	}

	// Create and Insert Target Artist to the Database if they do not exist in the current database
	// targetArtist := FindArtistByName(context.Background(),find)
	targetArtist := sixdegrees.InputArtist(find)
	if targetArtist == nil || targetArtist.ID == "" {
		log.Fatalf("Target artist %q not found on Spotify.", find)
	}

	// Ensure startArtist is the *less popular* one
	if startArtist.Popularity > targetArtist.Popularity {
		switchingArtist = true
		startArtist, targetArtist = targetArtist, startArtist
	}

	h := sixdegrees.NewHelper()

	var album = "" // Keep this empty for now
	dbt, err := store.ListTracksByArtistID(context.Background(), startArtist.ID, 1e6)
	if err != nil {
		log.Printf("Warning: failed to fetch tracks for album %s: %v", startArtist.Name, err)
	}

	tracks, err := json.Marshal(dbt)
	if err != nil {
		log.Printf("warning: failed to fetch tracks for album %s: %v", startArtist.Name, err)
	}
	os.WriteFile("tracks_view.json", tracks, 0644)
	t, _ := startArtist.CreateTracks(tracks, h)

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
		for _, feat := range tr.Featured {
			mh = createMainHelper(*startArtist, tr, *feat)
			store.UpsertGraph(context.Background(), mh)

		}
	}
	startArtist.Tracks = append(startArtist.Tracks, t...)

	// The second layer of the queue will be the targetArtist features

	dbt, err = store.ListTracksByArtistID(context.Background(), targetArtist.ID, 1e6)
	if err != nil {
		log.Printf("Warning: failed to fetch tracks for album %s: %v", targetArtist.Name, err)
	}

	tracks, err = json.Marshal(dbt)
	if err != nil {
		log.Printf("warning: failed to fetch tracks for album %s: %v", targetArtist.Name, err)
	}
	t, _ = targetArtist.CreateTracks(tracks, h)

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
		for _, feat := range tr.Featured {
			mh = createMainHelper(*startArtist, tr, *feat)
			store.UpsertGraph(context.Background(), mh)

		}
	}
	targetArtist.Tracks = append(targetArtist.Tracks, t...)
	// Run the connection search
	helper, path, songs, ok := RunSearchOptsDB(startArtist, targetArtist, depth, verbose, &limit)
	if !ok || len(path) == 0 {
		if depth >= 0 {
			fmt.Printf("No path found between %q and %q within depth %d\n", startArtist.Name, targetArtist.Name, depth)
		} else {
			fmt.Printf("No path found between %q and %q\n", startArtist.Name, targetArtist.Name)
		}
		os.Exit(0)
	}

	// Display the found path
	fmt.Printf("Path found between %q and %q (%d hops):\n\n", startArtist.Name, targetArtist.Name, len(path)-1)
	fmt.Print(songs)
	if switchingArtist {
		for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
			path[i], path[j] = path[j], path[i]
		}
	}

	for i := 1; i < len(path); i++ {
		from := path[i-1]
		to := path[i]
		track := helper.Evidence[from]
		if track != "" {
			fmt.Printf("%d. %s —[%s]→ %s\n", i, from, track, to)
		} else {
			fmt.Printf("%d. %s → %s\n", i, from, to)
		}
	}
	endTime := time.Now().UTC().Unix()
	fmt.Printf("Analysis took %s seconds", strconv.FormatInt(endTime-startTime, 10))
	fmt.Println("\nDone.")
}

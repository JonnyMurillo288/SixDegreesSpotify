package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"time"

	sixdegrees "github.com/Jonnymurillo288/SixDegreesSpotify/sixDegrees"
	"github.com/Jonnymurillo288/SixDegreesSpotify/spotify"
)

// This helper is an easy to save in DB
// This will be just a singluar row in our database for easy search if there is a path
type MainHelper struct {
	ArtistID         string
	ArtistName       string
	FeaturedID       string
	FeaturedName     string
	TrackIDLink      string
	PopularityWeight int16
}

func createMainHelper(artist sixdegrees.Artists, track sixdegrees.Track) MainHelper {
	var feat *sixdegrees.Artists
	if len(track.Featured) > 0 {
		feat = track.Featured[0]
	}
	ret := MainHelper{
		ArtistID:         artist.ID,
		ArtistName:       artist.Name,
		FeaturedID:       feat.ID,
		FeaturedName:     feat.Name,
		TrackIDLink:      track.ID,
		PopularityWeight: int16(artist.Popularity+track.Artist.Popularity) / 2,
	}
	return ret
}

func main() {

	var store, err = Open("")
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

	dba := DBArtist{
		ID:         startArtist.ID,
		Name:       startArtist.Name,
		Popularity: sql.NullInt64{Int64: int64(startArtist.Popularity), Valid: startArtist.Popularity > 0},
		Genres:     startArtist.Genres,
	}

	if err := store.UpsertArtist(context.Background(), dba); err != nil {
		log.Fatalf("Upsert failed: %v", err)
	}

	// Create and Insert Target Artist to the Database if they do not exist in the current database
	// targetArtist := FindArtistByName(context.Background(),find)
	targetArtist := sixdegrees.InputArtist(find)
	if targetArtist == nil || targetArtist.ID == "" {
		log.Fatalf("Target artist %q not found on Spotify.", find)
	}

	dba = DBArtist{
		ID:         targetArtist.ID,
		Name:       targetArtist.Name,
		Popularity: sql.NullInt64{Int64: int64(targetArtist.Popularity), Valid: targetArtist.Popularity > 0},
		Genres:     targetArtist.Genres,
	}

	if err := store.UpsertArtist(context.Background(), dba); err != nil {
		log.Fatalf("Upsert failed: %v", err)
	}

	// Ensure startArtist is the *less popular* one
	if startArtist.Popularity > targetArtist.Popularity {
		switchingArtist = true
		startArtist, targetArtist = targetArtist, startArtist
	}

	// Retrieve albums for the starting artist
	albums, err := spotify.ArtistAlbums(startArtist.ID, limit)
	if err != nil {
		log.Fatalf("Error fetching albums for %s: %v", startArtist.Name, err)
	}

	h := sixdegrees.NewHelper()

	// The first layer of the queue will be the startArtist features
	for _, album := range startArtist.ParseAlbums(albums) {
		dba := DBAlbum{
			ID:              album,
			PrimaryArtistID: sql.NullString{String: startArtist.ID, Valid: startArtist.ID != ""},
		}

		if err := store.UpsertAlbum(context.Background(), dba); err != nil {
			log.Fatalf("Upsert failed: %v", err)
		}

		tracks, err := spotify.GetAlbumTracks(album)
		if err != nil {
			log.Printf("Warning: failed to fetch tracks for album %s: %v", album, err)
			continue
		}
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
		}
		startArtist.Tracks = append(startArtist.Tracks, t...)
	}

	// The second layer of the queue will be the targetArtist features
	targetAlbums, err := spotify.ArtistAlbums(targetArtist.ID, limit)
	if err != nil {
		log.Fatalf("Error fetching albums for %s: %v", targetArtist.Name, err)
	}
	for _, album := range targetArtist.ParseAlbums(targetAlbums) {
		tracks, err := spotify.GetAlbumTracks(album)
		if err != nil {
			log.Printf("Warning: failed to fetch tracks for album %s: %v", album, err)
			continue
		}
		t, _ := targetArtist.CreateTracks(tracks, h)
		targetArtist.Tracks = append(targetArtist.Tracks, t...)
	}

	// Run the connection search
	helper, path, ok := RunSearchOpts(startArtist, targetArtist, depth, verbose, &limit)
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

// ensureSpotifyAuth verifies valid token exists or triggers auth flow.
func ensureSpotifyAuth() error {
	// Ensure auth configuration exists (bootstrap from sample if needed)
	cfg := "./main/authConfig.txt"
	if _, err := os.Stat(cfg); err != nil {
		if os.IsNotExist(err) {
			sample := "./main/authConfig.sample.json"
			if b, rerr := os.ReadFile(sample); rerr == nil {
				_ = os.WriteFile(cfg, b, 0o600)
				return fmt.Errorf("created %s from sample; edit it with your Spotify credentials and re-run", cfg)
			}
			return fmt.Errorf("missing %s; create it with your Spotify app credentials", cfg)
		}
		return fmt.Errorf("failed to check authConfig.txt: %w", err)
	}

	// If token already valid, nothing to do
	if _, ok := tokenValid("./main/authToken.txt"); ok {
		return nil
	}

	// Start the local auth server
	cmd := exec.Command("go", "run", "./main/auth.go")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start auth server: %w", err)
	}

	// Wait for the server to become reachable on localhost:8392
	client := &http.Client{Timeout: 2 * time.Second}
	readyDeadline := time.Now().Add(10 * time.Second)
	var up bool
	for time.Now().Before(readyDeadline) {
		resp, err := client.Get("http://localhost:8392/")
		if err == nil {
			if resp.Body != nil {
				resp.Body.Close()
			}
			up = true
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if !up {
		return fmt.Errorf("authorization server did not start on http://localhost:8392; run `go run ./main/auth.go` manually to inspect errors")
	}

	fmt.Println("Spotify auth server is running on http://localhost:8392/")
	fmt.Println("If your browser does not open automatically, visit the URL above to authorize.")

	// Attempt to open the browser for user authorization (best-effort)
	_ = exec.Command("xdg-open", "http://localhost:8392/").Start()

	// Wait for token to be created and become valid
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		if _, ok := tokenValid("./main/authToken.txt"); ok {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("timeout waiting for Spotify token; complete the authorization in your browser and retry")
}

// tokenValid parses the stored token and checks expiry safety window.
func tokenValid(path string) (*spotify.Auth, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var t spotify.Auth
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, false
	}
	if t.AccessToken == "" {
		return nil, false
	}
	if t.Expires == "" {
		return nil, false
	}
	exp, err := time.Parse(time.RFC3339Nano, t.Expires)
	if err != nil {
		exp, err = time.Parse(time.RFC3339, t.Expires)
	}
	if err != nil {
		return nil, false
	}
	if time.Now().After(exp.Add(-1 * time.Minute)) {
		return nil, false
	}
	return &t, true
}

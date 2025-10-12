package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	sixdegrees "github.com/Jonnymurillo288/SixDegreesSpotify/sixDegrees"
	"github.com/Jonnymurillo288/SixDegreesSpotify/spotify"
)

func main() {
	var start, find string
	var depth int
	var verbose bool

	flag.StringVar(&start, "start", "", "Starting artist name")
	flag.StringVar(&find, "find", "", "Target artist name to find connection to")
	flag.IntVar(&depth, "depth", -1, "Maximum BFS depth in hops (-1 for unlimited)")
	flag.BoolVar(&verbose, "verbose", false, "Enable verbose logging")
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

	// Look up start and target artists
	startArtist := sixdegrees.InputArtist(start)
	if startArtist == nil || startArtist.ID == "" {
		log.Fatalf("Start artist %q not found on Spotify.", start)
	}
	targetArtist := sixdegrees.InputArtist(find)
	if targetArtist == nil || targetArtist.ID == "" {
		log.Fatalf("Target artist %q not found on Spotify.", find)
	}

	// Retrieve albums for the starting artist
	albums, err := spotify.ArtistAlbums(startArtist.ID, 15)
	if err != nil {
		log.Fatalf("Error fetching albums for %s: %v", startArtist.Name, err)
	}

	h := sixdegrees.NewHelper()

	// Populate all tracks for BFS
	for _, album := range startArtist.ParseAlbums(albums) {
		tracks, err := spotify.GetAlbumTracks(album)
		if err != nil {
			log.Printf("Warning: failed to fetch tracks for album %s: %v", album, err)
			continue
		}
		t, _ := startArtist.CreateTracks(tracks, h)
		startArtist.Tracks = append(startArtist.Tracks, t...)
	}

	// Run the connection search
	helper, path, ok := sixdegrees.RunSearchOpts(startArtist, targetArtist, depth, verbose)
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
	for i := 1; i < len(path); i++ {
		from := path[i-1]
		to := path[i]
		track := helper.Evidence[to]
		if track != "" {
			fmt.Printf("%d. %s —[%s]→ %s\n", i, from, track, to)
		} else {
			fmt.Printf("%d. %s → %s\n", i, from, to)
		}
	}
	fmt.Println("\nDone.")
}

// ensureSpotifyAuth verifies valid token exists or triggers auth flow.
func ensureSpotifyAuth() error {
	// Ensure auth configuration exists
	if _, err := os.Stat("./main/authConfig.txt"); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("missing ./main/authConfig.txt; copy and edit ./main/authConfig.sample.json with your Spotify app credentials")
		}
		return fmt.Errorf("failed to check authConfig.txt: %w", err)
	}

	// If token already valid, nothing to do
	if _, ok := tokenValid("./main/authToken.txt"); ok {
		return nil
	}

	// Start the local auth server
	cmd := exec.Command("go", "run", "./main/auth.go")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start auth server: %w", err)
	}

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
	if t.Expires == "" { // treat as non-expiring if missing
		return &t, true
	}
	exp, err := time.Parse(time.RFC3339, t.Expires)
	if err != nil {
		return &t, true
	}
	if time.Now().After(exp.Add(-1 * time.Minute)) { // renew slightly early
		return nil, false
	}
	return &t, true
}

package main

import (
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

func main() {
	startTime := time.Now().UTC().Unix()
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

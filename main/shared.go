package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
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
	TrackNameLink    string
	PopularityWeight int16
}

func createMainHelper(artist sixdegrees.Artists, track sixdegrees.Track, feat sixdegrees.Artists) MainHelper {
	ret := MainHelper{
		ArtistID:         artist.ID,
		ArtistName:       artist.Name,
		FeaturedID:       feat.ID,
		FeaturedName:     feat.Name,
		TrackIDLink:      track.ID,
		TrackNameLink:    track.Name,
		PopularityWeight: int16(artist.Popularity+track.Artist.Popularity) / 2,
	}
	return ret
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

// This function will take in a list of DB track object and return type sixdegrees.Track
// DB TRACK
// ID string
// Name string
// AlbumID nullString
// PrimaryArtistID nullstring
// func (store *Store) DBTracksToTracks(tracks []DBTrack) ([]sixdegrees.Track, error) {
// 	var res []sixdegrees.Track
// 	var t sixdegrees.Track
// 	var f []*sixdegrees.Artists

// 	for _, track := range tracks {
// 		art, err := store.GetArtistByID(context.Background(), track.PrimaryArtistID.String)
// 		a := sixdegrees.CreateArtists(art.Name, art.ID)
// 		if err != nil {
// 			fmt.Println("DBTracksToTracks: Error with getting artist ID")
// 			return nil, fmt.Errorf("error with dbtracks to tracks: %s & %s", art.Name, art.ID)
// 		}
// 		feat, err := store.ListFeaturedArtistsForTrack(context.Background(), track.ID)
// 		for _, fe := range feat {
// 			if a.Name == fe.Name {
// 				continue
// 			}
// 			// fmt.Printf("Artist: %s\nFeatured: %s\n", a.Name, fe.Name)
// 			featArt := sixdegrees.CreateArtists(fe.Name, fe.ID)
// 			f = append(f, featArt)
// 		}
// 		if err != nil {
// 			fmt.Println("DBTracksToTracks: Error with getting artist ID")
// 			return nil, fmt.Errorf("error with dbtracks to tracks")
// 		}

// 		t = sixdegrees.Track{
// 			Artist:   a,
// 			Name:     track.Name,
// 			ID:       track.ID,
// 			Featured: f,
// 		}
// 		res = append(res, t)
// 	}
// 	return res, nil
// }

// // Need a function for offline getting
// // Need a function for online getting
// // Need a function that checks if to do one or other or both

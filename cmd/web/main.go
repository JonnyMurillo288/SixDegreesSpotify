package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"

	sixdegrees "github.com/Jonnymurillo288/SixDegreesSpotify/sixDegrees"
	"github.com/Jonnymurillo288/SixDegreesSpotify/spotify"
)

// Step represents a single hop between two artists
type Step struct {
	From  string
	To    string
	Track string
}

// ResultView is passed to the HTML template for displaying results
type ResultView struct {
	Start   string
	Target  string
	Hops    int
	Steps   []Step
	Message string
}

// Server holds templates and serves HTTP requests
type Server struct {
	formTmpl   *template.Template
	resultTmpl *template.Template
}

func main() {
	// Load templates
	formTmpl := template.Must(template.ParseFiles("templates/path_form.html"))
	resultTmpl := template.Must(template.ParseFiles("templates/path_result.html"))
	s := &Server{formTmpl: formTmpl, resultTmpl: resultTmpl}

	// Kick off background Spotify auth check (non-blocking)
	go func() {
		log.Println("Initializing Spotify auth (this may open a browser once)...")
		if _, err := spotify.SearchArtist("healthcheck"); err != nil {
			log.Printf("Auth initialization warning: %v", err)
		}
	}()

	// Register handlers
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleForm)
	mux.HandleFunc("/search", s.handleSearch)

	addr := "127.0.0.1:8080"
	log.Printf("Web UI listening on http://%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

// Serve form template
func (s *Server) handleForm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.formTmpl.Execute(w, nil); err != nil {
		log.Printf("template execute error (form): %v", err)
		http.Error(w, "Template error", http.StatusInternalServerError)
	}
}

// Handle /search form submission
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form submission", http.StatusBadRequest)
		return
	}

	start := r.FormValue("start")
	target := r.FormValue("find")
	depthStr := r.FormValue("depth")

	if start == "" || target == "" {
		http.Error(w, "Both 'start' and 'find' fields are required", http.StatusBadRequest)
		return
	}

	depth := -1
	if depthStr != "" {
		if d, err := strconv.Atoi(depthStr); err == nil {
			depth = d
		}
	}

	res, err := runSearch(start, target, depth)
	if err != nil {
		log.Printf("Search error: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = s.resultTmpl.Execute(w, ResultView{
			Start: start, Target: target, Message: fmt.Sprintf("Error: %v", err),
		})
		return
	}

	if res == nil || len(res.Steps) == 0 {
		_ = s.resultTmpl.Execute(w, ResultView{
			Start: start, Target: target, Message: "No path found",
		})
		return
	}

	if err := s.resultTmpl.Execute(w, res); err != nil {
		log.Printf("template execute error (result): %v", err)
	}
}

// Core search logic
func runSearch(start, target string, depth int) (*ResultView, error) {
	// Look up artists
	srcArtist := sixdegrees.InputArtist(start)
	if srcArtist == nil || srcArtist.ID == "" {
		return &ResultView{Start: start, Target: target, Message: "Start artist not found"}, nil
	}
	dstArtist := sixdegrees.InputArtist(target)
	if dstArtist == nil || dstArtist.ID == "" {
		return &ResultView{Start: start, Target: target, Message: "Target artist not found"}, nil
	}

	// Fetch albums
	albums, err := spotify.ArtistAlbums(srcArtist.ID, 15)
	if err != nil {
		return nil, fmt.Errorf("artist albums: %w", err)
	}

	h := sixdegrees.NewHelper()

	// Populate artist tracks
	for _, album := range srcArtist.ParseAlbums(albums) {
		tracks, err := spotify.GetAlbumTracks(album)
		if err != nil {
			log.Printf("Warning: failed to fetch tracks for album %s: %v", album, err)
			continue
		}
		t, _ := srcArtist.CreateTracks(tracks, h)
		srcArtist.Tracks = append(srcArtist.Tracks, t...)
	}

	// Run the actual graph search
	helper, path, ok := sixdegrees.RunSearchOpts(srcArtist, dstArtist, depth, false)
	if !ok || len(path) == 0 {
		return &ResultView{Start: srcArtist.Name, Target: dstArtist.Name, Message: "No path found"}, nil
	}

	// Build step list
	steps := make([]Step, 0, len(path)-1)
	for i := 1; i < len(path); i++ {
		from := path[i-1]
		to := path[i]
		track := helper.Evidence[to]
		steps = append(steps, Step{From: from, To: to, Track: track})
	}

	return &ResultView{
		Start:  srcArtist.Name,
		Target: dstArtist.Name,
		Hops:   len(path) - 1,
		Steps:  steps,
	}, nil
}

package main

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"os"

	"github.com/Jonnymurillo288/SixDegreesSpotify/spotify"
)

// ---- Structs for frontend search ----
type searchRequest struct {
	Start  string `json:"start"`
	Target string `json:"target"`
	Depth  int    `json:"depth"`
}

type step struct {
	From     string `json:"from"`
	Track    string `json:"track"`
	To       string `json:"to"`
	TrackID  string `json:"trackID"`
	TrackURL string `json:"trackURL"`
}

type searchResponse struct {
	Start   string `json:"start"`
	Target  string `json:"target"`
	Hops    int    `json:"hops"`
	Path    []step `json:"path"`
	Message string `json:"message,omitempty"`
	Status  int    `json:"status"`
}

// ------------------------------------------------------------
// GLOBAL memory store for the last search result (edge list)
// ------------------------------------------------------------

var lastSearchEdges []step

// ------------------------------------------------------------
// MAIN
// ------------------------------------------------------------
func main() {
	mux := http.NewServeMux()

	// ---- Templates ----
	tmpl := template.Must(template.ParseFiles("templates/graph.html"))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := tmpl.Execute(w, nil); err != nil {
			log.Println("template error:", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
	})

	// ----------------------------------------------------------------------
	// /expandNode  — NO DB — simply returns all edges from last search
	// This is all you asked for: "just use the edge result thing"
	// ----------------------------------------------------------------------
	mux.HandleFunc("/expandNode", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var body struct {
			Artist string `json:"artist"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		if body.Artist == "" {
			http.Error(w, "artist required", http.StatusBadRequest)
			return
		}

		// ----------------------------------------------------
		// Build Sigma-compatible edges from lastSearchEdges
		// ----------------------------------------------------
		type EdgeResult struct {
			From     string `json:"from"`
			To       string `json:"to"`
			Track    string `json:"track"`
			TrackID  string `json:"trackID"`
			TrackURL string `json:"trackURL"`
		}

		out := []EdgeResult{}

		for _, s := range lastSearchEdges {
			if s.From == body.Artist || s.To == body.Artist {
				out = append(out, EdgeResult{
					From:     s.From,
					To:       s.To,
					Track:    s.Track,
					TrackID:  s.TrackID,
					TrackURL: s.TrackURL,
				})
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	})

	// ---- Status check ----
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})

	// ---- Spotify OAuth ----
	mux.HandleFunc("/auth", HomePage)
	mux.HandleFunc("/callback", Authorize)

	// ----------------------------------------------------------------------
	// /search — does the big path search & stores the edge results in memory
	// ----------------------------------------------------------------------
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req searchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if req.Start == "" || req.Target == "" {
			http.Error(w, "start and target required", http.StatusBadRequest)
			return
		}

		// Default depth
		if req.Depth == 0 {
			req.Depth = -1
		}

		limit := 1200

		hops, stepsList, message, status, err := SearchArtists(
			req.Start, req.Target, req.Depth, limit, false,
		)
		if err != nil {
			if status == 429 {
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error":   "rate_limited",
					"message": message,
					"status":  429,
				})
				return
			}
			http.Error(w, "search failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Save edges globally for graph expansion later
		lastSearchEdges = nil
		for _, s := range stepsList {
			lastSearchEdges = append(lastSearchEdges, step{
				From:     s.From,
				Track:    s.Track,
				To:       s.To,
				TrackID:  s.TrackID,
				TrackURL: s.TrackURL,
			})
		}

		resp := searchResponse{
			Start:   req.Start,
			Target:  req.Target,
			Hops:    hops,
			Message: message,
			Status:  status,
		}

		for _, s := range stepsList {
			resp.Path = append(resp.Path, step{
				From:     s.From,
				Track:    s.Track,
				To:       s.To,
				TrackID:  s.TrackID,
				TrackURL: s.TrackURL,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	// ----------------------------------------------------------------------
	// /createPlaylist
	// ----------------------------------------------------------------------
	mux.HandleFunc("/createPlaylist", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Name     string   `json:"name"`
			TrackIDs []string `json:"trackIDs"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if req.Name == "" || len(req.TrackIDs) == 0 {
			http.Error(w, "name and trackIDs required", http.StatusBadRequest)
			return
		}

		url, err := spotify.CreatePlaylist(req.Name, req.TrackIDs)
		if err != nil {
			http.Error(w, "playlist creation failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"url":    url,
			"status": "success",
		})
	})

	// ---- Server startup ----
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Println("listening on :" + port)

	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}

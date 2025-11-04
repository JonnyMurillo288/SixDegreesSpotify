package main

import (
	"context"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"os"
)

// ---- Structs for frontend search ----
type searchRequest struct {
	Start  string `json:"start"`
	Target string `json:"target"`
	Depth  int    `json:"depth"`
}

type step struct {
	From  string `json:"from"`
	Track string `json:"track"`
	To    string `json:"to"`
}

type searchResponse struct {
	Start   string `json:"start"`
	Target  string `json:"target"`
	Hops    int    `json:"hops"`
	Path    []step `json:"path"`
	Message string `json:"message,omitempty"`
}

func main() {
	mux := http.NewServeMux()

	// ---- Templates ----
	tmpl := template.Must(template.ParseFiles("templates/index.html"))

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

	// ---- Status check ----
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})

	// ---- Spotify OAuth ----
	mux.HandleFunc("/auth", HomePage)      // starts OAuth flow
	mux.HandleFunc("/callback", Authorize) // handles redirect from Spotify

	// ---- Search API ----
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
			http.Error(w, "start and target are required", http.StatusBadRequest)
			return
		}

		// Defaults
		if req.Depth == 0 {
			req.Depth = -1
		}
		limit := 1000 // or make this configurable later

		ctx := context.Background()
		store, err := Open("") // your DB open function; pass path/config as needed
		if err != nil {
			http.Error(w, "database open failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer store.Close()

		hops, steps, message, err := SearchArtists(ctx, store, req.Start, req.Target, req.Depth, limit, false)
		if err != nil {
			http.Error(w, "search failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		resp := searchResponse{
			Start:   req.Start,
			Target:  req.Target,
			Hops:    hops,
			Message: message,
		}

		for _, s := range steps {
			resp.Path = append(resp.Path, step{
				From:  s.From,
				Track: s.Track,
				To:    s.To,
			})
		}

		// If there was a message (like "No path found"), return gracefully
		if message != "" && len(resp.Path) == 0 {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, "failed to encode response: "+err.Error(), http.StatusInternalServerError)
		}
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

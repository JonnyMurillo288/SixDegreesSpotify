package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	"golang.org/x/oauth2"
)

// OAuth2 endpoints for Spotify
var Endpoint = oauth2.Endpoint{
	AuthURL:  "https://accounts.spotify.com/authorize",
	TokenURL: "https://accounts.spotify.com/api/token",
}

// Struct to load credentials from JSON
type preconfig struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	RedirectURL  string   `json:"redirect_url"`
	Scopes       []string `json:"scopes"`
}

var config *oauth2.Config

// HomePage starts OAuth flow
func HomePage(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Redirecting user to Spotify authorization...")
	config = createConfig()
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// Authorize handles Spotify redirect and saves token
func Authorize(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Received callback from Spotify...")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	code := r.Form.Get("code")
	if code == "" {
		http.Error(w, "Authorization code missing", http.StatusBadRequest)
		return
	}

	if config == nil {
		config = createConfig()
	}

	token, err := config.Exchange(context.Background(), code)
	if err != nil {
		http.Error(w, "Failed to exchange code for token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	st := struct {
		AccessToken string `json:"access_token"`
		Type        string `json:"token_type"`
		Refresh     string `json:"refresh_token"`
		Expires     string `json:"expiry"`
	}{
		AccessToken: token.AccessToken,
		Type:        token.TokenType,
		Refresh:     token.RefreshToken,
		Expires:     token.Expiry.Format(time.RFC3339Nano),
	}
	tokenJSON, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		http.Error(w, "Failed to marshal token", http.StatusInternalServerError)
		return
	}

	tokenPath := "./main/authToken.txt"
	tokenFile, err := os.OpenFile(tokenPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		http.Error(w, "Error writing to token file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer tokenFile.Close()

	if _, err := tokenFile.Write(tokenJSON); err != nil {
		http.Error(w, "Failed to write token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "Authorization complete! Token saved to %s\n", tokenPath)
	fmt.Println("Token written to file successfully.")
}

// Load credentials and create OAuth2 config
func createConfig() *oauth2.Config {
	data, err := os.ReadFile("./main/authConfig.txt")
	if err != nil {
		panic("Failed to read authConfig.txt: " + err.Error())
	}

	var p preconfig
	if err := json.Unmarshal(data, &p); err != nil {
		panic("Failed to parse authConfig.txt: " + err.Error())
	}

	return &oauth2.Config{
		ClientID:     p.ClientID,
		ClientSecret: p.ClientSecret,
		Endpoint:     Endpoint,
		RedirectURL:  p.RedirectURL,
		Scopes:       p.Scopes,
	}
}

func main() {
	router := mux.NewRouter()
	router.HandleFunc("/", HomePage)
	router.HandleFunc("/auth", Authorize)

	fmt.Println("Server running on http://localhost:8392/")
	if err := http.ListenAndServe(":8392", router); err != nil {
		fmt.Println("Error starting server:", err)
	}
}

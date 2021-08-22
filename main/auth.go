package main

/**********************************************
Get the authorization for the client and send it to a file
for communication with python scripts.



**********************************************/

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"net/http"

	"github.com/gorilla/mux"

	"golang.org/x/oauth2"
)

// copied from oauth2.spotify.Endpoint
var Endpoint = oauth2.Endpoint{
	AuthURL:  "https://accounts.spotify.com/authorize",
	TokenURL: "https://accounts.spotify.com/api/token",
}

// write this then transfer the data to the real config type
type preconfig struct {
	ClientID string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURL string `json:"redirect_url"`
	Scopes []string `json:"scopes"`
}

var config *oauth2.Config


// Use this for the login page
func HomePage(w http.ResponseWriter, r *http.Request){
    // w.Write([]byte("Hello, this is the Home Page"))
    fmt.Println("Sending to Spotify!")
	config = createConfig()
    u := config.AuthCodeURL(Endpoint.AuthURL)
    http.Redirect(w, r, u, http.StatusFound)
}

// Authorize the client and get the code and token
func Authorize(w http.ResponseWriter, r *http.Request) {
    fmt.Println("Authorizing!")
    r.ParseForm()

    code := r.Form.Get("code")
    if code == "" {
        http.Error(w, "Code not found", http.StatusBadRequest)
        return 
    }

    token, err := config.Exchange(context.Background(), code)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return 
    }

    res,_ := json.Marshal(token)
    newRes := string(res)

    // write the token to a token File for communicaiton with the python scripts
    tokenFile, err := os.OpenFile("./main/authToken.txt",os.O_WRONLY,7775)
    if err != nil {
        fmt.Println("Error writing to the token file:",err.Error())
    }
    defer tokenFile.Close()
    tokenFile.WriteString(newRes)
}

// create a config object to do the redirecting and assigning an http.client
func createConfig() *oauth2.Config {
    var p preconfig
	f,err := ioutil.ReadFile("./main/authConfig.txt")
    if err != nil {
        panic(err)
    }
	err = json.Unmarshal(f,&p)
	if err != nil {
		panic(err)
	}
    return &oauth2.Config{
        ClientID: p.ClientID,
        ClientSecret: p.ClientSecret,
        Endpoint: Endpoint,
        RedirectURL: p.RedirectURL,
        Scopes: p.Scopes,
    }
}



func main() {
	mx := mux.NewRouter() //.SkipClean(true)
	mx.HandleFunc("/", HomePage)
	mx.HandleFunc("/auth", Authorize)

	http.ListenAndServe(":8392", mx)
}
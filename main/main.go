package main

/*********************************************
WEBSOCKETS NOTES:

After joining a room the client will open up a websocket for more transmission of data for the
messages

- Clients: redirected to URL/room and sees all messages sent through the socket

**********************************************/

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

var PORT string = ":8392"
var templates *tpl = newTemplate()
const (
	loginURL = "http://localhost:8392"
	joinURL = "http://localhost:8392/join"
	roomURL = "http://localhost:8392/room"
	wsURL = "http://localhost:8392/ws"
)

func (c *client) serveRoom(w http.ResponseWriter, r *http.Request) {
	log.Println(r.URL)
	if r.URL.Path != "/recommend" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	templates.Templates["home"].Execute(w,c)
	// http.ServeFile(w, r, "/Users/jonnymurillo/Desktop/Chat-Room/templates/home.html")
}


func generateKey() (string, error) {
    p := make([]byte, 16)
    if _, err := io.ReadFull(rand.Reader, p); err != nil {
        return "", err
    }
    return base64.StdEncoding.EncodeToString(p), nil
}


func main() {
	
	mx := mux.NewRouter()

	s := newServer("PORT",mx)
	c := s.newClient()

	go s.run()

	templates.createTemplate("newClient")
	templates.createTemplate("index")
	templates.createTemplate("rooms")
	templates.createTemplate("home")
	templates.createTemplate("loading")
	templates.createTemplate("gather")
	templates.createTemplate("recommend")
	
	fmt.Println("connected to server at",PORT)
	mx.HandleFunc("/", c.login) // asks user for name
	mx.HandleFunc("/ws",func(w http.ResponseWriter, r *http.Request) {
		wsKey, err := generateKey()
		if err != nil {
			log.Printf("Cannot generate challenge key %v", err)
		}
	
		r.Header.Add("Connection", "Upgrade")
		r.Header.Add("Upgrade", "websocket")
		r.Header.Add("Sec-WebSocket-Version", "13")
		r.Header.Add("Sec-WebSocket-Key", wsKey)
	
		log.Printf("ws key '%v'", wsKey)
	
		c.serveWs(s,w,r)
	})
	mx.HandleFunc("/recommend",c.displayRecommendation)
	mx.HandleFunc("/recommend/player",c.serveRoom)
	mx.HandleFunc("/loading",checkData)


	log.Fatal(http.ListenAndServe(PORT,mx))
		

}

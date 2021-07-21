package main

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

type Server struct {
	Clients map[*client]bool
	Sv *http.Server
	register chan *client
	unregister chan *client
	cmd chan Command

}

func newServer(addr string, mx *mux.Router) *Server {
	fmt.Println("Created a server")
	return &Server{
		Clients: make(map[*client]bool),
		Sv: &http.Server{
			Addr:              addr,
			Handler:           mx,
			TLSConfig:         &tls.Config{},
			ReadTimeout:       15 * time.Second,
			ReadHeaderTimeout: 0,
			WriteTimeout:      15 * time.Second,
			IdleTimeout:       0,
			MaxHeaderBytes:    0,
		},
		register: make(chan *client),
		unregister: make(chan *client),
		cmd: make(chan Command),
		}
	}	


// runs the server to accept of send messgages and closes out the client socket
func (s *Server) run() {
	for {
		select {
		case client := <-s.register:
			s.Clients[client] = true
		case client := <-s.unregister:
			fmt.Println("Unregistering the client")
			if _,ok := s.Clients[client]; ok {
				delete(s.Clients,client)
				close(client.send)
			}
		for c := range s.cmd {
			switch c.id {
			case BACK:
				controller("https://api.spotify.com/v1/me/player/previous")
			case PLAY:
				controller("https://api.spotify.com/v1/me/player/play")
			case SKIP:
				controller("https://api.spotify.com/v1/me/player/next")
			case PAUSE:
				controller("https://api.spotify.com/v1/me/player/pause")
			}
		}
		
			
		}
	}
}

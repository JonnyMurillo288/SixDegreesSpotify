package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)


const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second
	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second
	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10
	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

var (
	newline = []byte{'\n'}
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}


type client struct {
	Name string
	Cli *http.Client
	// added for the websocket implementation
	Server *Server //
	conn *websocket.Conn //
	send chan []byte //
}


// create a client object for the site
// empty websocket until they have provide auth and 
// have gotten their data collected by SpotifyAPI (python)
func (s *Server) newClient() *client {
	return &client{
		Cli: &http.Client{
			Timeout: 20 * time.Second,
			Transport: &http.Transport{
				DisableKeepAlives: false,
				Proxy:             http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
					DualStack: true,
				}).DialContext,
				MaxIdleConns:          100,
				IdleConnTimeout:       50 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
				
		},
		// CheckRedirect: func(req *http.Request, via []*http.Request) error {
        //     return http.ErrUseLastResponse
        },//},
		Server: s,
		conn: &websocket.Conn{},
		send: make(chan []byte),
	}
}

//reads messages pumped from the websocket connection to the server
func (c *client) readPump() {
	defer func() {
		fmt.Println("closing the socket")
		c.Server.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { 
		c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil
	})
	for {
		_,message,err := c.conn.ReadMessage()
		log.Println("Read message from the client:",string(message))
		
		if err != nil {
			if websocket.IsUnexpectedCloseError(err,websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error with closing socket: %s",err.Error())
			}
			break
		}
		// handles the message sent to the server from the client
		switch string(message) {
		// controller messages
		case "back":
			c.Server.cmd <- Command{
				id: BACK,
			}
		case "play":
			c.Server.cmd <- Command{
				id: PLAY,
			}
		case "pause":
			c.Server.cmd <- Command{
				id: PAUSE,
			}
		case "skip":
			c.Server.cmd <- Command{
				id: SKIP,
			}
		case "token":
			c.send <- getAuthToken()
		case "playback":
			p,err := json.Marshal(getPlayback())
			if err != nil {
				log.Fatal(err)
			}
			c.send <- p
		case "queue":
			queueFromJS()
		}
	}
}

func getAuthToken() []byte{
	f, err := ioutil.ReadFile("authToken.txt")
	if err != nil {
		log.Fatal(err)
	}
	return f
}

func (c *client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message,ok := <-c.send:
			log.Println("Sending the message back to the client",string(message))
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// if c.send channel is closed, close the socket
				c.conn.WriteMessage(websocket.CloseMessage,[]byte{})
				return
			}
			
			// prepare the writer by calling nextwriter
			w,err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				log.Printf("Error with writing the message: %s",err.Error())
				return 
			}
			log.Print("This is the message:",string(message))
			w.Write(message)

			if err := w.Close(); err != nil {
				return
			}
		
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage,nil); err != nil {
				return
			}
		}
	}
}

// handles websocket requests from the peer
// pass in existing client from their login and the room they are joining 
func (c *client) serveWs(s *Server, w http.ResponseWriter, r *http.Request) {
	conn,err := upgrader.Upgrade(w,r,nil)
	if err != nil {
		log.Println("error upgrading to websocket:",err.Error())
		return
	}

	c.Server = s
	c.conn = conn
	c.send = make(chan[]byte, 256)
	c.Server.register <- c

	go c.writePump()
	go c.readPump()
}


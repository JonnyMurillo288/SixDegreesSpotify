package main

import (
	"net/http"
	"testing"

	"github.com/gorilla/mux"
)

/**********************************************

TESTS:
- Server:
	-̶ s̶e̶r̶v̶e̶r̶ o̶p̶e̶n̶s̶ a̶n̶d̶ i̶s̶ a̶b̶l̶e̶ t̶o̶ r̶u̶n̶ t̶h̶e̶ h̶a̶n̶d̶l̶e̶r̶s̶
	-̶ e̶a̶c̶h̶ r̶e̶d̶i̶r̶e̶c̶t̶ t̶o̶ n̶e̶x̶t̶ p̶a̶g̶e̶
	-̶ o̶a̶u̶t̶h̶
	-̶ w̶r̶i̶t̶e̶s̶ t̶o̶k̶e̶n̶ t̶o̶ f̶i̶l̶e̶ f̶o̶r̶ p̶y̶t̶h̶o̶n̶ t̶o̶ r̶e̶a̶d̶ i̶f̶ n̶e̶e̶d̶ b̶e̶
- login:
	- failed, redirect register
	- ask to register
	- success
- client:
	- reads from html files
	- opens socket

- configure:
	-̶ c̶r̶e̶a̶t̶e̶s̶ c̶o̶n̶f̶i̶g̶ f̶r̶o̶m̶ f̶i̶l̶e̶
	-

- recommend
	- reads recommend file

- spotify:
	- test playback
	- test playback sent to websocket

***********************************************/

/*
 */
// Testing auth token and sending is to auth file
func TestAuth(t *testing.T) {
	mx := mux.NewRouter()//.SkipClean(true)
	mx.HandleFunc("/",HomePage)
	mx.HandleFunc("/auth",Authorize)

	http.ListenAndServe(":8392",mx)
}


/*
Testing playback and currently playing

func TestPlayback(t *testing.T) {
	mx := mux.NewRouter()
	s := newServer(":8932",mx)

	pb := s.getPlayback()
	fmt.Println(pb)
}
*/

/*
Testing the databases and that we are getting the correct data from it
*/

/*

func TestGather(t *testing.T) {
	f, err := os.OpenFile("testlogfile", os.O_RDWR | os.O_CREATE | os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer f.Close()

	log.SetOutput(f)
	log.Println("This is a test log entry")

	templates.createTemplate("index")
	templates.createTemplate("recommend")
	templates.createTemplate("loading")
	templates.createTemplate("home")
	templates.createTemplate("playing")
	templates.createTemplate("empty")

	mx := mux.NewRouter()//.SkipClean(true)
	s := newServer(":8392",mx)
	go s.run()

	c := s.newClient()
	fs := http.StripPrefix("/js/",http.FileServer(http.Dir("./js")))

	mx.HandleFunc("/",HomePage)
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
	mx.HandleFunc("/auth",Authorize)
	mx.HandleFunc("/gather",checkData)
	mx.HandleFunc("/recommend",c.displayRecommendation)
	mx.Handle("/js/",fs)
	mx.HandleFunc("/recommend/undefined",func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("This is the undefined page!"))
	})
	mx.HandleFunc("/recommend/{playlist}",c.displaySelected)

	http.ListenAndServe(":8392",mx)

}

*/

/*
//  */
// func TestReadPlaylists(t *testing.T) {
	
	
// 	fmt.Println(readPlaylist())
	
// }

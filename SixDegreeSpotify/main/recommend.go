package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
)

/*******************************************
this will check the users data has been collected after they have logged in
- Display a loading page when directed to this section
-
********************************************/

var playlist string

type Track struct {
	TrackName string
	TrackPhoto string
	TrackID string
}

// [playlist name] = track.trackname, track.trackphoto
type Playlists struct {
	Playlists map[string][]Track
	// for name,tracks := Playlists.playlists {
	//  	
	//}
}


// handler func for displaying recommended playlists
// p := readPlaylist()
// http.HandleFunc("/recommendations", p.displayRecommenations)
func (c *client) displayRecommendation(w http.ResponseWriter, r *http.Request) {
	log.Print(r.URL)
	if r.URL.Path != "/recommend" {
		http.Error(w,"Path Not Found",http.StatusNotFound)
	}
	if r.Method != "GET" {
		http.Error(w,"Method not allowed",http.StatusMethodNotAllowed)
	}
	p,err := json.Marshal(readPlaylist())
	if err != nil {
		log.Fatal(err)
	}
	if err := templates.Templates["recommend"].Execute(w,string(p)); err != nil {
		log.Print(err)
	}
}

// read the playlist that was selected and add the first 3 songs to their queue
// send the rest of the songs to toBeQueued.txt for the client to read 
func (c *client) displaySelected(w http.ResponseWriter, r *http.Request) {
	go c.Server.run()
	log.Println(r.URL.Path)
	var tracks []string
	input := make(map[string]string)

	split := strings.Split(r.URL.Path,"/")
	playlist = split[len(split)-1]
	if r.Method != "GET" {
		http.Error(w, "Mehod not allowed",http.StatusMethodNotAllowed)
	}
	p := readPlaylist().Playlists[playlist]
	f,err := json.Marshal(p)
	if err != nil {
		log.Print(err)
	}
	input[playlist] = string(f)
	in, err := json.Marshal(input)
	if err != nil {
		log.Fatalf("Error Marshalling Playlist: %s, err: %s",playlist,err.Error())
	}
	for i,track := range p {
		if i < 3 {
			tracks = append(tracks,track.TrackID)
		}
	}
	log.Println("Adding tracks to queue and removing them from the rec file:\n\n",tracks)
	addQueue(tracks)
	removeRec(tracks)

	templates.Templates["playing"].Execute(w,string(in))
}

// appends the queue after being instructed from JS
func queueFromJS() {
	var tracks []string
	p := readPlaylist().Playlists[playlist]
	next := p[0]
	tracks = append(tracks,next.TrackID)
	addQueue(tracks)
}


// reads playlist file and creates a playlists object
func readPlaylist() Playlists {
	var t Track // trackname, trackphoto
	var ts []Track
	var prev, name string // playlist name
	prev = ""
	p := Playlists{ // p[name] = t
		make(map[string][]Track),
	} 
	f,err := os.Open("recommendedTracks.txt")
	if err != nil {
		log.Println(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		line := strings.Split(scanner.Text(),",")
		name = line[0]
		fmt.Println("Name of the playlist:",name,len(ts))
		if line[0] != prev{
			fmt.Println("====")
			p.Playlists[prev] = ts
			name = line[0] // playlist name
			ts = ts[:len(ts)-1]
		}
		prev = name
		t.TrackName = line[1] // track name
		t.TrackPhoto = line[2] // track photo
		t.TrackID = line[3] // id for track, manually add uri for the spotify req
		ts = append(ts,t)
	}
	p.Playlists[name] = ts
	fmt.Println("Playlists object is:",p)

	if err != nil {
		panic(err)
	}
	return p
}


func removeRec(tracks []string) {
	var trackID string
	var lines []string
	f,err := os.Open("./recommendedTracks.txt")
	if err != nil {
		log.Fatal(err)
	}
	
	if err != nil {
		log.Fatal(err)
	}
	scanner := bufio.NewScanner(f)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		line := strings.Split(scanner.Text(),",")
		trackID = line[3]
		if !checkDup(trackID,tracks) {
			lines = append(lines,scanner.Text())
		}
	}
	f.Close()
	os.Remove("./recommendedTracks.txt")
	f,err = os.Create("./recommendedTracks.txt")
	if err != nil {
		log.Println(err)
	}

	for _,l := range lines {
		fmt.Println(l)
		f.WriteString(l + "\n")
	}
}

// checks an array if it is already in the array
// function like "if x in:" from python
func checkDup(a string, t []string) bool{
	for _,s := range t {
		if a == s {
			return true
		}
	}
	return false
}

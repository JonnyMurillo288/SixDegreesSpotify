package main

import (
	"encoding/gob"
	"flag"
	"fmt"
	"log"
	"os"

	sixdegrees "github.com/Jonnymurillo288/SixDegreesSpotify/sixDegrees"
	"github.com/Jonnymurillo288/SixDegreesSpotify/spotify"
)

func main() {
	var start, find string
	flag.StringVar(&start, "start", "", "Starting artist")
	flag.StringVar(&find, "find", "", "Find artist from the starting artist")
	flag.Parse()

	g := sixdegrees.NewGraph()

	art := sixdegrees.InputArtist("lil wayne")
	target := sixdegrees.InputArtist("YG")
	albums, _ := spotify.ArtistAlbums(art.ID, 15)
	h := sixdegrees.NewHelper()
	for _, al := range art.ParseAlbums(albums) {
		tr, _ := spotify.GetAlbumTracks(al)
		T, _ := art.CreateTracks(tr, h)
		art.Tracks = append(art.Tracks, T...)
	}
	searchHelp, ret := sixdegrees.RunSearch(art, target)
	if !ret {
		log.Fatalf("Could not find (%v) target %v", target.Name, searchHelp.ArtistMap)
		os.Exit(1)
	}
	// for artistname, []*Artists
	for k, artists := range searchHelp.EdgeTo {
		// v = ArtistObject
		v := searchHelp.ArtistMap[k]
		// edgefrom v to w
		for _, w := range artists {
			g.AddEdge(sixdegrees.NewEdge(target, v, w))
		}
	}

	saveDecoder((*g))

	dij := sixdegrees.NewDijkstras(g, art)
	for _, e := range dij.EdgeTo {
		v := g.Keys[e.From()]
		w := g.Keys[e.To()]
		fmt.Printf("\nPath: %v --> %v : %v", v, w, dij.DistTo[w])
	}
}

// Create an ecoder file and save object.
func saveDecoder(g sixdegrees.Graph) bool {
	file, err := os.OpenFile("./weightedSpotifyGraph.gob", os.O_APPEND|os.O_WRONLY, os.ModeAppend)
	if err != nil {
		panic(err)
	}
	enc := gob.NewEncoder(file)
	err = enc.Encode(g)
	if err != nil {
		log.Fatal("Encode:", err)
		return false
	}
	return true
}

// Create a decoder and receive a value.
func returnDecoder() sixdegrees.Graph {
	file, err := os.Open("./weightedSpotifyGraph.gob")
	if err != nil {
		panic(err)
	}

	dec := gob.NewDecoder(file)
	var g sixdegrees.Graph
	err = dec.Decode(&g)
	if err != nil {
		log.Fatal("Decode:", err)
	}
	return g
}

package main

import (
	"encoding/gob"
	"flag"
	"fmt"
	"log"
	"os"

	sixdegrees "github.com/Jonnymurillo288/SixDegreeSpotify/sixDegrees"
	"github.com/Jonnymurillo288/SixDegreeSpotify/spotify"
)

func main() {
	var start,find string
	flag.StringVar(&start,"start","","Starting artist")
	flag.StringVar(&find,"find","","Find artist from the starting artist")
	flag.Parse()
	if len(start) == 0 || len(find) == 0 {
		fmt.Printf("Error with inputs for the start: %s or find:%s\nExiting!!\n",start,find)
		os.Exit(1)
	}

	g := sixdegrees.NewGraph()

	art := sixdegrees.InputArtist(start)
	target := sixdegrees.InputArtist(find)
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
	dij := sixdegrees.NewDijkstras(g, art)
	for _, e := range dij.EdgeTo {
		v := g.Keys[e.From()]
		w := g.Keys[e.To()]
		fmt.Printf("\nPath: %v --> %v : %v", v, w, dij.DistTo[w])
	}
}


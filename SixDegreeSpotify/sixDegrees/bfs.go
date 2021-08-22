package sixdegrees

import (
	"fmt"

	"github.com/Jonnymurillo288/SixDegreeSpotify/spotify"
)

// Searching a tree helper function
type Helper struct {
	ArtistMap map[string]*Artists // Used to see if we have visited this artist yet
	DistTo map[string]int // distance to an artist from the source
	EdgeTo map[string][]*Artists // Edge from one artist to the next artist, only change if we find a shorter distTo[Name]
}

func NewHelper() *Helper {
	return &Helper{
		ArtistMap: make(map[string]*Artists),
		DistTo: make(map[string]int),
		EdgeTo: make(map[string][]*Artists),
	}
}


func RunSearch(art *Artists, target *Artists) (*Helper, bool) {
	
	h := NewHelper()
	var q []*Artists
	q = append(q,art)
	
	return h.bfs(target.Name,q)
}

func isEmpty(q []*Artists) bool {
	return len(q) == 0
}

func (h *Helper) bfs(target string, q []*Artists) (*Helper, bool) {
	for !isEmpty(q) {
		fmt.Println("================ Queue Len:",len(q))
		art := q[0]
		q = q[1:]
		fmt.Printf("\nSearching %v tracks for %v\nTarget: %v",len(art.Tracks),art.Name,target)
		for _,track := range art.Tracks {
			if track.Artist.Name == target {
				fmt.Println("WE FOUND THE TARGET!!!",target)
				return h,true
			}
			for _,feat := range track.Featured {
				if feat.Name == art.Name {
					continue
				}
				if !inArr(h.EdgeTo, feat.Name, track.Artist){
					h.EdgeTo[feat.Name] = append(h.EdgeTo[feat.Name],track.Artist)
				}
				if _,ok := h.ArtistMap[feat.Name]; !ok {
					albums,_ := spotify.ArtistAlbums(feat.ID,10)
					for _,al := range feat.ParseAlbums(albums) {
						tr, _ := spotify.GetAlbumTracks(al)
						T,_ := feat.CreateTracks(tr,h)
						if r,i := checkFeat(T,target); r{
							albums,_ := spotify.ArtistAlbums(i,10)
							for _,al := range feat.ParseAlbums(albums) {
								tr, _ := spotify.GetAlbumTracks(al)
								T,_ := feat.CreateTracks(tr,h)
								feat.Tracks = append(feat.Tracks,T...)
							}
							return h,true
						}
						feat.Tracks = append(feat.Tracks,T...)
					}
					q = append(q, feat)
					h.ArtistMap[feat.Name] = feat
				}
				if feat.Name == target {
					albums,_ := spotify.ArtistAlbums(feat.ID,10)
					for _,al := range feat.ParseAlbums(albums) {
						tr, _ := spotify.GetAlbumTracks(al)
						T,_ := feat.CreateTracks(tr,h)
						feat.Tracks = append(feat.Tracks,T...)
					}
					fmt.Println("Adding",feat.Name,"to the queue")
					h.ArtistMap[feat.Name] = feat
					fmt.Println("WE FOUND THE TARGET IN FEATURED",target)
					return h,true
				}
			}
		}
	}
	_,ok := h.ArtistMap[target] // check to see if the target was reached
	return h, ok
}

// check the featured tracks to see if the target artist is in there
func checkFeat(tracks []Track, target string) (bool,string) {
	for _,track := range tracks {
		if track.Artist.Name == target {
			return true,track.Artist.ID
		}
	}
	return false,""
}

func inArr(arr map[string][]*Artists, feat string, art *Artists) bool {
	for _,a := range arr[feat] {
		if a.Name == art.Name {
			return true
		} 
	}
	return false
}
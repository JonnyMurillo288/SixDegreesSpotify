package sixdegrees

import (
	"encoding/json"
	"log"

	"github.com/Jonnymurillo288/SixDegreeSpotify/spotify"
)

type Artists struct {
	Name string 
	ID string
	Tracks []Track
	Popularity float64
	PopularityKeys, NumFeatKeys []int // keys to sort and find in the Tracks list
	Genres map[string]int // add value for genres so we can search from artist 2 faster
}



type res struct {
	Artists interface{} `json:"artists"`
}


func InputArtist(name string) *Artists {
	// fmt.Println("Input Artist ",name)
	var r *res
	var a = &Artists{
		Tracks: make([]Track,0),
		PopularityKeys: []int{},
		NumFeatKeys: []int{},
		Genres: make(map[string]int),
	}
	ret,err := spotify.SearchArtist(name)
	if err != nil {
		log.Fatalf("Error with returning Artist %s",name)
	}
	err = json.Unmarshal(ret,&r)
	if err != nil {
		log.Printf("Error with the input artist %v: %v",name,err)
		a.Name = name 
		return a
	}
	switch r.Artists.(type) {
	case map[string]interface{}:
		log.Print("Artists input:",name)
	default:
		log.Printf("Error with the input artist: %v \n%v",name,r.Artists)
		return nil
	}
	itemMap := r.Artists.(map[string]interface{})
	for k,v := range itemMap {
		if k == "items" {
			items := v.([]interface{})
			if len(items) == 0 {
				return a
			}
			item := items[0].(map[string]interface{})
			for key,val := range item {
				if key == "name" {
					a.Name = val.(string)
				} 
				if key == "id" {
					a.ID = val.(string)
				}
				if key == "popularity" {
					q := val.(float64)
					a.Popularity = q
				}
				if key == "genres" {
					for _,g := range val.([]interface{}) {
						a.Genres[g.(string)]++
					}
				}
			}
			
		}
	}
	return a
}

// create an artist from their tracks that we can find in the DB
func CreateArtists(name string, id string) *Artists {
	return &Artists{
		Name: name,
		ID: id,
	}
}

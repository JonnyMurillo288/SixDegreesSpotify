package sixdegrees

import (
	"database/sql"
	"encoding/json"
	"log"

	_ "github.com/go-sql-driver/mysql"
)

type Track struct {
	Artist *Artists // Artist objects with the track
	Name string 
	PhotoURL string
	ID string // trackId
	Featured []*Artists // list of featured artist ID
}

type resu struct {
	Items interface{} `json:"items"`
}

func newTrack(art *Artists, name string, photo string, id string, feat []*Artists) Track {
	return Track{
		Artist: art,
		Name:name,
		PhotoURL:photo,
		ID:id,
		Featured: feat,
	}
}

// creates tracks struct from the ablums that are passed in 
// pulled in bytes from spotify.GetAlbumTracks()
// Returns list of tracks for the Artists and a helper function 
func (a *Artists) CreateTracks(r []byte, h *Helper) ([]Track, *Helper) {
	
	if h == nil {
		h = NewHelper()
	}
	var res *resu
	var ret []Track
	var feat []*Artists
	json.Unmarshal(r,&res)
	switch res.Items.(type) {
	case []interface{}:
		log.Printf("Creating tracks for %s",a.Name)
	default:
		log.Printf("Having to skip tracks for %s\n\nItems:  %s",a.Name,res.Items)
		return ret,h
	}
	var id,name string
	for _,Items := range res.Items.([]interface{}) { // items
		for k,v := range Items.(map[string]interface{}) { 
			var newName string
			if k == "artists" {
				for _,art := range v.([]interface{}) {
					for key,val := range art.(map[string]interface{}) {
						if key == "name" {
							newName = val.(string)
						}
					}
					if artist,ok := h.ArtistMap[newName]; ok {
						feat = append(feat,artist)
					} else {
						if artist := InputArtist(newName); artist != nil {
							feat = append(feat,artist)
							h.ArtistMap[newName] = artist
						} else {
							break
						}
					}
				}
			}
			if k == "id" {
				id = v.(string)
			}
			if k == "name" {
				name = v.(string)
			}
			if name != "" && id != "" {
				ret = append(ret,newTrack(a,name,"",id,feat))
				break
			}
		}
		name = ""
		id = ""
		feat = []*Artists{}
	}
	// }
	return ret,h
}


// return a list of ablum IDs for the artist
func (a *Artists) ParseAlbums(r []byte) []string {
	var res []string
	// var T Track
	var result *resu
	json.Unmarshal(r,&result)
	switch result.Items.(type) {
		case []interface{}:
			log.Printf("%s is good to parse albums",a.Name)
		default:
			log.Printf("Having to skip artist: %s\n\n%v", a.Name,result.Items)
			return res
	}
	skip := false
	for _, item := range result.Items.([]interface{}) {
		for k,v := range item.(map[string]interface{}) {
			if k == "artists" {
				for _,art := range v.([]interface{}) {
					for key,val := range art.(map[string]interface{}) {
						if key == "name" {
							if val.(string) == "Various Artists" {
								skip = true
								break
							}
						}
					}
				}
			}
			if k == "id" && !skip {
				res = append(res, v.(string))
			}
		}
		skip = false
	}
	return res
}



// check the number of tracks that we have for the artist
func (art *Artists) CheckTracks(db *sql.DB) int {
	var count int = 0
	rows, err := db.Query("SELECT COUNT(*) FROM Tracks")
	
	if err != nil {
		log.Fatal(err)
	}
	
	for rows.Next() {   
		if err := rows.Scan(&count); err != nil {
			log.Fatal(err)
		}
		} 
		rows.Close()
		log.Printf("\nThere are %v rows",count)
		return count
}



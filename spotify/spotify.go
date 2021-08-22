package spotify

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
)

type auth struct {
	AccessToken string `json:"access_token"`
	Type string `json:"token_type"`
	Refresh string `json:"refresh_token"`
	Expires string `json:"expiry"`
}

type playback struct {
	Progress float64 `json:"progress_ms"`
	Item interface{} `json:"item"`

}

// queue struct for songs with the info needed for playing
type queue struct {
	Progress,Duration float64
	TrackName, TrackPhoto, TrackID string
}


func getSpotify(endpoint string, header map[string]string) *http.Response {
	req, err := http.NewRequest("GET", endpoint,nil)
	for k,v := range header {
		req.Header.Set(k,v)
	}
	find := strings.Split(endpoint,"/")
	if err != nil {
		log.Println("error getting",find[len(find)-1],":",err)
	}
	resp,err := http.DefaultClient.Do(req)
	// fmt.Println("Status code for getting:",find,resp.StatusCode)
	if err != nil {
		log.Println("error getting",find[len(find)-1],":",err)
	}
	return resp
}

func postSpotify(endpoint string, header map[string]string, query map[string]string) {
	req, err := http.NewRequest("POST", endpoint, nil)
	// adds query values to the req
	for a,b := range query {
		newU,_ := url.Parse(req.URL.String())
		q := newU.Query()
		q.Set(a,b)
		newU.RawQuery = q.Encode()
		req.URL = newU
	}
	for k,v := range header {
		req.Header.Set(k,v)
	}
	if err != nil {
		log.Println(err)
	}
	resp, err := http.DefaultClient.Do(req)
	log.Println("The status code for posting is:",resp.StatusCode)
	if resp.StatusCode != 204 {
		log.Println("Response for posting:",resp)
	}
	if err != nil {
		log.Fatal("Error with Posting",err)
	}
}

func reqPlayback() (playback,[]byte) {
	ep := "https://api.spotify.com/v1/me/player/currently-playing?market=US"
	var q playback
	headers := getHeader()
	headers["Accept"] = "application/json"
	headers["Content-Type"] = "application/json"
	resp := getSpotify(ep,headers)
	body,err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	json.Unmarshal(body, &q)
	return q,body
}

// function for searching for an artist
// TODO: check if the input from user is acceptable
func SearchArtist(artist string) ([]byte, error) {
	header := getHeader()
	// fmt.Println(header)
	url := "https://api.spotify.com/v1/search"
	header["Accept"] = "application/json"
	header["Content-Type"] = "application/json"

	endpoint := url + "?q=" + strings.Join(strings.Split(artist," "),"%20") + "&type=artist"
	resp := getSpotify(endpoint,header)
	res, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return []byte(":("), err
	}
	defer resp.Body.Close()
	// fmt.Println(endpoint)
	return res,nil
}

// get the auth header with token
func getHeader() map[string]string {
	header := make(map[string]string)
	var token *auth

	f,erro := ioutil.ReadFile("./main/authToken.txt")
	if erro != nil {
		fmt.Println(erro)
	}
	err := json.Unmarshal(f,&token)
	if err != nil {
		log.Fatal("Error with the token header:",err.Error())
	}
	header["Authorization"] = "Bearer " + token.AccessToken
	return header
}



func ArtistAlbums(id string, limit int) ([]byte, error) {
	url := "https://api.spotify.com/v1/artists/" + id + "/albums?include_groups=album,single&limit=" + fmt.Sprint(limit)
	if limit == -1 {
		url = "https://api.spotify.com/v1/artists/" + id + "/albums?include_groups=album,single&limit=50"
	}
	header := getHeader()
	header["Accept"] = "application/json"
	header["Content-Type"] =  "application/json"

	res := getSpotify(url,header)
	r, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return []byte(":("), err
	}
	defer res.Body.Close()

	return r,nil

}

// PARAM: id - Album id 
func GetAlbumTracks(id string) ([]byte, error) {
	url := "https://api.spotify.com/v1/albums/" + id + "/tracks"
	header := getHeader()
	header["Accept"] = "application/json"
	header["Content-Type"] =  "application/json"

	res := getSpotify(url,header)
	r, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return []byte(":("), err
	}
	defer res.Body.Close()

	return r,nil
}







// ================================================================================================ //

func addQueue(tracks []string) {
	log.Println("adding to queue:",len(tracks))
	q := make(map[string]string)
	endpoint := "https://api.spotify.com/v1/me/player/queue"
	skip := "https://api.spotify.com/v1/me/player/next"
	uri := "spotify:track:"
	headers := getHeader()
	headers["Content-Type"] = "application/json"
	headers["Accept"] = "application/json"
	for _,track := range tracks {
		q["uri"] = uri + track
		postSpotify(endpoint, headers, q)
	}
	postSpotify(skip,headers,nil)
}

// function for controlling the playback for spotify
func controller(endpoint string) {
	log.Println("Envoking controller")
	q := make(map[string]string)
	headers := getHeader()
	headers["Content-Type"] = "application/json"
	headers["Accept"] = "application/json"
	postSpotify(endpoint,headers,q)
}


// get the information on the users playback
func getPlayback() queue {
	var ind = 0
	pb,_ := reqPlayback()
	var q queue
	// w.Write(b)
	q.Progress = pb.Progress
	itemMap := pb.Item.(map[string]interface{})
	for k,v := range itemMap {
		switch k {
		case "duration_ms":
			if dur,ok := v.(float64); ok {
				q.Duration = dur
			}
		case "name":
			if name,ok := v.(string); ok {
				q.TrackName = name
			}
		case "id":
			if i,ok := v.(string);ok {
				q.TrackID = i
			}
		}
		switch jsonObj := v.(type) {
		case string:
			continue
		case bool: 
			continue
		case []interface{}:
			continue
		case float64:
			continue
		case float32:
			continue
		case interface{}: // if the value type is an interface then do this
			for key,value := range jsonObj.(map[string]interface{}) { // remap the obj to keys and values
				switch key {
				case "images":
					for _,phVal := range value.([]interface{}) {
						ind++
						if ind == 2 {
							for ke,va := range phVal.(map[string]interface{}) {
								if ke == "url"{
									q.TrackPhoto = fmt.Sprintf("%s",va)
								}
							}
						}
					}					
				}
			}
		default:
			log.Fatal("Went to default ===")
		}
	}
	return q
}
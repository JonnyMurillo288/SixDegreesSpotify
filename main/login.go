package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"

	_ "github.com/go-sql-driver/mysql"
)

// save the users login info if they are logging in
func saveLogin(un string, pw string) {
	db, err := sql.Open("mysql", "jonny:Yankees162162@/Spotify")
	if err != nil {
		log.Fatalln("could not open login database:", err)
	}
	defer db.Close()
	q := `INSERT OR IGNORE INTO login VALUES (?,?)`
	create := `CREATE TABLE IF NOT EXISTS login (username STRING UNIQUE, password STRING);`
	_, err = db.Exec(q, un, pw)
	if err != nil {
		_, err = db.Exec(create)
		if err != nil {
			log.Fatalln("could not create login db:", err)
		}
		db.Close()
		saveLogin(un, pw)
	}

}

// get login info, if it doesnt exist then inform the user
func getLogin(un string, pw string) bool {
	var res *sql.Rows
	var username, password string
	db, err := sql.Open("mysql", "jonny:Yankees162162@/Spotify")
	if err != nil {
		log.Fatalln("could not open login database:", err)
	}
	defer db.Close()
	q := `SELECT username, password FROM login WHERE username=?;`
	res, err = db.Query(q, un)
	if err != nil {
		log.Println("Error getting login info:", err)
	}
	for res.Next() {
		if err := res.Scan(&username, &password); err != nil {
			log.Println("Error getting usernam and login", err.Error())
		}
		if username == un && password == pw {
			return true
		}
	}
	return false
}

// login function that will redirect to the auth package
func (c *client) login(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		templates.Templates["newClient"].Execute(w, nil)
	} else {
		r.ParseForm()
		fmt.Println("Client Name:", r.Form["username"])
		c.Name = r.Form["username"][0]
		password := r.Form["password"][0]
		if !getLogin(c.Name, password) {
			// send user JavaScript message saying incorrect password
			// ask if they would like to create an account with this login
			saveLogin(c.Name, password)
		}
	}
	http.Redirect(w, r, joinURL, http.StatusFound)
}

// checks users data in db
// "/gather" path -> "/recommend" path
func checkData(w http.ResponseWriter, r *http.Request) {
	var cmd *exec.Cmd
	w.Write([]byte("This is the gather page"))
	// templates.Templates["loading"].Execute(w,nil) // empty template telling user we are checking their data
	db, err := sql.Open("mysql", "jonny:Yankees162162@/Spotify")
	if err != nil {
		log.Fatalln("could not open login database:", err)
	}

	if checkTracks(db) <= 1000 {
		cmd = exec.Command("python3", "SpotifyAuth.py", "gather")
		runCMD(cmd)
	}

	if !checkRecommendation(db) {
		cmd = exec.Command("python3", "SpotifyAuth.py", "rec")
		runCMD(cmd)
	}

	cmd = exec.Command("python3", "MachineLearning.py")
	runCMD(cmd)
	db.Close()

	newU := r.URL
	newU.Path = "/recommend"
	http.Redirect(w, r, newU.String(), http.StatusSeeOther)
	// redirects user to the main page where we will open up a socket
}

// checks how many tracks are in recommended tracks db
func checkRecommendation(db *sql.DB) bool {
	var count int
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS recommendedTracks (id CHAR, track_id TEXT, UNIQUE(track_id))"); err != nil {
		log.Fatalf("Error creating recommendedTracks: %s", err)
	}
	rows, err := db.Query("SELECT COUNT(*) FROM recommendedTracks")

	if err != nil {
		log.Fatal(err)
	}
	for rows.Next() {
		if err := rows.Scan(&count); err != nil {
			log.Fatal(err)
		}
	}
	rows.Close()
	if count > 20 {
		return true
	} else {
		fmt.Printf("\nThere are %v tracks in recommendeTracks table\n", count)
		return false
	}
}

// checks how many tracks are in the database
func checkTracks(db *sql.DB) int {
	var count int = 0
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS userTracks (id CHAR, track_name TEXT, track_id TEXT UNIQUE, added_at INT, image_url TEXT, top_track INT, playlist_track INTEGER)")
	if err != nil {
		log.Fatalf("Error creating userTracks: %s", err)
	}
	rows, err := db.Query("SELECT COUNT(*) FROM userTracks")

	if err != nil {
		log.Fatal(err)
	}

	for rows.Next() {
		if err := rows.Scan(&count); err != nil {
			log.Fatal(err)
		}
	}
	rows.Close()
	log.Printf("\nThere are %v rows", count)
	return count
}

// runs exec.Cmd commands
func runCMD(e *exec.Cmd) {
	e.Stdout = os.Stdout
	e.Stderr = os.Stderr
	log.Println(e.Run())
}

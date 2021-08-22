package main

import (
	"net/http"

	"github.com/gorilla/mux"
)

func main() {
	mx := mux.NewRouter() //.SkipClean(true)
	mx.HandleFunc("/", HomePage)
	mx.HandleFunc("/auth", Authorize)

	http.ListenAndServe(":8392", mx)
}
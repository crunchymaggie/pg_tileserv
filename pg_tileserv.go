package main

import (
	// "bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	// "os"
	"strconv"
	"time"
)

// type Coordinate struct {
// 	x, y float64
// }

type Config struct {
	ConnStr            string `json:"connstr"`
	Host               string `json:"host"`
	Port               int    `json:"port"`
	Addr               string `json:"addr"`
	Program            string `json:"program"`
	Version            string `json:"version"`
	DefaultResolution  int    `json:"default_resolution"`
	DefaultBuffer      int    `json:"default_buffer"`
	MaxFeaturesPerTile int    `json:"max_features_per_tile"`
	Attribution        string `json:"attribution"`
}

// A global variable for configuration parameters and defaults
var globalConfig Config = Config{
	ConnStr:            "dbname=pramsey sslmode=disable",
	Host:               "localhost",
	Port:               7800,
	Addr:               "http://localhost:7800",
	Program:            "pg_tileserv",
	Version:            "0.1",
	DefaultBuffer:      256,
	DefaultResolution:  4094,
	MaxFeaturesPerTile: 50000,
}

// A global array of Layer where the state is held for performance
// Refreshed when GetLayerTableList is called
var globalLayers map[string]Layer

// A global database connection pointer
var globalDb *sql.DB = nil

// type LayerFunction struct {
// 	namespace string
// 	funcname string
// }

func DbConnect() (*sql.DB, error) {
	if globalDb == nil {
		var err error
		globalDb, err = sql.Open("postgres", globalConfig.ConnStr)
		if err != nil {
			log.Fatal(err)
		}
		return globalDb, err
	}
	err := globalDb.Ping()
	if err != nil {
		return nil, err
	}
	return globalDb, nil
}

func AssetFileAsString(assetPath string) (asset string) {
	b, err := ioutil.ReadFile(assetPath)
	if err != nil {
		log.Fatal(err)
	}
	return string(b)
}

func HandleRequestRoot(w http.ResponseWriter, r *http.Request) {
	log.Println("HandleRequestRoot")
	// html := AssetFileAsString("assets/index.html")
	// fmt.Fprintf(w, html)
	GetLayerTableList()

	t, err := template.ParseFiles("assets/index.html")
	if err != nil {
		log.Println(err)
	}
	t.Execute(w, globalLayers)
}

func HandleRequestIndex(w http.ResponseWriter, r *http.Request) {
	log.Println("HandleRequestIndex")
	// Update the local copy
	GetLayerTableList()
	w.Header().Add("Content-Type", "application/json")
	json.NewEncoder(w).Encode(globalLayers)
}

func HandleRequestLayer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	lyrname := vars["name"]
	log.Printf("HandleRequestLayer: %s", lyrname)

	if lyr, ok := globalLayers[lyrname]; ok {
		err := lyr.AddDetails()
		if err != nil {
			log.Fatal(err)
		}
		w.Header().Add("Content-Type", "application/json")
		json.NewEncoder(w).Encode(lyr)
	}
}

func HandleRequestLayerPreview(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	lyrname := vars["name"]
	log.Printf("HandleRequestLayerPreview: %s", lyrname)

	if lyr, ok := globalLayers[lyrname]; ok {
		t, err := template.ParseFiles("assets/preview.html")
		if err != nil {
			log.Println(err)
		}
		t.Execute(w, lyr)
	}
}

func HandleRequestTile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	lyrname := vars["name"]
	if lyr, ok := globalLayers[lyrname]; ok {
		x, _ := strconv.Atoi(vars["x"])
		y, _ := strconv.Atoi(vars["y"])
		zoom, _ := strconv.Atoi(vars["zoom"])
		ext := vars["ext"]
		log.Printf("HandleRequestTile: %d/%d/%d.%s", zoom, x, y, ext)
		tile := Tile{Zoom: zoom, X: x, Y: y, Ext: ext}
		if !tile.IsValid() {
			log.Fatal("HandleRequestTile: invalid map tile")
		}
		// Replace with SQL fun
		pbf, err := lyr.GetTile(&tile)
		if err != nil {
			// TODO return a 500 or something
		}
		w.Header().Add("Content-Type", "application/vnd.mapbox-vector-tile")
		_, err = w.Write(pbf)
		return
	}

}

// func trace() (string, int, string) {
//     pc, file, line, ok := runtime.Caller(1)
//     if !ok { return "?", 0, "?" }

//     fn := runtime.FuncForPC(pc)
//     return file, line, fn.Name()
// }

func HandleRequests() {

	// creates a new instance of a mux router
	myRouter := mux.NewRouter().StrictSlash(true)
	// replace http.HandleFunc with myRouter.HandleFunc
	myRouter.HandleFunc("/", HandleRequestRoot)
	myRouter.HandleFunc("/index.html", HandleRequestRoot)
	myRouter.HandleFunc("/index.json", HandleRequestIndex)
	myRouter.HandleFunc("/{name}.json", HandleRequestLayer)
	myRouter.HandleFunc("/{name}.html", HandleRequestLayerPreview)
	myRouter.HandleFunc("/{name}/{zoom:[0-9]+}/{x:[0-9]+}/{y:[0-9]+}.{ext}", HandleRequestTile)

	// more "production friendly" timeouts
	// https://blog.simon-frey.eu/go-as-in-golang-standard-net-http-config-will-break-your-production/#You_should_at_least_do_this_The_easy_path
	s := &http.Server{
		ReadTimeout:  1 * time.Second,
		WriteTimeout: 10 * time.Second,
		Addr:         fmt.Sprintf("%s:%d", globalConfig.Host, globalConfig.Port),
		Handler:      myRouter,
	}

	// TODO figure out how to gracefully shut down on ^C
	// and shut down all the database connections / statements
	log.Fatal(s.ListenAndServe())
}

/******************************************************************************/

func main() {

	log.Printf("%s: %s\n", globalConfig.Program, globalConfig.Version)
	log.Printf("Listening on: %s", globalConfig.Addr)

	// Load the layer list right away
	GetLayerTableList()

	HandleRequests()
}
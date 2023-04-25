package main

import (
	"flag"
	"net/http"
)

// Author: Ceyhun Uzunoglu <ceyhunuzngl [AT] gmail {DOT} com>
// Reverse proxy to expose internal service response

// Example:
//   Serve: go run http_pingpong.go -port ":17000" -endpoint "/metrics" -uri "http://localhost:8270/crabserver/metrics"
//   Get:   curl http://IP:17000/metrics

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
)

var (
	listeningPort   = flag.String("port", ":17000", "port to expose metrics and web interface.")
	metricsEndpoint = flag.String("endpoint", "/metrics", "Path under which to expose metrics.")
	scrapeURI       = flag.String("uri", "", "URI of server status page we're going to scrape")
	verbose         = flag.Bool("verbose", false, "verbose output")
)

func getResult(w http.ResponseWriter, _ *http.Request) {
	resp, err := http.Get(*scrapeURI)
	if *verbose {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	} else {
		log.SetFlags(log.LstdFlags)
	}
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, err = io.WriteString(w, string(body))
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}
}

func main() {
	flag.Parse()
	if *verbose {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	} else {
		log.SetFlags(log.LstdFlags)
	}
	http.HandleFunc(*metricsEndpoint, getResult)
	err := http.ListenAndServe(*listeningPort, nil)
	if errors.Is(err, http.ErrServerClosed) {
		fmt.Printf("server closed\n")
	} else if err != nil {
		fmt.Printf("error starting server: %s\n", err)
		os.Exit(1)
	}
}

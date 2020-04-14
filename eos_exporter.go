package main

// Author: Valentin Kuznetsov <vkuznet [AT] gmail {DOT} com>
// Example of cmsweb data-service exporter for prometheus.io

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	listeningAddress = flag.String("port", ":18000", "port to expose metrics and web interface.")
	metricsEndpoint  = flag.String("endpoint", "/metrics", "Path under which to expose metrics.")
	scrapeURI        = flag.String("uri", "", "URI of server status page we're going to scrape")
	proxyfile        = flag.String("proxyfile", "", "proxy file name")
	eosPath          = flag.String("eosPath", "", "EOS path to check")
	namespace        = flag.String("namespace", "eos", "EOS namespace name")
	verbose          = flag.Bool("verbose", false, "verbose output")
)

type Exporter struct {
	URI            string
	mutex          sync.Mutex
	scrapeFailures prometheus.Counter
	status         *prometheus.Desc
}

const (
	OkEOS = iota
	NoAccessToEOS
	FailedToWriteTempfile
	FailedToCloseTempfile
)

// helper function to check eos path and return error code
func eosAccess(path string) int {
	_, err := os.Stat(path)
	if err != nil {
		if *verbose {
			log.Println(err)
		}
		return NoAccessToEOS
	}

	// create temp file in our path
	tmpFile, err := ioutil.TempFile(path, "tmp-")
	defer os.Remove(tmpFile.Name())

	// Example writing to the file
	text := []byte("This is a test")
	if _, err = tmpFile.Write(text); err != nil {
		if *verbose {
			log.Println("Failed to write to temporary file", err)
		}
		return FailedToWriteTempfile
	}

	// Close the file
	if err := tmpFile.Close(); err != nil {
		if *verbose {
			log.Println(err)
		}
		return FailedToCloseTempfile
	}
	return OkEOS
}

func NewExporter(uri string) *Exporter {
	return &Exporter{
		URI:            uri,
		scrapeFailures: prometheus.NewCounter(prometheus.CounterOpts{}),
		status: prometheus.NewDesc(
			prometheus.BuildFQName(*namespace, "", "status"),
			fmt.Sprintf("Current status of %s", *scrapeURI),
			nil,
			nil),
	}
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.status
}

// Collect performs metrics collectio of exporter attributes
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.mutex.Lock() // To protect metrics from concurrent collects.
	defer e.mutex.Unlock()
	if err := e.collect(ch); err != nil {
		log.Printf("Error scraping: %s", err)
		e.scrapeFailures.Inc()
		e.scrapeFailures.Collect(ch)
	}
	return
}

// helper function which collects exporter attributes
func (e *Exporter) collect(ch chan<- prometheus.Metric) error {
	// here is an example how we may collect server stats
	// we'll make an HTTP call to server URI which will return
	// status as a JSON document, then we'll assign metrics using JSON values

	// query EOS path
	ecode := eosAccess(*eosPath)

	ch <- prometheus.MustNewConstMetric(e.status, prometheus.CounterValue, float64(ecode))
	return nil
}

// main function
func main() {
	flag.Parse()

	// log time, filename, and line number
	if *verbose {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	} else {
		log.SetFlags(log.LstdFlags)
	}

	exporter := NewExporter(*scrapeURI)
	prometheus.MustRegister(exporter)

	log.Printf("Starting Server: %s", *listeningAddress)
	http.Handle(*metricsEndpoint, promhttp.Handler())
	log.Fatal(http.ListenAndServe(*listeningAddress, nil))
}

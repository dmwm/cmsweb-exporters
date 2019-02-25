package main

// Author: Valentin Kuznetsov <vkuznet [AT] gmail {DOT} com>
// Example of cmsweb data-service exporter for prometheus.io

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
)

var (
	listeningAddress = flag.String("port", ":17000", "port to expose metrics and web interface.")
	metricsEndpoint  = flag.String("endpoint", "/metrics", "Path under which to expose metrics.")
	scrapeURI        = flag.String("uri", "", "URI of server status page we're going to scrape")
	pid              = flag.Int("pid", 0, "PID of the process to scrape")
	prefix           = flag.String("prefix", "process_exporter", "Assign prefix to prometheus reports to monitor this PID")
)

// main function
func main() {
	flag.Parse()
	if *pid == 0 {
		fmt.Println("Please provide valid PID")
		os.Exit(1)
	}
	opts := prometheus.ProcessCollectorOpts{
		PidFn:        func() (int, error) { return *pid, nil },
		Namespace:    *prefix,
		ReportErrors: true,
	}
	exporter := prometheus.NewProcessCollector(opts)
	prometheus.MustRegister(exporter)

	log.Infof("Starting Server: %s", *listeningAddress)
	http.Handle(*metricsEndpoint, prometheus.Handler())
	log.Fatal(http.ListenAndServe(*listeningAddress, nil))
}

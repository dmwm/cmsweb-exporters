package main

// Author: Valentin Kuznetsov <vkuznet [AT] gmail {DOT} com>
// CherryPy server metrics based cpstats: exporter for prometheus.io

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	namespace = "cpy" // For Prometheus metrics.
)

var (
	listeningAddress = flag.String("address", ":19000", "address to expose metrics on web interface.")
	metricsEndpoint  = flag.String("endpoint", "/metrics", "Path under which to expose metrics.")
	scrapeURI        = flag.String("uri", "", "URI of server status page we're going to scrape")
	verbose          = flag.Bool("verbose", false, "verbose output")
)

type Exporter struct {
	URI   string
	mutex sync.Mutex

	accepts                 *prometheus.Desc
	acceptInSec             *prometheus.Desc
	bytesRead               *prometheus.Desc
	readThroughput          *prometheus.Desc
	writeThroughput         *prometheus.Desc
	socketErrors            *prometheus.Desc
	threads                 *prometheus.Desc
	threadsIdle             *prometheus.Desc
	requests                *prometheus.Desc
	uptime                  *prometheus.Desc
	queue                   *prometheus.Desc
	thrBytesRead            *prometheus.Desc
	thrBytesWrite           *prometheus.Desc
	thrReadThroughput       *prometheus.Desc
	thrRequests             *prometheus.Desc
	thrWorkTime             *prometheus.Desc
	thrWriteThroughput      *prometheus.Desc
	threadsInfo             *prometheus.Desc
	cpyBytesReadPerRequest  *prometheus.Desc
	cpyBytesReadPerSecond   *prometheus.Desc
	cpyBytesWritePerRequest *prometheus.Desc
	cpyBytesWritePerSecond  *prometheus.Desc
	cpyCurrentRequest       *prometheus.Desc
	cpyCurrentTime          *prometheus.Desc
	cpyRequestsPerSecond    *prometheus.Desc
	cpyTotalBytesRead       *prometheus.Desc
	cpyTotalBytesWrite      *prometheus.Desc
	cpyTotalRequests        *prometheus.Desc
	cpyTotalTime            *prometheus.Desc
	cpyBytesRead            *prometheus.Desc
	cpyBytesWrite           *prometheus.Desc
	cpyProcTime             *prometheus.Desc
}

func NewExporter(uri string) *Exporter {
	var labels = []string{"thread"}
	var appLabels = []string{"app"}
	return &Exporter{
		URI: uri,
		accepts: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "accepts"),
			"total number of accepts", nil, nil),
		acceptInSec: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "acceptInSec"),
			"total number of acceptInSec", nil, nil),
		bytesRead: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "bytesRead"),
			"total number of bytesRead", nil, nil),
		readThroughput: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "readThroughput"),
			"total number of readThroughput", nil, nil),
		writeThroughput: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "writeThroughput"),
			"total number of readThroughput", nil, nil),
		socketErrors: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "socketErrors"),
			"total number of socketErrors", nil, nil),
		threads: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "threads"),
			"total number of threads", nil, nil),
		threadsIdle: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "threadsIdle"),
			"total number of threadsIdle", nil, nil),
		requests: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "requests"),
			"total number of requests", nil, nil),
		uptime: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "uptime"),
			"Current uptime in seconds", nil, nil),
		queue: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "queue"),
			"Current queue value", nil, nil),

		thrBytesRead: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "thrBytesRead"),
			"Current thrBytesRead value", labels, nil),
		thrBytesWrite: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "thrBytesWrite"),
			"Current thrBytesWrite value", labels, nil),
		thrReadThroughput: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "thrReadThroughput"),
			"Current thrReadThroughput value", labels, nil),
		thrRequests: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "thrRequests"),
			"Current thrRequests value", labels, nil),
		thrWorkTime: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "thrWorkTime"),
			"Current thrWorkTime value", labels, nil),
		thrWriteThroughput: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "thrWriteThroughput"),
			"Current thrWriteThroughput value", labels, nil),

		cpyBytesReadPerRequest: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "cpyBytesReadPerRequest"),
			"Current cpyBytesReadPerRequest value", nil, nil),
		cpyBytesReadPerSecond: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "cpyBytesReadPerSecond"),
			"Current cpyBytesReadPerSecond value", nil, nil),
		cpyBytesWritePerRequest: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "cpyBytesWritePerRequest"),
			"Current cpyBytesWritePerRequest value", nil, nil),
		cpyBytesWritePerSecond: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "cpyBytesWritePerSecond"),
			"Current cpyBytesWritePerSecond value", nil, nil),
		cpyCurrentRequest: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "cpyCurrentRequest"),
			"Current cpyCurrentRequest value", nil, nil),
		cpyCurrentTime: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "cpyCurrentTime"),
			"Current cpyCurrentTime value", nil, nil),
		cpyRequestsPerSecond: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "cpyRequestsPerSecond"),
			"Current cpyRequestsPerSecond value", nil, nil),
		cpyTotalBytesRead: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "cpyTotalBytesRead"),
			"Current cpyTotalBytesRead value", nil, nil),
		cpyTotalBytesWrite: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "cpyTotalBytesWrite"),
			"Current cpyTotalBytesWrite value", nil, nil),
		cpyTotalRequests: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "cpyTotalRequests"),
			"Current cpyTotalRequests value", nil, nil),
		cpyTotalTime: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "cpyTotalTime"),
			"Current cpyTotalTime value", nil, nil),
		cpyBytesRead: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "cpyBytesRead"),
			"Current cpyBytesRead value", appLabels, nil),
		cpyBytesWrite: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "cpyBytesWrite"),
			"Current cpyBytesWrite value", appLabels, nil),
		cpyProcTime: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "cpyProcTime"),
			"Current cpyProcTime value", appLabels, nil),
	}
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.accepts
	ch <- e.acceptInSec
	ch <- e.bytesRead
	ch <- e.readThroughput
	ch <- e.writeThroughput
	ch <- e.socketErrors
	ch <- e.threads
	ch <- e.threadsIdle
	ch <- e.requests
	ch <- e.uptime
	ch <- e.queue
	ch <- e.thrBytesRead
	ch <- e.thrBytesWrite
	ch <- e.thrReadThroughput
	ch <- e.thrRequests
	ch <- e.thrWorkTime
	ch <- e.thrWriteThroughput
	ch <- e.cpyBytesReadPerRequest
	ch <- e.cpyBytesReadPerSecond
	ch <- e.cpyBytesWritePerRequest
	ch <- e.cpyBytesWritePerSecond
	ch <- e.cpyCurrentRequest
	ch <- e.cpyCurrentTime
	ch <- e.cpyRequestsPerSecond
	ch <- e.cpyTotalBytesRead
	ch <- e.cpyTotalBytesWrite
	ch <- e.cpyTotalRequests
	ch <- e.cpyTotalTime
	ch <- e.cpyBytesRead
	ch <- e.cpyBytesWrite
	ch <- e.cpyProcTime
}

// Collect performs metrics collectio of exporter attributes
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.mutex.Lock() // To protect metrics from concurrent collects.
	defer e.mutex.Unlock()
	if err := e.collect(ch); err != nil {
		log.Fatalf("Error scraping: %s", err)
	}
	return
}

// helper function which collects exporter attributes
func (e *Exporter) collect(ch chan<- prometheus.Metric) error {
	// here is an example how we may collect server stats
	// we'll make an HTTP call to server URI which will return
	// status as a JSON document, then we'll assign metrics using JSON values
	var req *http.Request
	client := &http.Client{}
	req, _ = http.NewRequest("GET", e.URI, nil)
	req.Header.Add("Accept-Encoding", "identity")
	req.Header.Add("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Error scraping apache: %v", err)
	}

	data, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		if err != nil {
			data = []byte(err.Error())
		}
		return fmt.Errorf("Status %s (%d): %s", resp.Status, resp.StatusCode, data)
	}
	var stats map[string]interface{}
	err = json.Unmarshal(data, &stats)
	if err != nil {
		return fmt.Errorf("Fail to unmarshal JSON data %s", err.Error())
	}
	var srv map[string]interface{}
	for k, v := range stats {
		if strings.Contains(k, "Server") {
			srv = v.(map[string]interface{})
			if *verbose {
				fmt.Println("CherryPy server", v)
			}
			ch <- prometheus.MustNewConstMetric(e.accepts, prometheus.CounterValue, convert(srv, "Accepts"))
			ch <- prometheus.MustNewConstMetric(e.acceptInSec, prometheus.CounterValue, convert(srv, "Accepts/sec"))
			ch <- prometheus.MustNewConstMetric(e.bytesRead, prometheus.CounterValue, convert(srv, "Bytes Read"))
			ch <- prometheus.MustNewConstMetric(e.readThroughput, prometheus.CounterValue, convert(srv, "Read Throughput"))
			ch <- prometheus.MustNewConstMetric(e.writeThroughput, prometheus.CounterValue, convert(srv, "Write Throughput"))
			ch <- prometheus.MustNewConstMetric(e.socketErrors, prometheus.CounterValue, convert(srv, "Socket Errors"))
			ch <- prometheus.MustNewConstMetric(e.threads, prometheus.CounterValue, convert(srv, "Threads"))
			ch <- prometheus.MustNewConstMetric(e.threadsIdle, prometheus.CounterValue, convert(srv, "Threads Idle"))
			ch <- prometheus.MustNewConstMetric(e.requests, prometheus.CounterValue, convert(srv, "Requests"))
			ch <- prometheus.MustNewConstMetric(e.queue, prometheus.CounterValue, convert(srv, "Queue"))
			if d, ok := srv["Worker Threads"]; ok {
				var tdata map[string]interface{}
				tdata = d.(map[string]interface{})
				for k, v := range tdata {
					var d map[string]interface{}
					d = v.(map[string]interface{})
					labels := []string{k}
					ch <- prometheus.MustNewConstMetric(e.thrBytesRead, prometheus.CounterValue, convert(d, "Bytes Read"), labels...)
					ch <- prometheus.MustNewConstMetric(e.thrBytesWrite, prometheus.CounterValue, convert(d, "Bytes Written"), labels...)
					ch <- prometheus.MustNewConstMetric(e.thrReadThroughput, prometheus.CounterValue, convert(d, "Read Throughput"), labels...)
					ch <- prometheus.MustNewConstMetric(e.thrRequests, prometheus.CounterValue, convert(d, "Requests"), labels...)
					ch <- prometheus.MustNewConstMetric(e.thrWorkTime, prometheus.CounterValue, convert(d, "Work Time"), labels...)
					ch <- prometheus.MustNewConstMetric(e.thrWriteThroughput, prometheus.CounterValue, convert(d, "Write Throughput"), labels...)
				}
			}
		} else if strings.Contains(k, "Application") {
			srv = v.(map[string]interface{})
			if *verbose {
				fmt.Println("CherryPy Application", srv)
			}
			ch <- prometheus.MustNewConstMetric(e.cpyBytesReadPerRequest, prometheus.CounterValue, convert(srv, "Bytes Read/Request"))
			ch <- prometheus.MustNewConstMetric(e.cpyBytesReadPerSecond, prometheus.CounterValue, convert(srv, "Bytes Read/Second"))
			ch <- prometheus.MustNewConstMetric(e.cpyBytesWritePerRequest, prometheus.CounterValue, convert(srv, "Bytes Written/Request"))
			ch <- prometheus.MustNewConstMetric(e.cpyBytesWritePerSecond, prometheus.CounterValue, convert(srv, "Bytes Written/Second"))
			ch <- prometheus.MustNewConstMetric(e.cpyCurrentRequest, prometheus.CounterValue, convert(srv, "Current Requests"))
			ch <- prometheus.MustNewConstMetric(e.cpyCurrentTime, prometheus.CounterValue, convert(srv, "Current Time"))
			ch <- prometheus.MustNewConstMetric(e.cpyRequestsPerSecond, prometheus.CounterValue, convert(srv, "Requests/Second"))
			ch <- prometheus.MustNewConstMetric(e.cpyTotalBytesRead, prometheus.CounterValue, convert(srv, "Total Bytes Read"))
			ch <- prometheus.MustNewConstMetric(e.cpyTotalBytesWrite, prometheus.CounterValue, convert(srv, "Total Bytes Written"))
			ch <- prometheus.MustNewConstMetric(e.cpyTotalRequests, prometheus.CounterValue, convert(srv, "Total Requests"))
			ch <- prometheus.MustNewConstMetric(e.cpyTotalTime, prometheus.CounterValue, convert(srv, "Total Time"))
			ch <- prometheus.MustNewConstMetric(e.uptime, prometheus.CounterValue, convert(srv, "Uptime"))
			if d, ok := srv["Requests"]; ok {
				var tdata map[string]interface{}
				tdata = d.(map[string]interface{})
				for k, v := range tdata {
					var d map[string]interface{}
					d = v.(map[string]interface{})
					labels := []string{k}
					ch <- prometheus.MustNewConstMetric(e.cpyBytesRead, prometheus.CounterValue, convert(d, "Bytes Read"), labels...)
					ch <- prometheus.MustNewConstMetric(e.cpyBytesWrite, prometheus.CounterValue, convert(d, "Bytes Written"), labels...)
					ch <- prometheus.MustNewConstMetric(e.cpyProcTime, prometheus.CounterValue, convert(d, "Processing Time"), labels...)
				}
			}
		}
	}
	return nil
}

func convert(srv map[string]interface{}, key string) float64 {
	r := srv[key]
	switch v := r.(type) {
	case float64:
		return v
	case int, int32, int64:
		return float64(64)
	default:
		if *verbose {
			fmt.Println("### unable to cast %v %v %v", key, r, v)
		}
		return 0
	}
}

// main function
func main() {
	flag.Parse()
	exporter := NewExporter(*scrapeURI)
	prometheus.MustRegister(exporter)

	log.Printf("Starting Server: %s", *listeningAddress)
	http.Handle(*metricsEndpoint, promhttp.Handler())
	log.Fatal(http.ListenAndServe(*listeningAddress, nil))
}

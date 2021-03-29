package main

// Author: Valentin Kuznetsov <vkuznet [AT] gmail {DOT} com>
// Example of cmsweb data-service exporter for prometheus.io

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/user"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/log"
	"github.com/vkuznet/x509proxy"
)

const (
	namespace = "das2go" // For Prometheus metrics.
)

var (
	listeningAddress = flag.String("address", ":18217", "address to expose metrics on web interface.")
	metricsEndpoint  = flag.String("endpoint", "/metrics", "Path under which to expose metrics.")
	scrapeURI        = flag.String("uri", "http://localhost:8217/das/status", "URI of server status page we're going to scrape")
	verbose          = flag.Bool("verbose", false, "verbose output")
)

// global HTTP client
var _client = HttpClient()

// global client's x509 certificates
var _certs []tls.Certificate

// UserDN function parses user Distinguished Name (DN) from client's HTTP request
func UserDN(r *http.Request) string {
	var names []interface{}
	for _, cert := range r.TLS.PeerCertificates {
		for _, name := range cert.Subject.Names {
			switch v := name.Value.(type) {
			case string:
				names = append(names, v)
			}
		}
	}
	parts := names[:7]
	return fmt.Sprintf("/DC=%s/DC=%s/OU=%s/OU=%s/CN=%s/CN=%s/CN=%s", parts...)
}

// client X509 certificates
func tlsCerts() ([]tls.Certificate, error) {
	if len(_certs) != 0 {
		return _certs, nil // use cached certs
	}
	uproxy := os.Getenv("X509_USER_PROXY")
	uckey := os.Getenv("X509_USER_KEY")
	ucert := os.Getenv("X509_USER_CERT")

	// check if /tmp/x509up_u$UID exists, if so setup X509_USER_PROXY env
	u, err := user.Current()
	if err == nil {
		fname := fmt.Sprintf("/tmp/x509up_u%s", u.Uid)
		if _, err := os.Stat(fname); err == nil {
			uproxy = fname
		}
	}
	if *verbose {
		log.Infof(uproxy, uckey, ucert)
	}

	if uproxy == "" && uckey == "" { // user doesn't have neither proxy or user certs
		return nil, fmt.Errorf("Neither proxy or user certs are found, please setup X509 environment variables")
	}
	if uproxy != "" {
		// use local implementation of LoadX409KeyPair instead of tls one
		x509cert, err := x509proxy.LoadX509Proxy(uproxy)
		if err != nil {
			return nil, fmt.Errorf("failed to parse proxy X509 proxy set by X509_USER_PROXY: %v", err)
		}
		_certs = []tls.Certificate{x509cert}
		return _certs, nil
	}
	x509cert, err := tls.LoadX509KeyPair(ucert, uckey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse user X509 certificate: %v", err)
	}
	_certs = []tls.Certificate{x509cert}
	return _certs, nil
}

// HttpClient provides HTTP client
func HttpClient() *http.Client {
	// get X509 certs
	certs, err := tlsCerts()
	if err != nil {
		fmt.Println("unable to get TLS certificate: ", err.Error())
		return &http.Client{}
	}
	if len(certs) == 0 {
		return &http.Client{}
	}
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{Certificates: certs,
			InsecureSkipVerify: true},
	}
	return &http.Client{Transport: tr}
}

type Exporter struct {
	URI   string
	mutex sync.Mutex

	scrapeFailures     prometheus.Counter
	getCalls           *prometheus.Desc
	postCalls          *prometheus.Desc
	getRequests        *prometheus.Desc
	postRequests       *prometheus.Desc
	uptime             *prometheus.Desc
	memPercent         *prometheus.Desc
	memTotal           *prometheus.Desc
	memFree            *prometheus.Desc
	memUsed            *prometheus.Desc
	memStatsSys        *prometheus.Desc
	memStatsAlloc      *prometheus.Desc
	memStatsTotalAlloc *prometheus.Desc
	memStatsHeapSys    *prometheus.Desc
	memStatsStackSys   *prometheus.Desc
	swapPercent        *prometheus.Desc
	cpuPercent         *prometheus.Desc
	coresPercent       *prometheus.Desc
	numThreads         *prometheus.Desc
	numGoroutines      *prometheus.Desc
	numQueries         *prometheus.Desc
	load1              *prometheus.Desc
	load5              *prometheus.Desc
	load15             *prometheus.Desc
	openFiles          *prometheus.Desc
	totCon             *prometheus.Desc
	lisCon             *prometheus.Desc
	estCon             *prometheus.Desc
}

func NewExporter(uri string) *Exporter {
	var labels = []string{"cores"}
	return &Exporter{
		URI: uri,
		getCalls: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "get_calls"),
			"Current total number of GET HTTP calls server",
			nil,
			nil),
		postCalls: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "post_calls"),
			"Current total number of POST HTTP calls to server",
			nil,
			nil),
		getRequests: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "get_requests"),
			"Current total number of GET HTTP calls server",
			nil,
			nil),
		postRequests: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "post_requests"),
			"Current total number of POST HTTP calls to server",
			nil,
			nil),
		uptime: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "uptime"),
			"Current uptime in seconds",
			nil,
			nil),
		memPercent: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "memory_percent"),
			"Virtual memory usage of the server",
			nil,
			nil),
		memTotal: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "memory_total"),
			"Virtual total memory usage of the server",
			nil,
			nil),
		memFree: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "memory_free"),
			"Virtual free memory usage of the server",
			nil,
			nil),
		memUsed: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "memory_used"),
			"Virtual used memory usage of the server",
			nil,
			nil),
		memStatsSys: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "memstats_sys"),
			"total bytes of memory obtained from the OS", nil, nil),
		memStatsAlloc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "memstats_alloc"),
			"bytes of allocated heap objects", nil, nil),
		memStatsTotalAlloc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "memstats_tot_alloc"),
			"cumulative bytes allocated for heap objects", nil, nil),
		memStatsHeapSys: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "memstats_heap_sys"),
			"bytes of heap memory obtained from the OS", nil, nil),
		memStatsStackSys: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "memstats_stack_sys"),
			"bytes of stack memory obtained from the OS", nil, nil),
		swapPercent: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "swap_percent"),
			"Swap memory usage of the server",
			nil,
			nil),
		cpuPercent: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "cpu_percent"),
			"cpu percent of the server",
			nil,
			nil),
		coresPercent: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "cores_percent"),
			"cpu cores percentage on the server", labels, nil),
		numThreads: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "num_threads"),
			"Number of threads",
			nil,
			nil),
		numGoroutines: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "num_go_routines"),
			"Number of Go routines",
			nil,
			nil),
		load1: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "load1"),
			"Load average in last 1m",
			nil,
			nil),
		load5: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "load5"),
			"Load average in last 5m",
			nil,
			nil),
		load15: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "load15"),
			"Load average in last 15m",
			nil,
			nil),
		openFiles: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "open_files"),
			"Number of open files",
			nil,
			nil),
		totCon: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "total_connections"),
			"Server TOTAL number of connections",
			nil,
			nil),
		lisCon: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "listen_connections"),
			"Server LISTEN number of connections",
			nil,
			nil),
		estCon: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "established_connections"),
			"Server ESTABLISHED number of connections",
			nil,
			nil),
	}
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.getCalls
	ch <- e.postCalls
	ch <- e.getRequests
	ch <- e.postRequests
	ch <- e.uptime
	ch <- e.memPercent
	ch <- e.memTotal
	ch <- e.memFree
	ch <- e.memUsed
	ch <- e.memStatsSys
	ch <- e.memStatsAlloc
	ch <- e.memStatsTotalAlloc
	ch <- e.memStatsHeapSys
	ch <- e.memStatsStackSys
	ch <- e.swapPercent
	ch <- e.cpuPercent
	ch <- e.coresPercent
	ch <- e.numThreads
	ch <- e.numGoroutines
	ch <- e.load1
	ch <- e.load5
	ch <- e.load15
	ch <- e.openFiles
	ch <- e.totCon
	ch <- e.lisCon
	ch <- e.estCon
}

// Collect performs metrics collectio of exporter attributes
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.mutex.Lock() // To protect metrics from concurrent collects.
	defer e.mutex.Unlock()
	if err := e.collect(ch); err != nil {
		log.Errorf("Error scraping: %s", err)
		//e.scrapeFailures.Inc()
		//e.scrapeFailures.Collect(ch)
	}
	return
}

// helper function which collects exporter attributes
func (e *Exporter) collect(ch chan<- prometheus.Metric) error {
	// here is an example how we may collect server stats
	// we'll make an HTTP call to server URI which will return
	// status as a JSON document, then we'll assign metrics using JSON values
	var req *http.Request
	req, _ = http.NewRequest("GET", e.URI, nil)
	req.Header.Add("Accept-Encoding", "identity")
	req.Header.Add("Accept", "application/json")
	resp, err := _client.Do(req)
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
	var rec map[string]interface{}
	err = json.Unmarshal(data, &rec)
	if err != nil {
		return fmt.Errorf("Fail to unmarshal JSON data %s", err.Error())
	}
	if *verbose {
		fmt.Println(string(data))
	}
	var getCalls, postCalls, getRequests, postRequests float64
	if v, ok := rec["getCalls"]; ok {
		getCalls = v.(float64)
	}
	if v, ok := rec["postCalls"]; ok {
		postCalls = v.(float64)
	}
	if v, ok := rec["getRequests"]; ok {
		getRequests = v.(float64)
	}
	if v, ok := rec["postRequests"]; ok {
		postRequests = v.(float64)
	}
	var mem map[string]interface{}
	var mempct, swappct, cpupct, memtotal, memused, memfree float64
	if v, ok := rec["Memory"]; ok {
		mem = v.(map[string]interface{})
		if r, ok := mem["Virtual"]; ok {
			v := r.(map[string]interface{})
			mempct = v["usedPercent"].(float64)
			memtotal = v["total"].(float64)
			memused = v["used"].(float64)
			memfree = v["free"].(float64)
		}
		if r, ok := mem["Swap"]; ok {
			v := r.(map[string]interface{})
			swappct = v["usedPercent"].(float64)
		}
	}
	var memStatsSys, memStatsAlloc, memStatsTotalAlloc, memStatsHeapSys, memStatsStackSys float64
	if v, ok := rec["MemStats"]; ok {
		mem = v.(map[string]interface{})
		if v, ok := mem["Sys"]; ok {
			memStatsSys = v.(float64)
		}
		if v, ok := mem["Alloc"]; ok {
			memStatsAlloc = v.(float64)
		}
		if v, ok := mem["TotalAlloc"]; ok {
			memStatsTotalAlloc = v.(float64)
		}
		if v, ok := mem["HeapSys"]; ok {
			memStatsHeapSys = v.(float64)
		}
		if v, ok := mem["StackSys"]; ok {
			memStatsStackSys = v.(float64)
		}
	}
	var cores []float64
	if v, ok := rec["CPU"]; ok {
		cpus := v.([]interface{})
		for _, c := range cpus {
			val := c.(float64)
			cpupct += val
			cores = append(cores, val)
		}
		cpupct = cpupct / float64(len(cpus)) // take average of all available cores
	}
	var load1, load5, load15 float64
	if r, ok := rec["Load"]; ok {
		load := r.(map[string]interface{})
		if v, ok := load["load1"]; ok {
			load1 = v.(float64)
		}
		if v, ok := load["load5"]; ok {
			load5 = v.(float64)
		}
		if v, ok := load["load15"]; ok {
			load15 = v.(float64)
		}
	}
	var openFiles float64
	if v, ok := rec["OpenFiles"]; ok {
		files := v.([]interface{})
		openFiles = float64(len(files))
	}
	var ngo float64
	if v, ok := rec["NGo"]; ok {
		ngo = v.(float64)
	}
	var nthr float64
	if v, ok := rec["NThreads"]; ok {
		nthr = v.(float64)
	}
	var uptime float64
	if v, ok := rec["Uptime"]; ok {
		uptime = v.(float64)
	}
	var totCon, estCon, lisCon float64
	if v, ok := rec["Connections"]; ok {
		switch connections := v.(type) {
		case []interface{}:
			for _, c := range connections {
				con := c.(map[string]interface{})
				v, _ := con["status"]
				switch v {
				case "ESTABLISHED":
					estCon += 1
				case "LISTEN":
					lisCon += 1
				}
			}
			totCon = float64(len(connections))
		}
	}

	ch <- prometheus.MustNewConstMetric(e.getCalls, prometheus.CounterValue, getCalls)
	ch <- prometheus.MustNewConstMetric(e.postCalls, prometheus.CounterValue, postCalls)
	ch <- prometheus.MustNewConstMetric(e.getRequests, prometheus.CounterValue, getRequests)
	ch <- prometheus.MustNewConstMetric(e.postRequests, prometheus.CounterValue, postRequests)
	ch <- prometheus.MustNewConstMetric(e.uptime, prometheus.CounterValue, uptime)
	ch <- prometheus.MustNewConstMetric(e.memPercent, prometheus.GaugeValue, mempct)
	ch <- prometheus.MustNewConstMetric(e.memTotal, prometheus.GaugeValue, memtotal)
	ch <- prometheus.MustNewConstMetric(e.memFree, prometheus.GaugeValue, memfree)
	ch <- prometheus.MustNewConstMetric(e.memUsed, prometheus.GaugeValue, memused)
	ch <- prometheus.MustNewConstMetric(e.memStatsSys, prometheus.GaugeValue, memStatsSys)
	ch <- prometheus.MustNewConstMetric(e.memStatsAlloc, prometheus.GaugeValue, memStatsAlloc)
	ch <- prometheus.MustNewConstMetric(e.memStatsTotalAlloc, prometheus.GaugeValue, memStatsTotalAlloc)
	ch <- prometheus.MustNewConstMetric(e.memStatsHeapSys, prometheus.GaugeValue, memStatsHeapSys)
	ch <- prometheus.MustNewConstMetric(e.memStatsStackSys, prometheus.GaugeValue, memStatsStackSys)
	ch <- prometheus.MustNewConstMetric(e.swapPercent, prometheus.GaugeValue, swappct)
	ch <- prometheus.MustNewConstMetric(e.cpuPercent, prometheus.GaugeValue, cpupct)
	for i, v := range cores {
		labels := []string{fmt.Sprintf("core-%d", i)}
		ch <- prometheus.MustNewConstMetric(e.coresPercent, prometheus.GaugeValue, v, labels...)
	}
	ch <- prometheus.MustNewConstMetric(e.numThreads, prometheus.GaugeValue, nthr)
	ch <- prometheus.MustNewConstMetric(e.numGoroutines, prometheus.GaugeValue, ngo)
	ch <- prometheus.MustNewConstMetric(e.load1, prometheus.GaugeValue, load1)
	ch <- prometheus.MustNewConstMetric(e.load5, prometheus.GaugeValue, load5)
	ch <- prometheus.MustNewConstMetric(e.load15, prometheus.GaugeValue, load15)
	ch <- prometheus.MustNewConstMetric(e.openFiles, prometheus.GaugeValue, openFiles)
	ch <- prometheus.MustNewConstMetric(e.totCon, prometheus.GaugeValue, totCon)
	ch <- prometheus.MustNewConstMetric(e.lisCon, prometheus.GaugeValue, lisCon)
	ch <- prometheus.MustNewConstMetric(e.estCon, prometheus.GaugeValue, estCon)
	return nil
}

// main function
func main() {
	flag.Parse()
	exporter := NewExporter(*scrapeURI)
	prometheus.MustRegister(exporter)

	log.Infof("Starting Server: %s", *listeningAddress)
	http.Handle(*metricsEndpoint, promhttp.Handler())
	log.Fatal(http.ListenAndServe(*listeningAddress, nil))
}

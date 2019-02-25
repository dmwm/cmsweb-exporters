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
	"github.com/prometheus/common/log"
	"github.com/vkuznet/x509proxy"
)

const (
	namespace = "das2go" // For Prometheus metrics.
)

var (
	listeningAddress = flag.String("port", ":18217", "port to expose metrics on web interface.")
	metricsEndpoint  = flag.String("endpoint", "/metrics", "Path under which to expose metrics.")
	scrapeURI        = flag.String("uri", "", "URI of server status page we're going to scrape")
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

	scrapeFailures prometheus.Counter
	getCalls       *prometheus.Desc
	postCalls      *prometheus.Desc
	getRequests    *prometheus.Desc
	postRequests   *prometheus.Desc
	uptime         *prometheus.Desc
	connections    *prometheus.GaugeVec
	memPercent     *prometheus.Desc
	swapPercent    *prometheus.Desc
	cpuPercent     *prometheus.Desc
	numThreads     *prometheus.Desc
}

func NewExporter(uri string) *Exporter {
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
		connections: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "connections",
			Help:      "connection statuses",
		},
			[]string{"state"},
		),
		memPercent: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "memory_percent"),
			"Virtual memory usage of the server",
			nil,
			nil),
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
		numThreads: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "num_threads"),
			"Number of threads or Go routines",
			nil,
			nil),
		//         client: &http.Client{},
	}
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.getCalls
	ch <- e.postCalls
	ch <- e.getRequests
	ch <- e.postRequests
	ch <- e.uptime
	ch <- e.memPercent
	ch <- e.swapPercent
	ch <- e.cpuPercent
	ch <- e.numThreads
	e.connections.Describe(ch)
}

// Collect performs metrics collectio of exporter attributes
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.mutex.Lock() // To protect metrics from concurrent collects.
	defer e.mutex.Unlock()
	if err := e.collect(ch); err != nil {
		log.Errorf("Error scraping: %s", err)
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
	var mempct, swappct, cpupct float64
	if v, ok := rec["Memory"]; ok {
		mem = v.(map[string]interface{})
		if r, ok := mem["Virtual"]; ok {
			v := r.(map[string]interface{})
			mempct = v["usedPercent"].(float64)
		}
		if r, ok := mem["Swap"]; ok {
			v := r.(map[string]interface{})
			swappct = v["usedPercent"].(float64)
		}
	}
	if v, ok := rec["CPU"]; ok {
		cpus := v.([]interface{})
		for _, c := range cpus {
			cpupct += c.(float64)
		}
		cpupct = cpupct / float64(len(cpus)) // take average of all available cores
	}
	var nthr float64
	if v, ok := rec["NGo"]; ok {
		nthr = v.(float64)
	} else if v, ok := rec["NThreads"]; ok {
		nthr = v.(float64)
	}
	var uptime float64
	if v, ok := rec["Uptime"]; ok {
		uptime = v.(float64)
	}
	if v, ok := rec["Connections"]; ok {
		switch connections := v.(type) {
		case [][]interface{}:
			var totCon, estCon, lisCon float64
			for _, c := range connections {
				v := c[len(c)-1].(string)
				switch v {
				case "ESTABLISHED":
					estCon += 1
				case "LISTEN":
					lisCon += 1
				}
			}
			totCon = float64(len(connections))
			e.connections.WithLabelValues("total").Set(totCon)
			e.connections.WithLabelValues("established").Set(estCon)
			e.connections.WithLabelValues("listen").Set(lisCon)
			e.connections.Collect(ch)
		}
	}

	ch <- prometheus.MustNewConstMetric(e.getCalls, prometheus.CounterValue, getCalls)
	ch <- prometheus.MustNewConstMetric(e.postCalls, prometheus.CounterValue, postCalls)
	ch <- prometheus.MustNewConstMetric(e.getRequests, prometheus.CounterValue, getRequests)
	ch <- prometheus.MustNewConstMetric(e.postRequests, prometheus.CounterValue, postRequests)
	ch <- prometheus.MustNewConstMetric(e.uptime, prometheus.CounterValue, uptime)
	ch <- prometheus.MustNewConstMetric(e.memPercent, prometheus.CounterValue, mempct)
	ch <- prometheus.MustNewConstMetric(e.swapPercent, prometheus.CounterValue, swappct)
	ch <- prometheus.MustNewConstMetric(e.cpuPercent, prometheus.CounterValue, cpupct)
	ch <- prometheus.MustNewConstMetric(e.numThreads, prometheus.CounterValue, nthr)
	return nil
}

// main function
func main() {
	flag.Parse()
	exporter := NewExporter(*scrapeURI)
	prometheus.MustRegister(exporter)

	log.Infof("Starting Server: %s", *listeningAddress)
	http.Handle(*metricsEndpoint, prometheus.Handler())
	log.Fatal(http.ListenAndServe(*listeningAddress, nil))
}

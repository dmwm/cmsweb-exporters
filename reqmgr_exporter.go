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

// const (
//     namespace = "wmcore" // For Prometheus metrics.
// )

var (
	listeningAddress = flag.String("port", ":18240", "port to expose metrics and web interface.")
	metricsEndpoint  = flag.String("endpoint", "/metrics", "Path under which to expose metrics.")
	scrapeURI        = flag.String("uri", "", "URI of server status page we're going to scrape")
	namespace        = flag.String("namespace", "wmcore", "namespace for prometheus metrics")
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
	uptime         *prometheus.Desc
	memPercent     *prometheus.Desc
	memVms         *prometheus.Desc
	memRss         *prometheus.Desc
	memSwap        *prometheus.Desc
	memPss         *prometheus.Desc
	memUss         *prometheus.Desc
	cpuPercent     *prometheus.Desc
	cpuSystem      *prometheus.Desc
	cpuUser        *prometheus.Desc
	cpuChSystem    *prometheus.Desc
	cpuChUser      *prometheus.Desc
	cpuNumber      *prometheus.Desc
	time           *prometheus.Desc
}

func NewExporter(uri string) *Exporter {
	return &Exporter{
		URI: uri,
		uptime: prometheus.NewDesc(
			prometheus.BuildFQName(*namespace, "", "uptime"),
			"Current uptime in seconds",
			nil,
			nil),
		memPercent: prometheus.NewDesc(
			prometheus.BuildFQName(*namespace, "", "memory_percent"),
			"Virtual memory usage of the server",
			nil,
			nil),
		cpuPercent: prometheus.NewDesc(
			prometheus.BuildFQName(*namespace, "", "cpu_percent"),
			"cpu percent of the server",
			nil,
			nil),
		cpuNumber: prometheus.NewDesc(
			prometheus.BuildFQName(*namespace, "", "num_cpu"),
			"Number of CPUs",
			nil,
			nil),
		time: prometheus.NewDesc(
			prometheus.BuildFQName(*namespace, "", "time"),
			"Timestamp of the metric",
			nil,
			nil),
		memVms: prometheus.NewDesc(
			prometheus.BuildFQName(*namespace, "", "vms"),
			"Memory VMS metric",
			nil,
			nil),
		memRss: prometheus.NewDesc(
			prometheus.BuildFQName(*namespace, "", "rss"),
			"Memory RSS metric",
			nil,
			nil),
		memSwap: prometheus.NewDesc(
			prometheus.BuildFQName(*namespace, "", "swap"),
			"Memory Swap metric",
			nil,
			nil),
		memPss: prometheus.NewDesc(
			prometheus.BuildFQName(*namespace, "", "pss"),
			"Memory PSS metric",
			nil,
			nil),
		memUss: prometheus.NewDesc(
			prometheus.BuildFQName(*namespace, "", "uss"),
			"Memory USS metric",
			nil,
			nil),
		cpuSystem: prometheus.NewDesc(
			prometheus.BuildFQName(*namespace, "", "cpu_system"),
			"CPU system metric",
			nil,
			nil),
		cpuUser: prometheus.NewDesc(
			prometheus.BuildFQName(*namespace, "", "cpu_user"),
			"CPU user metric",
			nil,
			nil),
		cpuChSystem: prometheus.NewDesc(
			prometheus.BuildFQName(*namespace, "", "cpu_children_system"),
			"CPU children system metric",
			nil,
			nil),
		cpuChUser: prometheus.NewDesc(
			prometheus.BuildFQName(*namespace, "", "cpu_children_user"),
			"CPU children user metric",
			nil,
			nil),
	}
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.uptime
	ch <- e.memPercent
	ch <- e.cpuPercent
	ch <- e.cpuNumber
	ch <- e.memVms
	ch <- e.memRss
	ch <- e.memSwap
	ch <- e.memPss
	ch <- e.memUss
	ch <- e.cpuSystem
	ch <- e.cpuUser
	ch <- e.cpuChSystem
	ch <- e.cpuChUser
	ch <- e.time
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

// MemoryInfo holds information about memory returned by psutil
type MemoryInfo struct {
	Data   int64 `json:"data"`
	Dirty  int64 `json:"dirty"`
	Lib    int64 `json:"lib"`
	Pss    int64 `json:"pss"`
	Rss    int64 `json:"rss"`
	Shared int64 `json:"shared"`
	Swap   int64 `json:"swap"`
	Text   int64 `json:"text"`
	Uss    int64 `json:"uss"`
	Vms    int64 `json:"vms"`
}

// String dumps MemoryInfo into string object
func (m *MemoryInfo) String() string {
	data, _ := json.Marshal(m)
	return string(data)
}

// CPUTimes holds information about CPU metrics
type CPUTimes struct {
	ChildrenUser   float64 `json:"children_user"`
	ChildrenSystem float64 `json:"children_system"`
	System         float64 `json:"system"`
	User           float64 `json:"user"`
}

// String dumps CPUTimes into string object
func (c *CPUTimes) String() string {
	data, _ := json.Marshal(c)
	return string(data)
}

// ReqMgrMetrics represents metrics used by request manager
// so far we declare MemoryMapInfo, OpenFileInfo, TheadsInfo and Connectioninfo
// as generic interfaces since psutil returns list of mixed data-types
type ReqMgrMetrics struct {
	CpuTimes      CPUTimes   `json:"cpu_times"`
	MemoryPercent float64    `json:"memory_percent"`
	MemoryInfo    MemoryInfo `json:"memory_full_info"`
	Uptime        float64    `json:"uptime"`
	CpuPercent    float64    `json:"cpu_percent"`
	CpuNum        int64      `json:"cpu_num"`
	Timestamp     string     `json:"timestamp"`
	Time          float64    `json:"time"`
	Pid           int64      `json:"pid"`
}

// String dumps ReqMgrMetrics into string object
func (r *ReqMgrMetrics) String() string {
	data, _ := json.Marshal(r)
	return string(data)
}

// MetricsInfo holds server object returned by rquest manager
type MetricsInfo struct {
	Server ReqMgrMetrics `json:"server"`
}

// String dumps MetricsInfo into string object
func (r *MetricsInfo) String() string {
	data, _ := json.Marshal(r)
	return string(data)
}

// ReqMgrResults holds results object returned by request manager
type ReqMgrResults struct {
	Result []MetricsInfo `json:"result"`
}

// String dumps ReqMgrResults into string object
func (r *ReqMgrResults) String() string {
	data, _ := json.Marshal(r)
	return string(data)
}

// helper function to parse input data
func parseData(data []byte) (ReqMgrMetrics, error) {
	var m ReqMgrMetrics
	var r ReqMgrResults
	err := json.Unmarshal(data, &r)
	if err != nil {
		return m, err
	}
	m = r.Result[0].Server
	return m, nil
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
		return fmt.Errorf("Error scraping service: %v", err)
	}

	data, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		if err != nil {
			data = []byte(err.Error())
		}
		return fmt.Errorf("Status %s (%d): %s", resp.Status, resp.StatusCode, data)
	}
	// here we parse input data and extract from it metrics we want to monitor
	rec, err := parseData(data)
	if err != nil {
		return fmt.Errorf("Error to parse incoming data: %v", err)
	}

	ch <- prometheus.MustNewConstMetric(e.uptime, prometheus.CounterValue, rec.Uptime)
	ch <- prometheus.MustNewConstMetric(e.memPercent, prometheus.CounterValue, rec.MemoryPercent)
	ch <- prometheus.MustNewConstMetric(e.cpuPercent, prometheus.CounterValue, rec.CpuPercent)
	ch <- prometheus.MustNewConstMetric(e.cpuNumber, prometheus.CounterValue, float64(rec.CpuNum))
	ch <- prometheus.MustNewConstMetric(e.memVms, prometheus.CounterValue, float64(rec.MemoryInfo.Vms))
	ch <- prometheus.MustNewConstMetric(e.memRss, prometheus.CounterValue, float64(rec.MemoryInfo.Rss))
	ch <- prometheus.MustNewConstMetric(e.memSwap, prometheus.CounterValue, float64(rec.MemoryInfo.Swap))
	ch <- prometheus.MustNewConstMetric(e.memPss, prometheus.CounterValue, float64(rec.MemoryInfo.Pss))
	ch <- prometheus.MustNewConstMetric(e.memUss, prometheus.CounterValue, float64(rec.MemoryInfo.Uss))
	ch <- prometheus.MustNewConstMetric(e.cpuSystem, prometheus.CounterValue, rec.CpuTimes.System)
	ch <- prometheus.MustNewConstMetric(e.cpuUser, prometheus.CounterValue, rec.CpuTimes.User)
	ch <- prometheus.MustNewConstMetric(e.cpuChSystem, prometheus.CounterValue, rec.CpuTimes.ChildrenSystem)
	ch <- prometheus.MustNewConstMetric(e.cpuChUser, prometheus.CounterValue, rec.CpuTimes.ChildrenUser)
	ch <- prometheus.MustNewConstMetric(e.time, prometheus.CounterValue, rec.Time)

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

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

// const (
//     namespace = "wmcore" // For Prometheus metrics.
// )

var (
	listeningAddress = flag.String("address", ":7300", "Address on which to expose metrics and web interface.")
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
	connections    *prometheus.GaugeVec
	memPercent     *prometheus.Desc
	swapPercent    *prometheus.Desc
	cpuPercent     *prometheus.Desc
	numThreads     *prometheus.Desc
	numFds         *prometheus.Desc
}

func NewExporter(uri string) *Exporter {
	return &Exporter{
		URI: uri,
		uptime: prometheus.NewDesc(
			prometheus.BuildFQName(*namespace, "", "uptime"),
			"Current uptime in seconds",
			nil,
			nil),
		connections: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: *namespace,
			Name:      "connections",
			Help:      "connection statuses",
		},
			[]string{"state"},
		),
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
		numThreads: prometheus.NewDesc(
			prometheus.BuildFQName(*namespace, "", "num_threads"),
			"Number of threads or Go routines",
			nil,
			nil),
		numFds: prometheus.NewDesc(
			prometheus.BuildFQName(*namespace, "", "num_fds"),
			"Number of file descriptors",
			nil,
			nil),
	}
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.uptime
	ch <- e.memPercent
	ch <- e.cpuPercent
	ch <- e.numThreads
	ch <- e.numFds
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

// HostInfo holds information about host info returned by psutil
type HostInfo struct {
	Ip   string //`json:"ip"`
	Port int64  //`json:"port"`
}

// Connectioninfo holds information about connections returned by psutil
type ConnectionInfo struct {
	Fd     int64      //`json:"fd"`
	Family int64      //`json:"family"`
	Type   int64      //`json:"type"`
	Host   []HostInfo //`json:"host"`
	Status string     //`json:"status"`
}

// ThreadInfo holds information about threads returned by psutil
type ThreadInfo struct {
	Id         int64   //`json:"id"`
	UserTime   float64 //`json:"user_time"`
	SystemTime float64 //`json:"system_time"`
}

// OpenFileInfo holds information about open files returned by psutil
type OpenFileInfo struct {
	Path string //`json:"path"`
	Fd   int64  //`json:"fd"`
}

// MemoryMapInfo holds information about memory returned by psutil
type MemoryMapInfo struct {
	Path         string //`json:"path"`
	Rss          int64  //`json:"rss"`
	Size         int64  //`json:"size"`
	Pss          int64  //`json:"pss"`
	SharedClean  int64  //`json:"shared_clean"`
	SharedDirty  int64  //`json:"shared_dirty"`
	PrivateClean int64  //`json:"private_clean"`
	PrivateDirty int64  //`json:"private_dirty"`
	Referenced   int64  //`json:"referenced"`
	Anonymous    int64  //`json:"anonymous"`
	Swap         int64  //`json:"swap"`
}

// ReqMgrMetrics represents metrics used by request manager
// so far we declare MemoryMapInfo, OpenFileInfo, TheadsInfo and Connectioninfo
// as generic interfaces since psutil returns list of mixed data-types
type ReqMgrMetrics struct {
	Username   string    `json:"username"`
	Status     string    `json:"status"`
	Cputimes   []float64 `json:"cputimes"`
	Timestamp  string    `json:"timestamp"`
	Pid        int64     `json:"pid"`
	Uptime     float64   `json:"uptime"`
	Iocounters []int64   `json:""iocounters`
	//     Connections    []ConnectionInfo  `json:"connections"`
	Connections interface{} `json:"connections"`
	Cmdline     []string    `json:"cmdline"`
	CreateTime  float64     `json:"create_time"`
	//     Threads     [][]ThreadInfo `json:"threads"`
	Threads      interface{} `json:"threads"`
	MemoryInfoEx []int64     `json:"memory_info_ex"`
	Ionice       []int64     `json:"ionice"`
	//     OpenFiles      [][]OpenFileInfo  `json:"open_files"`
	OpenFiles  interface{} `json:"open_files"`
	NumFds     int64       `json:"num_fds"`
	Uids       []int64     `json:"uids"`
	NumThreads int64       `json:"num_threads"`
	//     MemoryMaps     [][]MemoryMapInfo `json:"memory_maps"`
	MemoryMaps     interface{} `json:"memory_maps"`
	Exe            string      `json:"exe"`
	Name           string      `json:"name"`
	CpuPercent     float64     `json:"cpu_percent"`
	CpuAffinity    []int64     `json:"cpu_affinity"`
	Gids           []int64     `json:"gids"`
	Terminal       string      `json:"terminal"`
	MemoryPercent  float64     `json:"memory_percent"`
	MemoryInfo     []int64     `json:"memory_info"`
	NumCtxSwitches []int64     `json:"num_ctx_switches"`
	Time           float64     `json:"time"`
	Ppid           int64       `json:"ppid"`
	Cwd            string      `json:"cwd"`
	Nice           int64       `json:"nice"`
}

// MetricsInfo holds server object returned by rquest manager
type MetricsInfo struct {
	Server ReqMgrMetrics `json:"server"`
}

// ReqMgrResults holds results object returned by request manager
type ReqMgrResults struct {
	Result []MetricsInfo `json:"result"`
}

// helper function to parse input data
func parseData(data []byte) (ReqMgrMetrics, error) {
	var m ReqMgrMetrics
	var r ReqMgrResults
	err := json.Unmarshal(data, &r)
	if err != nil {
		return m, err
	}
	fmt.Println(string(data))
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
    // here we parse input data and extract from it metrics we want to monitor
    rec := parseData(data)
	var uptime, mempct, cpupct, nthr, nfds float64

	ch <- prometheus.MustNewConstMetric(e.uptime, prometheus.CounterValue, uptime)
	ch <- prometheus.MustNewConstMetric(e.memPercent, prometheus.CounterValue, mempct)
	ch <- prometheus.MustNewConstMetric(e.cpuPercent, prometheus.CounterValue, cpupct)
	ch <- prometheus.MustNewConstMetric(e.numThreads, prometheus.CounterValue, nthr)
	ch <- prometheus.MustNewConstMetric(e.numThreads, prometheus.CounterValue, nfds)
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

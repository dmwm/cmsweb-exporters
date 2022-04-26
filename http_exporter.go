package main

// Author: Valentin Kuznetsov <vkuznet [AT] gmail {DOT} com>
// Example of cmsweb data-service exporter for prometheus.io

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/user"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/vkuznet/x509proxy"
)

var (
	listeningAddress    = flag.String("port", ":18000", "port to expose metrics and web interface.")
	metricsEndpoint     = flag.String("endpoint", "/metrics", "Path under which to expose metrics.")
	scrapeURI           = flag.String("uri", "", "URI of server status page we're going to scrape")
	proxyfile           = flag.String("proxyfile", "", "proxy file name")
	agent               = flag.String("agent", "", "User-agent to use")
	namespace           = flag.String("namespace", "http", "namespace for prometheus metrics")
	contentType         = flag.String("contentType", "", "ContentType to use for HTTP request")
	connectionTimeout   = flag.Int("connectionTimeout", 3, "connection timeout for HTTP request")
	renewClientInterval = flag.Int("renewClientInterval", 600, "renew interval for http client in seconds. If proxy is not needed, please provide 0 or negative integer")
	verbose             = flag.Bool("verbose", false, "verbose output")
)

// global client's x509 certificates
var _certs []tls.Certificate

type HttpClientMgr struct {
	Client *http.Client
	Expire int64
}

func (h *HttpClientMgr) getHttpClient() *http.Client {
	// No need to renew proxy auth or set expiration
	if int(*renewClientInterval) <= 0 {
		h.Client = HttpClient()
	} else if h.Expire < time.Now().Unix() {
		_certs = []tls.Certificate{} // remove cached certs
		h.Client = HttpClient()
		h.Expire = time.Now().Unix() + int64(*renewClientInterval)
		if *verbose {
			log.Printf("Renew http client, new expire %+v\n", time.Unix(h.Expire, 0))
		}
	}
	return h.Client
}

// global http client manager
var httpClientMgr HttpClientMgr

// client X509 certificates
func tlsCerts() ([]tls.Certificate, error) {
	// No need to renew proxy auth
	if int(*renewClientInterval) <= 0 {
		return nil, nil
	}
	if len(_certs) != 0 {
		return _certs, nil // use cached certs
	}
	uproxy := os.Getenv("X509_USER_PROXY")
	uckey := os.Getenv("X509_USER_KEY")
	ucert := os.Getenv("X509_USER_CERT")

	if *proxyfile == "" {
		// check if /tmp/x509up_u$UID exists, if so setup X509_USER_PROXY env
		u, err := user.Current()
		if err == nil {
			fname := fmt.Sprintf("/tmp/x509up_u%s", u.Uid)
			if _, err := os.Stat(fname); err == nil {
				uproxy = fname
			}
		}
	} else {
		if _, err := os.Stat(*proxyfile); err == nil {
			uproxy = *proxyfile
		}
	}
	if *verbose {
		log.Printf("user credentials: proxy=%s cert=%s ckey=%s\n", uproxy, ucert, uckey)
	}

	if uproxy == "" && uckey == "" { // user doesn't have neither proxy or user certs
		return nil, fmt.Errorf("neither proxy or user certs are found, please setup X509 environment variables")
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

// help function to timeout on stale connection
func dialTimeout(network, addr string) (net.Conn, error) {
	var timeout = time.Duration(*connectionTimeout) * time.Second
	conn, err := net.DialTimeout(network, addr, timeout)
	return conn, err
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
		IdleConnTimeout:   time.Duration(1 * time.Second),
		Dial:              dialTimeout,
		DisableKeepAlives: true,
	}
	timeout := time.Duration(*connectionTimeout) * time.Second
	return &http.Client{Transport: tr, Timeout: timeout}
}

type Exporter struct {
	URI            string
	mutex          sync.Mutex
	scrapeFailures prometheus.Counter
	status         *prometheus.Desc
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
		log.Printf("Error scraping: %s\n", err)
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
	if *contentType != "" {
		req.Header.Add("Accept", *contentType)
	}
	if *agent != "" {
		req.Header.Add("User-Agent", *agent)
	}

	/*
		// Example how to organize termination of function
		// see: https://blog.golang.org/concurrency-timeouts

		// get http response from the site
		var resp *http.Response
		var respError error
		abort := make(chan struct{})
		go func(r *http.Request) {
			resp, respError = httpClient.Do(r)
			abort <- "ok"
		}(req)

		// try to get response or timeout after connection timeout interval
		select {
		case <-abort:
			// a read from abort channel has occurred, let's close it
			close(abort)
		case <-time.After(time.Duration(*connectionTimeout) * time.Second):
			// the read from ch has timed out
			msg := fmt.Sprintf("Timeout after %v (sec)", *connectionTimeout)
			respError = errors.New(msg)
			close(abort)
		}
	*/

	httpClient := httpClientMgr.getHttpClient()
	resp, respError := httpClient.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if respError != nil {
		ch <- prometheus.MustNewConstMetric(e.status, prometheus.CounterValue, 0)
		if *verbose {
			log.Printf("Faile to make HTTP request, url=%s, error=%v\n", e.URI, respError)
		}
		return nil
	}

	val := float64(resp.StatusCode)
	data, err := ioutil.ReadAll(resp.Body)
	// Stdout response body if not successful
	if resp.StatusCode != 200 {
		if err != nil {
			data = []byte(err.Error())
		}
		ch <- prometheus.MustNewConstMetric(e.status, prometheus.CounterValue, val)
		if *verbose {
			log.Printf("HTTP request info, status=%v code=%v data=%s\n", resp.Status, resp.StatusCode, string(data))
		}
		return nil
	}
	if *contentType == "application/json" {
		var rec map[string]interface{}
		err = json.Unmarshal(data, &rec)
		if err != nil {
			// let's try to decode list of records
			var records []map[string]interface{}
			err = json.Unmarshal(data, &records)
			if err != nil {
				if *verbose {
					log.Printf("Fail to unmarshal the data, error=%v data=%s\n", err.Error(), string(data))
				}
				ch <- prometheus.MustNewConstMetric(e.status, prometheus.CounterValue, 0)
				return nil
			}
			ch <- prometheus.MustNewConstMetric(e.status, prometheus.CounterValue, val)
			return nil
		}
		if *verbose {
			log.Println("received data", string(data))
		}
	} else {
		val := float64(resp.StatusCode)
		ch <- prometheus.MustNewConstMetric(e.status, prometheus.CounterValue, val)
	}
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

	log.Printf("Starting Server: %s\n", *listeningAddress)
	http.Handle(*metricsEndpoint, promhttp.Handler())
	log.Fatal(http.ListenAndServe(*listeningAddress, nil))
}

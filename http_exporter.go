package main

// Author: Valentin Kuznetsov <vkuznet [AT] gmail {DOT} com>
// Example of cmsweb data-service exporter for prometheus.io

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/user"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/log"
	logs "github.com/sirupsen/logrus"
	"github.com/vkuznet/x509proxy"
)

var (
	listeningAddress  = flag.String("port", ":18000", "port to expose metrics and web interface.")
	metricsEndpoint   = flag.String("endpoint", "/metrics", "Path under which to expose metrics.")
	scrapeURI         = flag.String("uri", "", "URI of server status page we're going to scrape")
	proxyfile         = flag.String("proxyfile", "", "proxy file name")
	namespace         = flag.String("namespace", "http", "namespace for prometheus metrics")
	contentType       = flag.String("contentType", "", "ContentType to use for HTTP request")
	connectionTimeout = flag.Int("connectionTimeout", 3, "connection timeout for HTTP request")
	verbose           = flag.Bool("verbose", false, "verbose output")
)

// global client's x509 certificates
var _certs []tls.Certificate

var httpClient *http.Client

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
		logs.WithFields(logs.Fields{
			"proxy": uproxy,
			"cert":  ucert,
			"key":   uckey,
		}).Info("user credentials")
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
	if *contentType != "" {
		req.Header.Add("Accept", *contentType)
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

	resp, respError := httpClient.Do(req)
	if respError != nil {
		ch <- prometheus.MustNewConstMetric(e.status, prometheus.CounterValue, 0)
		if *verbose {
			logs.WithFields(logs.Fields{
				"URL":   e.URI,
				"Error": respError,
			}).Info("Fail to make HTTP request")
		}
		return nil
	}

	val := float64(resp.StatusCode)
	if *contentType == "application/json" {
		data, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			if err != nil {
				data = []byte(err.Error())
			}
			ch <- prometheus.MustNewConstMetric(e.status, prometheus.CounterValue, val)
			if *verbose {
				logs.WithFields(logs.Fields{
					"Status": resp.Status,
					"Code":   resp.StatusCode,
					"Data":   string(data),
				}).Info("HTTP request info")
			}
			return nil
		}
		var rec map[string]interface{}
		err = json.Unmarshal(data, &rec)
		if err != nil {
			// let's try to decode list of records
			var records []map[string]interface{}
			err = json.Unmarshal(data, &records)
			if err != nil {
				if *verbose {
					logs.WithFields(logs.Fields{
						"Error": err.Error(),
						"Data":  string(data),
					}).Info("Fail to unmarshal the data")
				}
				ch <- prometheus.MustNewConstMetric(e.status, prometheus.CounterValue, 0)
				return nil
			}
			ch <- prometheus.MustNewConstMetric(e.status, prometheus.CounterValue, val)
			return nil
		}
		if *verbose {
			logs.WithFields(logs.Fields{
				"Data": string(data),
			}).Info("received data")
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
	exporter := NewExporter(*scrapeURI)
	prometheus.MustRegister(exporter)
	httpClient = HttpClient()

	log.Infof("Starting Server: %s", *listeningAddress)
	http.Handle(*metricsEndpoint, promhttp.Handler())
	log.Fatal(http.ListenAndServe(*listeningAddress, nil))
}

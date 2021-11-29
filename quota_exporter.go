package main

import (
	"encoding/json"
	"flag"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"net/http"
	"os/exec"
	"sync"
	"time"
)

// QuotaRecords represent quota.sh output
type QuotaRecords struct {
	TotalCpus           int64 `json:"total_cpus"`
	CpusUsed            int64 `json:"cpus_used"`
	TotalRam            int64 `json:"total_ram"`
	RamUsed             int64 `json:"ram_used"`
	TotalInstances      int64 `json:"total_instances"`
	InstancesUsed       int64 `json:"instances_used"`
	TotalVolume         int64 `json:"total_volume"`
	VolumesUsed         int64 `json:"volumes_used"`
	TotalVolumeSize     int64 `json:"total_volume_size"`
	TotalVolumeSizeUsed int64 `json:"total_volume_size_used"`
	TotalShares         int64 `json:"total_shares"`
	SharesUsed          int64 `json:"shares_used"`
	SharesSize          int64 `json:"shares_size"`
	SharesSizeUsed      int64 `json:"shares_size_used"`
}

// Exporter represents Prometheus exporter structure
type Exporter struct {
	scriptName     string
	envFile        string
	mutex          sync.Mutex
	scrapeFailures prometheus.Counter

	totalCpus           *prometheus.Desc
	cpusUsed            *prometheus.Desc
	totalRam            *prometheus.Desc
	ramUsed             *prometheus.Desc
	totalInstances      *prometheus.Desc
	instancesUsed       *prometheus.Desc
	totalVolume         *prometheus.Desc
	volumesUsed         *prometheus.Desc
	totalVolumeSize     *prometheus.Desc
	totalVolumeSizeUsed *prometheus.Desc
	totalShares         *prometheus.Desc
	sharesUsed          *prometheus.Desc
	sharesSize          *prometheus.Desc
	sharesSizeUsed      *prometheus.Desc
	timestamp           *prometheus.Desc
}

func NewExporter(namespace, scriptName, envFile string) *Exporter {
	return &Exporter{
		scriptName:     scriptName,
		envFile:        envFile,
		scrapeFailures: prometheus.NewCounter(prometheus.CounterOpts{}),
		totalCpus: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "total_cpus"),
			"",
			nil,
			nil),
		cpusUsed: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "cpus_used"),
			"",
			nil,
			nil),
		totalRam: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "total_ram"),
			"",
			nil,
			nil),
		ramUsed: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "ram_used"),
			"",
			nil,
			nil),
		totalInstances: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "total_instances"),
			"",
			nil,
			nil),
		instancesUsed: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "instances_used"),
			"",
			nil,
			nil),
		totalVolume: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "total_volume"),
			"",
			nil,
			nil),
		volumesUsed: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "volumes_used"),
			"",
			nil,
			nil),
		totalVolumeSize: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "total_volume_size"),
			"",
			nil,
			nil),
		totalVolumeSizeUsed: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "total_volume_size_used"),
			"",
			nil,
			nil),
		totalShares: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "total_shares"),
			"",
			nil,
			nil),
		sharesUsed: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "shares_used"),
			"",
			nil,
			nil),
		sharesSize: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "shares_size"),
			"",
			nil,
			nil),
		sharesSizeUsed: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "shares_size_used"),
			"",
			nil,
			nil),
		timestamp: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "timestamp"),
			"",
			nil,
			nil),
	}
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.totalCpus
	ch <- e.cpusUsed
	ch <- e.totalRam
	ch <- e.ramUsed
	ch <- e.totalInstances
	ch <- e.instancesUsed
	ch <- e.totalVolume
	ch <- e.volumesUsed
	ch <- e.totalVolumeSize
	ch <- e.totalVolumeSizeUsed
	ch <- e.totalShares
	ch <- e.sharesUsed
	ch <- e.sharesSize
	ch <- e.sharesSizeUsed
	ch <- e.timestamp

}

// Collect performs metrics collection of exporter attributes
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

func run(scriptPath, envFile string) QuotaRecords {
	var record QuotaRecords
	command := exec.Command("/bin/bash", scriptPath, envFile)
	stdout, err := command.Output()
	if err != nil {
		log.Fatal(err)
	}
	// Unmarshall bash script output.
	if err2 := json.Unmarshal(stdout, &record); err2 != nil {
		log.Fatal(err2)
	}
	return record
}

// helper function which collects exporter attributes
func (e *Exporter) collect(ch chan<- prometheus.Metric) error {
	// extract records
	records := run(e.scriptName, e.envFile)
	timestamp := time.Now().Unix()
	ch <- prometheus.MustNewConstMetric(e.totalCpus, prometheus.CounterValue, float64(records.TotalCpus))
	ch <- prometheus.MustNewConstMetric(e.cpusUsed, prometheus.CounterValue, float64(records.CpusUsed))
	ch <- prometheus.MustNewConstMetric(e.totalRam, prometheus.CounterValue, float64(records.TotalRam))
	ch <- prometheus.MustNewConstMetric(e.ramUsed, prometheus.CounterValue, float64(records.RamUsed))
	ch <- prometheus.MustNewConstMetric(e.totalInstances, prometheus.CounterValue, float64(records.TotalInstances))
	ch <- prometheus.MustNewConstMetric(e.instancesUsed, prometheus.CounterValue, float64(records.InstancesUsed))
	ch <- prometheus.MustNewConstMetric(e.totalVolume, prometheus.CounterValue, float64(records.TotalVolume))
	ch <- prometheus.MustNewConstMetric(e.volumesUsed, prometheus.CounterValue, float64(records.VolumesUsed))
	ch <- prometheus.MustNewConstMetric(e.totalVolumeSize, prometheus.CounterValue, float64(records.TotalVolumeSize))
	ch <- prometheus.MustNewConstMetric(e.totalVolumeSizeUsed, prometheus.CounterValue, float64(records.TotalVolumeSizeUsed))
	ch <- prometheus.MustNewConstMetric(e.totalShares, prometheus.CounterValue, float64(records.TotalShares))
	ch <- prometheus.MustNewConstMetric(e.sharesUsed, prometheus.CounterValue, float64(records.SharesUsed))
	ch <- prometheus.MustNewConstMetric(e.sharesSize, prometheus.CounterValue, float64(records.SharesSize))
	ch <- prometheus.MustNewConstMetric(e.sharesSizeUsed, prometheus.CounterValue, float64(records.SharesSizeUsed))
	ch <- prometheus.MustNewConstMetric(e.timestamp, prometheus.CounterValue, float64(timestamp))

	return nil
}

func main() {
	var verbose int
	flag.IntVar(&verbose, "verbose", 0, "verbose output")
	var listeningAddress string
	flag.StringVar(&listeningAddress, "address", ":18000", "address of web interface.")
	var endpoint string
	flag.StringVar(&endpoint, "endpoint", "/metrics", "Path under which to expose metrics.")
	var namespace string
	flag.StringVar(&namespace, "namespace", "default", "namespace to use for exporter")
	var scriptPath string
	flag.StringVar(&scriptPath, "script", "quota.sh", "bash script file name")
	var envFile string
	flag.StringVar(&envFile, "env", "/etc/secrets/env.sh", "env.sh")
	flag.Parse()
	if verbose > 0 {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	} else {
		log.SetFlags(log.LstdFlags)
	}

	exporter := NewExporter(namespace, scriptPath, envFile)
	prometheus.MustRegister(exporter)

	log.Printf("Starting Server: %s\n", listeningAddress)
	http.Handle(endpoint, promhttp.Handler())
	log.Fatal(http.ListenAndServe(listeningAddress, nil))
}

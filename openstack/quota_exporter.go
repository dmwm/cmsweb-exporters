package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/yaml.v2"
	"log"
	"net/http"
	"os/exec"
	"sync"
	"time"
)

// Exporter represents Prometheus exporter structure
type Exporter struct {
	quotaScriptPath string
	keystoneEnvFile string
	mutex           sync.Mutex
	scrapeFailures  prometheus.Counter

	cpusTotal          *prometheus.Desc
	cpusUsed           *prometheus.Desc
	ramTotalGB         *prometheus.Desc
	ramUsedGB          *prometheus.Desc
	instancesTotal     *prometheus.Desc
	instancesUsed      *prometheus.Desc
	volumesTotal       *prometheus.Desc
	volumesUsed        *prometheus.Desc
	volumesSizeTotalGB *prometheus.Desc
	volumesSizeUsedGB  *prometheus.Desc
	sharesTotal        *prometheus.Desc
	sharesUsed         *prometheus.Desc
	sharesSizeTotalGB  *prometheus.Desc
	sharesSizeUsedGB   *prometheus.Desc
	timestamp          *prometheus.Desc
}

// QuotaRecords represents output of the quota.sh script, in yaml format, that parse openstack cli responses
type QuotaRecords struct {
	// All below values belong to the openstack project which is set in keystone_env.sh (input argument to quota.sh)
	CpusTotal          int64 `yaml:"cpus_total"`                // Total assigned VCpus
	CpusUsed           int64 `yaml:"cpus_used"`                 // Total used VCpus
	RamTotalGB         int64 `yaml:"ram_total_gbytes"`          // Total assigned RAM in gigabytes
	RamUsedGB          int64 `yaml:"ram_used_gbytes"`           // Total used RAM in gigabytes
	InstancesTotal     int64 `yaml:"instances_total"`           // Total assigned instances count
	InstancesUsed      int64 `yaml:"instances_used"`            // Total used instances count
	VolumesTotal       int64 `yaml:"volumes_total"`             // Total assigned volumes count
	VolumesUsed        int64 `yaml:"volumes_used"`              // Total used volumes count
	VolumesSizeTotalGB int64 `yaml:"volumes_size_total_gbytes"` // Total assigned volumes size in gigabytes
	VolumesSizeUsedGB  int64 `yaml:"volumes_size_used_gbytes"`  // Total used volumes size in gigabytes
	SharesTotal        int64 `yaml:"shares_total"`              // Total assigned shares count
	SharesUsed         int64 `yaml:"shares_used"`               // Total used shares count
	SharesSizeTotalGB  int64 `yaml:"shares_size_total_gbytes"`  // Total assigned shares size in gigabytes
	SharesSizeUsedGB   int64 `yaml:"shares_size_used_gbytes"`   // Total used shares size in gigabytes
}

func NewExporter(namespace, quotaScriptPath, keystoneEnvFile string) *Exporter {
	return &Exporter{
		quotaScriptPath: quotaScriptPath,
		keystoneEnvFile: keystoneEnvFile,
		scrapeFailures:  prometheus.NewCounter(prometheus.CounterOpts{}),
		cpusTotal: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "cpus_total"),
			"Total assigned VCpus",
			nil,
			nil),
		cpusUsed: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "cpus_used"),
			"Total used VCpus",
			nil,
			nil),
		ramTotalGB: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "ram_total_gbytes"),
			"Total assigned RAM in gigabytes",
			nil,
			nil),
		ramUsedGB: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "ram_used_gbytes"),
			"Total used RAM in gigabytes",
			nil,
			nil),
		instancesTotal: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "instances_total"),
			"Total assigned instances count",
			nil,
			nil),
		instancesUsed: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "instances_used"),
			"Total used instances count",
			nil,
			nil),
		volumesTotal: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "volumes_total"),
			"Total assigned volumes count",
			nil,
			nil),
		volumesUsed: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "volumes_used"),
			"Total used volumes count",
			nil,
			nil),
		volumesSizeTotalGB: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "volumes_size_total_gbytes"),
			"Total assigned volumes size in gigabytes",
			nil,
			nil),
		volumesSizeUsedGB: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "volumes_size_used_gbytes"),
			"Total used volumes size in gigabytes",
			nil,
			nil),
		sharesTotal: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "shares_total"),
			"Total assigned shares count",
			nil,
			nil),
		sharesUsed: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "shares_used"),
			"Total used shares count",
			nil,
			nil),
		sharesSizeTotalGB: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "shares_size_total_gbytes"),
			"Total assigned shares size in gigabytes",
			nil,
			nil),
		sharesSizeUsedGB: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "shares_size_used_gbytes"),
			"Total used shares size in gigabytes",
			nil,
			nil),
		timestamp: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "timestamp"),
			"Metric timestamp",
			nil,
			nil),
	}
}

// Describe sends the super-set of all possible descriptors of metrics
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.cpusTotal
	ch <- e.cpusUsed
	ch <- e.ramTotalGB
	ch <- e.ramUsedGB
	ch <- e.instancesTotal
	ch <- e.instancesUsed
	ch <- e.volumesTotal
	ch <- e.volumesUsed
	ch <- e.volumesSizeTotalGB
	ch <- e.volumesSizeUsedGB
	ch <- e.sharesTotal
	ch <- e.sharesUsed
	ch <- e.sharesSizeTotalGB
	ch <- e.sharesSizeUsedGB
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

// quotaScriptPath: quota.sh, keystoneEnvFile: see quota.sh for details
func run(quotaScriptPath, keystoneEnvFile string) (QuotaRecords, error) {
	start := time.Now()
	var record QuotaRecords
	command := exec.Command("/bin/bash", quotaScriptPath, keystoneEnvFile)
	stdout, err := command.Output()
	if err != nil {
		msg := fmt.Sprintf("%s\n %v %v %v", "Failed to run bash script:", command, stdout, err)
		log.Println(msg)
		return record, errors.New(msg)
	}
	// Unmarshall bash script output.
	if err2 := yaml.Unmarshal(stdout, &record); err2 != nil {
		msg := fmt.Sprintf("%s\n %v %v %v", "Error parsing YAML file:", command, stdout, err)
		log.Println(msg)
		return record, errors.New(msg)
	}
	elapsed := time.Since(start)
	fmt.Printf("%v\n", string(stdout))
	log.Printf("%s took %s\n", "quota.sh", elapsed)
	return record, nil
}

// helper function which collects exporter attributes
func (e *Exporter) collect(ch chan<- prometheus.Metric) error {
	// extract records
	records, err := run(e.quotaScriptPath, e.keystoneEnvFile)
	if err != nil {
		return err
	}
	timestamp := time.Now().Unix()
	ch <- prometheus.MustNewConstMetric(e.cpusTotal, prometheus.CounterValue, float64(records.CpusTotal))
	ch <- prometheus.MustNewConstMetric(e.cpusUsed, prometheus.CounterValue, float64(records.CpusUsed))
	ch <- prometheus.MustNewConstMetric(e.ramTotalGB, prometheus.CounterValue, float64(records.RamTotalGB))
	ch <- prometheus.MustNewConstMetric(e.ramUsedGB, prometheus.CounterValue, float64(records.RamUsedGB))
	ch <- prometheus.MustNewConstMetric(e.instancesTotal, prometheus.CounterValue, float64(records.InstancesTotal))
	ch <- prometheus.MustNewConstMetric(e.instancesUsed, prometheus.CounterValue, float64(records.InstancesUsed))
	ch <- prometheus.MustNewConstMetric(e.volumesTotal, prometheus.CounterValue, float64(records.VolumesTotal))
	ch <- prometheus.MustNewConstMetric(e.volumesUsed, prometheus.CounterValue, float64(records.VolumesUsed))
	ch <- prometheus.MustNewConstMetric(e.volumesSizeTotalGB, prometheus.CounterValue, float64(records.VolumesSizeTotalGB))
	ch <- prometheus.MustNewConstMetric(e.volumesSizeUsedGB, prometheus.CounterValue, float64(records.VolumesSizeUsedGB))
	ch <- prometheus.MustNewConstMetric(e.sharesTotal, prometheus.CounterValue, float64(records.SharesTotal))
	ch <- prometheus.MustNewConstMetric(e.sharesUsed, prometheus.CounterValue, float64(records.SharesUsed))
	ch <- prometheus.MustNewConstMetric(e.sharesSizeTotalGB, prometheus.CounterValue, float64(records.SharesSizeTotalGB))
	ch <- prometheus.MustNewConstMetric(e.sharesSizeUsedGB, prometheus.CounterValue, float64(records.SharesSizeUsedGB))
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
	flag.StringVar(&namespace, "namespace", "openstack", "namespace to use for exporter")
	var scriptPath string
	flag.StringVar(&scriptPath, "script", "/data/cmsweb-exporters/quota.sh", "bash script file name")
	var envFile string
	flag.StringVar(&envFile, "env", "/etc/secrets/keystone_env.sh", "See details of keystone_env.sh in quota.sh")
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

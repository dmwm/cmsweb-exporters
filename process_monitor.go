package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	logs "github.com/sirupsen/logrus"
)

func findProcess(pat, prefix string) (int, string) {
	cmd := fmt.Sprintf("ps auxw | grep \"%s\" | grep -v grep", pat)
	out, err := exec.Command("sh", "-c", cmd).Output()
	if err != nil {
		logs.WithFields(logs.Fields{
			"Error":   err,
			"Pattern": pat,
		}).Error("Unable to find process pattern")
		return 0, prefix
	}
	matched, _ := regexp.MatchString(pat, fmt.Sprintf("%s", out))
	if matched {
		pieces := strings.Split(string(out), " ")
		pid, err := strconv.Atoi(pieces[1]) // pid
		if err != nil {
			logs.WithFields(logs.Fields{
				"Error": err,
			}).Error("Unable to parse process PID")
			return 0, prefix
		}
		if prefix == "" {
			prefix = fmt.Sprintf("process_%d", pid)
		}
	}
	return 0, prefix
}

func checkProcess(pat string) bool {
	cmd := fmt.Sprintf("ps auxw | grep \"%s\" | grep -v grep", pat)
	out, err := exec.Command("sh", "-c", cmd).Output()
	if err != nil {
		logs.WithFields(logs.Fields{
			"Error":   err,
			"Pattern": pat,
		}).Error("Unable to find process pattern")
		return false
	}
	matched, _ := regexp.MatchString(pat, fmt.Sprintf("%s", out))
	if matched {
		return true
	}
	return false
}

// helper function to start underlying process_exporter
// for pipe usage see https://zupzup.org/io-pipe-go/
func start(pid int, prefix string, pw *io.PipeWriter) {
	cmd := exec.Command("process_exporter", "-pid", fmt.Sprintf("%d", pid), "-prefix", prefix)
	cmd.Stdout = pw
	cmd.Stderr = pw
	err := cmd.Run()
	if err != nil {
		logs.WithFields(logs.Fields{
			"Error":  err,
			"pid":    pid,
			"prefix": prefix,
		}).Error("unable to start process_exporter")
		return
	}
}

func monitor(interval int64, pat, prefix string) {
	pr, pw := io.Pipe()
	defer pr.Close()
	defer pw.Close()
	go func() {
		if _, err := io.Copy(os.Stdout, pr); err != nil {
			logs.WithFields(logs.Fields{
				"Error": err,
			}).Error("Unable to pipe das2go output")
			return
		}
	}()
	// check or start process_exporter for given PID
	for {
		pid, prefix := findProcess(pat, prefix)
		status := checkProcess(pat)
		if !status {
			logs.WithFields(logs.Fields{
				"pattern": pat,
				"status":  status,
			}).Warn("Process is not running, re-starting ...")
			start(pid, prefix, pw)
		}
		sleep := time.Duration(interval) * time.Second
		time.Sleep(sleep)
	}
}

func main() {
	var pat string
	flag.StringVar(&pat, "pat", "", "Process pattern to monitor")
	var prefix string
	flag.StringVar(&prefix, "prefix", "", "Process prefix to use")
	var interval int64
	flag.Int64Var(&interval, "interval", 10, "Monitoring interval")
	flag.Parse()
	monitor(interval, pat, prefix)
}

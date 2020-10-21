package main

// cmsweb-ping - Go implementation of ping functionality for cmsweb services based on hmac
//
// Copyright (c) 2015-2016 - Valentin Kuznetsov <vkuznet@gmail.com>
//

import (
	"crypto/hmac"
	"crypto/sha1"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"os"
)

func main() {
	var url string
	flag.StringVar(&url, "url", "", "service url")
	var authz string
	flag.StringVar(&authz, "authz", "", "authz file")
	var verbose int
	flag.IntVar(&verbose, "verbose", 0, "verbose level")
	flag.Parse()
	res := run(url, authz, verbose)
	fmt.Println(res)
}

func run(rurl, authz string, verbose int) string {
	req, err := http.NewRequest("GET", rurl, nil)
	headers := make(map[string]string)
	headers["cms-auth-status"] = "OK"
	headers["cms-authn-method"] = "PingMonitor"
	headers["cms-authn-login"] = "ping-monitor"
	headers["cms-authn-name"] = "Ping Monitor"
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// sorted list of cms headers
	hkeys := []string{"cms-authn-login", "cms-authn-method", "cms-authn-name"}
	var prefix, suffix string
	for _, key := range hkeys {
		if val, ok := headers[key]; ok {
			prefix += fmt.Sprintf("h%xv%x", len(key), len(val))
			suffix += fmt.Sprintf("%s%s", key, val)
		}
	}
	// read hkey from given file
	hkey, err := ioutil.ReadFile(authz)
	if err != nil {
		fmt.Printf("Unable to read, file: %s, error: %v\n", authz, err)
		os.Exit(1)
	}

	value := []byte(fmt.Sprintf("%s#%s", prefix, suffix))
	sha1hex := hmac.New(sha1.New, hkey)
	sha1hex.Write(value)
	hmacValue := fmt.Sprintf("%x", sha1hex.Sum(nil))
	req.Header.Set("cms-authn-hmac", hmacValue)
	req.Header.Set("Accept", "*/*")

	if verbose > 0 {
		dump, err := httputil.DumpRequestOut(req, true)
		if err == nil {
			fmt.Println("request: ", string(dump))
		}
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Unable to get response from %s, error: %s", rurl, err)
		os.Exit(1)
	}
	if verbose > 0 {
		dump, err := httputil.DumpResponse(resp, true)
		if err == nil {
			fmt.Println("response:", string(dump))
		}
	}
	return resp.Status
}

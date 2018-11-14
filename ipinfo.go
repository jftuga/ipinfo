/*

ipinfo.go

Query https://ipinfo.io for IP address info including geographic location when given IP address, host name or URL
Multiple arguments can be given on cmd line

Example:
ipinfo gatech.edu clemson.edu sc.edu utk.edu auburn.edu unc.edu www.uky.edu ufl.edu olemiss.edu www.virginia.edu louisiana.edu umiami.edu missouri.edu utexas.edu texastech.edu

To compile:
go build -ldflags="-s -w" ipinfo.go

MIT License; Copyright (c) 2018 John Taylor
Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:
The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.
THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

*/

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
)

func main() {
	time_start := time.Now()
	workers := flag.Int("workers", 30, "number of simultaneous workers")
	flag.Parse()
	convertedArgs := convertArgs(flag.Args())
	ipAddrs, reverseIP := runDNS(*workers, convertedArgs)

	ipInfo := runIpInfo(*workers, ipAddrs)
	outputTable(ipInfo, reverseIP)
	elapsed := time.Since(time_start)
	fmt.Printf("\nelapsed time: %s\n", elapsed)
}

func convertArgs(rawArgs []string) []string {
	cleanArgs := []string{}
	for entry := range rawArgs {
		if strings.Contains(rawArgs[entry], "://") { // url
			slots := strings.SplitN(rawArgs[entry], "/", 4)
			cleanArgs = append(cleanArgs, slots[2])
		} else if strings.Contains(rawArgs[entry], "@") { // email
			slots := strings.SplitN(rawArgs[entry], "@", 2)
			cleanArgs = append(cleanArgs, slots[1])
		} else { // just a host name or IP address
			cleanArgs = append(cleanArgs, rawArgs[entry])
		}
	}
	return cleanArgs
}

func outputTable(ipInfo []ipInfoResult, reverseIP map[string]string) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Input", "IP", "Hostname", "Org", "City", "Region", "Country", "Loc"})
	for i, _ := range ipInfo {
		if strings.Contains(ipInfo[i].Ip, ":") { // skip IPv6
			continue
		}
		row := []string{reverseIP[ipInfo[i].Ip], ipInfo[i].Ip, ipInfo[i].Hostname, ipInfo[i].Org, ipInfo[i].City, ipInfo[i].Region, ipInfo[i].Country, ipInfo[i].Loc}
		table.Append(row)
	}
	table.Render()
}

/* https://stackoverflow.com/a/15323988/452281 */
func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

/*
runDNS will use N number of workers to concurrently query a DNS server for all
entries in the hostnames slice

Args:
	workers: the number of threads to use

	hostnames: a slice containing the hostnames to look up

	Returns:
		a slice containing IP addresses for all hostnames
		a map with key=ip, value=hostname
*/
func runDNS(workers int, hostnames []string) ([]string, map[string]string) {
	ipm, errors := resolveAllDNS(workers, hostnames)
	var ipAddrs []string
	ipAddrs = nil

	var reverseIP map[string]string
	reverseIP = make(map[string]string)

	for _, val := range ipm {
		for _, ip := range val.addresses {
			if stringInSlice(ip, ipAddrs) { // skip duplicate IP addresses
				continue
			}
			ipAddrs = append(ipAddrs, ip)
			reverseIP[ip] = val.hostname
		}
	}
	if len(errors) > 0 {
		var errBuilder strings.Builder
		for _, err := range errors {
			errBuilder.WriteString(err.Error())
		}
		fmt.Printf("\n%s\n\n", errBuilder.String())
	}
	return ipAddrs, reverseIP
}

func runIpInfo(workers int, ipAddrs []string) []ipInfoResult {
	allDnsResponses := resolveAllIpInfo(workers, ipAddrs)
	return allDnsResponses
}

type dnsResponse struct {
	hostname  string
	addresses []string
	err       error
}

func resolveAllDNS(workers int, hostnames []string) ([]dnsResponse, []error) {
	workCh := make(chan string)
	dnsResponseCh := make(chan dnsResponse)
	defer close(dnsResponseCh)

	for i := 0; i < workers; i++ {
		go workDNS(workCh, dnsResponseCh)
	}

	allDnsReplies := []dnsResponse{}
	waitingFor := 0
	errors := []error{}

	for len(hostnames) > 0 || waitingFor > 0 {
		sendCh := workCh
		host := ""
		if len(hostnames) > 0 {
			host = hostnames[0]
		} else {
			sendCh = nil
		}
		select {
		case sendCh <- host:
			waitingFor++
			hostnames = hostnames[1:]

		case dnsResponse := <-dnsResponseCh:
			waitingFor--
			if dnsResponse.err != nil {
				errors = append(errors, dnsResponse.err)
			} else {
				allDnsReplies = append(allDnsReplies, dnsResponse)
			}
		}
	}
	return allDnsReplies, errors
}

func workDNS(workCh chan string, dnsResponseCh chan dnsResponse) {
	for hostname := range workCh {
		addresses, err := net.LookupHost(hostname)
		dnsResponseCh <- dnsResponse{
			hostname:  hostname,
			addresses: addresses,
			err:       err,
		}
	}
}

type ipInfoResult struct {
	Ip       string
	Hostname string
	City     string
	Region   string
	Country  string
	Loc      string
	Postal   string
	Org      string
	Distance float32
	ErrMsg   error
}

func resolveAllIpInfo(workers int, ipAddrs []string) []ipInfoResult {
	workCh := make(chan string)
	resultsCh := make(chan ipInfoResult)
	defer close(resultsCh)

	for i := 0; i < workers; i++ {
		go workIpInfoLookup(workCh, resultsCh)
	}

	var iir []ipInfoResult
	waitingFor := 0

	for len(ipAddrs) > 0 || waitingFor > 0 {
		sendCh := workCh
		ip := ""
		if len(ipAddrs) > 0 {
			ip = ipAddrs[0]
		} else {
			sendCh = nil
		}

		select {
		case sendCh <- ip:
			waitingFor++
			ipAddrs = ipAddrs[1:]

		case result := <-resultsCh:
			waitingFor--
			iir = append(iir, result)

		}
	}
	return iir
}

func workIpInfoLookup(workCh chan string, resultCh chan ipInfoResult) {
	for ip := range workCh {
		url := "https://ipinfo.io/" + ip + "/json"
		resp, err := http.Get(url)
		if err != nil {
			fmt.Println("error: ", err)
			return
		}
		defer resp.Body.Close()

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			fmt.Println("error: ", err)
			return
		}
		var obj ipInfoResult
		json.Unmarshal(body, &obj)
		resultCh <- obj
	}
}

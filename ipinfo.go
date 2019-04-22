/*

ipinfo.go

Query https://ipinfo.io for IP address info including geographic location when given IP address, host name or URL
Multiple arguments can be given on cmd line

Example:
ipinfo gatech.edu clemson.edu sc.edu utk.edu auburn.edu unc.edu www.uky.edu ufl.edu olemiss.edu www.virginia.edu louisiana.edu umiami.edu missouri.edu utexas.edu texastech.edu

To compile:
go build -ldflags="-s -w" ipinfo.go

MIT License; Copyright (c) 2019 John Taylor
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
	"math"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
)

const version = "1.1.0"

// For a given DNS query, one hostname can return multiple IP addresses
type dnsResponse struct {
	hostname  string
	addresses []string
	err	   error
}

// This is the format returned by: https://ipinfo.io/w.x.y.z/json
type ipInfoResult struct {
	Ip	   string
	Hostname string
	City	 string
	Region   string
	Country  string
	Loc	  string
	Postal   string
	Org	  string
	Distance float32
	ErrMsg   error
}

/*
main will parse command line arguments, get the IP addresses for all command line args,
retreive the IP info for each of these IP addresses, and then output the results
*/
func main() {
	timeStart := time.Now()

	workers := flag.Int("t", 30, "number of simultaneous threads")
	tableAutoMerge := flag.Bool("m", false, "merge identical hosts")
	versionFlag := flag.Bool("v", false, "display program version and then exit")
    externalOnlyFlag := flag.Bool("x", false, "only display your external IP and then exit")
    wrapFlag := flag.Bool("w", false, "wrap output to better fit the screen width")

	flag.Parse()
	if *versionFlag {
		fmt.Println("version:", version)
		return
	}

	localIpInfo := callRemoteService("")
	args := flag.Args()
    if *externalOnlyFlag {
        fmt.Println(localIpInfo.Ip)
        return
    }
	if len(flag.Args()) == 0 {
		args = append(args,localIpInfo.Ip)
	}

	convertedArgs := truncateArgParts(args)
	ipAddrs, reverseIP := runDNS(*workers, convertedArgs)
	ipInfo := resolveAllIpInfo(*workers, ipAddrs)

	outputTable(ipInfo, reverseIP, localIpInfo.Loc, *tableAutoMerge, *wrapFlag)

	elapsed := time.Since(timeStart)
	fmt.Println("\n")
	fmt.Printf("your IP addr : %v\n", localIpInfo.Ip)
	fmt.Printf("your location: %v\n", localIpInfo.Loc)
	fmt.Printf("elapsed time : %v\n", elapsed)
}

/*
truncateArgParts will truncate a URL or email address to just the hostname

Args:
	rawArgs: a slice of entries that can be any of the following: URL, email, hostname, IP address

Returns:
	the same slice with entries shortened to just hostname or IP address
*/
func truncateArgParts(rawArgs []string) []string {
	truncateArgs := []string{}
	for entry := range rawArgs {
		if strings.Contains(rawArgs[entry], "://") { // url
			slots := strings.SplitN(rawArgs[entry], "/", 4)
			truncateArgs = append(truncateArgs, slots[2])
		} else if strings.Contains(rawArgs[entry], "@") { // email
			slots := strings.SplitN(rawArgs[entry], "@", 2)
			truncateArgs = append(truncateArgs, slots[1])
		} else { // just a host name or IP address
			truncateArgs = append(truncateArgs, rawArgs[entry])
		}
	}
	return truncateArgs
}

/*
latlon2coord converts a string such as "36.0525,-79.107" to a tuple of floats

Args:
	latlon: a string in "lat, lon" format

Returns:
	a tuple in (float64, float64) format
*/
func latlon2coord(latlon string) (float64, float64) {
	slots := strings.Split(latlon, ",")
	lat, err := strconv.ParseFloat(slots[0], 64)
	if err != nil {
		fmt.Println("Error converting latitude to float for:", latlon)
	}
	lon, err := strconv.ParseFloat(slots[1], 64)
	if err != nil {
		fmt.Println("Error converting longitude to float for:", latlon)
	}
	return lat, lon
}

// adapted from: https://gist.github.com/cdipaolo/d3f8db3848278b49db68
// haversin(θ) function
func hsin(theta float64) float64 {
	return math.Pow(math.Sin(theta/2), 2)
}

// HaversineDistance returns the distance (in miles) between two points of
//	 a given longitude and latitude relatively accurately (using a spherical
//	 approximation of the Earth) through the Haversin Distance Formula for
//	 great arc distance on a sphere with accuracy for small distances
//
// point coordinates are supplied in degrees and converted into rad. in the func
//
// http://en.wikipedia.org/wiki/Haversine_formula
func HaversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	// convert to radians
	// must cast radius as float to multiply later
	var la1, lo1, la2, lo2, r float64

	piRad := math.Pi / 180
	la1 = lat1 * piRad
	lo1 = lon1 * piRad
	la2 = lat2 * piRad
	lo2 = lon2 * piRad

	r = 6378100 // Earth radius in METERS

	// calculate
	h := hsin(la2-la1) + math.Cos(la1)*math.Cos(la2)*hsin(lo2-lo1)

	meters := 2 * r * math.Asin(math.Sqrt(h))
	miles := meters / 1609.344
	return miles
}

/*
outputTable outputs a table with IP info for each command line arg
It also computes the distance from the local IP address to the remote IP address

Args:
	ipInfo: a slice of ipInfoResult stucts containing the IP info metadata for each command line argument

	reverseIP: a map where key=IP address, value=hostname

	loc: the local IP addresses location in this format: "lat, lon"

	merge: if -merge was passed in as a command line parameter
*/
func outputTable(ipInfo []ipInfoResult, reverseIP map[string]string, loc string, merge bool, wrap bool) {
	var allRows [][]string

	var distanceStr = ""

	for i, _ := range ipInfo {
		if strings.Contains(ipInfo[i].Ip, ":") { // skip IPv6
			continue
		}
		if ipInfo[i].Loc == "37.7510,-97.8220" || len(ipInfo[i].Loc) == 0 { // https://en.wikipedia.org/wiki/Cheney_Reservoir#IP_Address_Geo_Location
			ipInfo[i].Loc = "N/A"
			ipInfo[i].City = "N/A"
			ipInfo[i].Region = "N/A"
			distanceStr = "N/A"
		} else {
			lat1, lon1 := latlon2coord(loc)
			lat2, lon2 := latlon2coord(ipInfo[i].Loc)
			//fmt.Printf("loc1: %v %v\nloc2: %v %v\n", lat1, lon1, lat2, lon2)
			miles := HaversineDistance(lat1, lon1, lat2, lon2)
			distanceStr = fmt.Sprintf("%.2f", miles)
		}
		row := []string{reverseIP[ipInfo[i].Ip], ipInfo[i].Ip, ipInfo[i].Hostname, ipInfo[i].Org, ipInfo[i].City, ipInfo[i].Region, ipInfo[i].Country, ipInfo[i].Loc, distanceStr}
		allRows = append(allRows, row)
	}

	// sort rows by input hostname
	sort.Slice(allRows, func(a, b int) bool {
		return allRows[a][0] < allRows[b][0]
	})

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Input", "IP", "Hostname", "Org", "City", "Region", "Country", "Loc", "Distance"})
	if merge == true {
		table.SetAutoMergeCells(true)
	}
    if wrap {
        table.SetAutoWrapText(true)
    } else {
        table.SetAutoWrapText(false)
    }
	table.AppendBulk(allRows)
	table.Render()
}

/*
stringInSlice checks to see if a string is located in the given slice
See also: https://stackoverflow.com/a/15323988/452281

Args:
	a: the string to search for

	list: a slice of strings

Returns:
	true if a is in list, false otherwise
*/
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

/*
resolveAllDNS returns a slice containing all IP addresses for each given hostname
The concurrency is limited by the workers values

Args:
	workers: the number of concurrent go routines to execute

	hostnames: a slice containing all hostnames (or IP addresses)

Returns:
	a slice containing the IP info for each given IP address
*/
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

/*
workDNS

Args:
	workCh:

	dnsResponseCh:
*/
func workDNS(workCh chan string, dnsResponseCh chan dnsResponse) {
	for hostname := range workCh {
		addresses, err := net.LookupHost(hostname)
		dnsResponseCh <- dnsResponse{
			hostname:  hostname,
			addresses: addresses,
			err:	   err,
		}
	}
}

/*
resolveAllIpInfo returns a slice containing all IP info for each IP given in ipAddrs
The concurrency is limited by the workers values

Args:
	workers: the number of concurrent go routines to execute

	ipAddrs: a slice of IP addresses

Returns:
	a slice containing the IP info for each given IP address
*/
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

/*
callRemoteService issues a web query to ipinfo.io
The JSON result is converted to an ipInfoResult struct
Args:
	ip: an IPv4 address

Returns:
	an ipInfoResult struct containing the information returned by the service
*/
func callRemoteService(ip string) ipInfoResult {
	var obj ipInfoResult

	api := "/json"
	if 0 == len(ip) {
		api = "json"
	}
	url := "https://ipinfo.io/" + ip + api
	resp, err := http.Get(url)
	if err != nil {
		fmt.Println("error: ", err)
		return obj
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("error: ", err)
		return obj
	}

	if strings.Contains(string(body), "Rate limit exceeded") {
		fmt.Println("\nError for:", url)
		fmt.Println(string(body))
		os.Exit(1)
	}

	json.Unmarshal(body, &obj)
	return obj
}

/*
workIpInfoLookup

Args:
	workCh:

	resultCh:
*/
func workIpInfoLookup(workCh chan string, resultCh chan ipInfoResult) {
	for ip := range workCh {
		obj := callRemoteService(ip)
		resultCh <- obj
	}
}

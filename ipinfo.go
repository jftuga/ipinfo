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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
)

const pgmVersion string = "1.3.0"
const pgmUrl string = "https://github.com/jftuga/ipinfo"

// dnsResponse represents the result of a DNS query.
// For a given DNS query, one hostname can return multiple IP addresses.
type dnsResponse struct {
	hostname  string
	addresses []string
	err       error
}

// ipInfoResult represents the response format returned by https://ipinfo.io/w.x.y.z/json.
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

// main parses command line arguments, gets the IP addresses for all command line args,
// retrieves the IP info for each of these IP addresses, and then outputs the results.
func main() {
	timeStart := time.Now()

	workers := flag.Int("t", 30, "number of simultaneous threads")
	tableAutoMerge := flag.Bool("m", false, "merge identical hosts")
	versionFlag := flag.Bool("v", false, "display program version and then exit")
	externalOnlyFlag := flag.Bool("x", false, "only display your external IP and then exit")
	wrapFlag := flag.Bool("w", false, "wrap output to better fit the screen width")
	oneRowFlag := flag.Bool("1", false, "display each entry on one row only")

	flag.Parse()
	if *versionFlag {
		fmt.Println("version:", pgmVersion)
		fmt.Println(pgmUrl)
		return
	}

	localIpInfo := callRemoteService("")
	args := flag.Args()
	if *externalOnlyFlag {
		fmt.Println(localIpInfo.Ip)
		return
	}
	if len(flag.Args()) == 0 {
		args = append(args, localIpInfo.Ip)
	}

	convertedArgs := truncateArgParts(args)
	ipAddrs, ipToHostnames := runDNS(*workers, convertedArgs)
	ipInfo := resolveAllIpInfo(*workers, ipAddrs)

	outputTable(ipInfo, ipToHostnames, localIpInfo.Loc, *tableAutoMerge, *wrapFlag, *oneRowFlag)

	elapsed := time.Since(timeStart)
	fmt.Println("\n")
	fmt.Printf("your IP addr : %v\n", localIpInfo.Ip)
	fmt.Printf("your location: %v\n", localIpInfo.Loc)
	fmt.Printf("elapsed time : %v\n", elapsed)
}

// truncateArgParts truncates a URL or email address to just the hostname.
//
// It takes a slice of entries that can be any of the following: URL, email, hostname, IP address
// and returns the same slice with entries shortened to just hostname or IP address.
func truncateArgParts(rawArgs []string) []string {
	v4re := regexp.MustCompile(`(?:[0-9]{1,3}\.){3}[0-9]{1,3}`)
	truncateArgs := []string{}
	for entry := range rawArgs {
		if strings.Contains(rawArgs[entry], "://") { // url
			slots := strings.SplitN(rawArgs[entry], "/", 4)
			truncateArgs = append(truncateArgs, slots[2])
			continue
		} else if strings.Contains(rawArgs[entry], "@") { // email
			slots := strings.SplitN(rawArgs[entry], "@", 2)
			truncateArgs = append(truncateArgs, slots[1])
			continue
		} else { // either a host name or IP address
			if v4re.Match([]byte(rawArgs[entry])) && strings.Contains(rawArgs[entry], ":") {
				// v4 address with port
				c := strings.Index(rawArgs[entry], ":")
				truncateArgs = append(truncateArgs, rawArgs[entry][0:c])
				continue
			}
			truncateArgs = append(truncateArgs, rawArgs[entry])
		}
	}
	return truncateArgs
}

// latlon2coord converts a string such as "36.0525,-79.107" to a tuple of floats.
//
// It takes a string in "lat, lon" format and returns a tuple in (float64, float64) format.
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

// hsin calculates the haversine(θ) function.
//
// It is a helper function used in the HaversineDistance calculation.
func hsin(theta float64) float64 {
	return math.Pow(math.Sin(theta/2), 2)
}

// HaversineDistance returns the distance (in miles) between two points of
// a given longitude and latitude relatively accurately (using a spherical
// approximation of the Earth) through the Haversine Distance Formula for
// great arc distance on a sphere with accuracy for small distances.
//
// Point coordinates are supplied in degrees and converted into rad. in the function.
// See http://en.wikipedia.org/wiki/Haversine_formula for more information.
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

// outputTable outputs a table with IP info for each command line arg.
// It also computes the distance from the local IP address to the remote IP address.
//
// Parameters:
//   - ipInfo: a slice of ipInfoResult structs containing the IP info metadata for each unique IP address
//   - ipToHostnames: a map where key=IP address, value=slice of original hostnames that resolved to this IP
//   - loc: the local IP addresses location in this format: "lat, lon"
//   - merge: if -merge was passed in as a command line parameter
//   - wrap: wrap long lines
//   - oneRow: display each entry on one row only
func outputTable(ipInfo []ipInfoResult, ipToHostnames map[string][]string, loc string, merge, wrap, oneRow bool) {
	var allRows [][]string

	var distanceStr = ""
	var row []string

	// Iterate through the fetched IP info (unique IPs)
	for _, info := range ipInfo {
		if strings.Contains(info.Ip, ":") { // skip IPv6
			continue
		}

		// Skip results that had errors during fetch (ErrMsg will be non-nil)
		if info.ErrMsg != nil {
			continue
		}

		// Determine location and distance once per unique IP
		currentLoc := info.Loc
		currentCity := info.City
		currentRegion := info.Region
		if currentLoc == "37.7510,-97.8220" || len(currentLoc) == 0 { // https://en.wikipedia.org/wiki/Cheney_Reservoir#IP_Address_Geo_Location
			currentLoc = "N/A"
			currentCity = "N/A"
			currentRegion = "N/A"
			distanceStr = "N/A"
		} else {
			lat1, lon1 := latlon2coord(loc)
			lat2, lon2 := latlon2coord(currentLoc)
			miles := HaversineDistance(lat1, lon1, lat2, lon2)
			distanceStr = fmt.Sprintf("%.2f", miles)
		}
		locParts := []string{"N/A", "N/A"}
		if currentLoc != "N/A" {
			locParts = strings.Split(currentLoc, ",")
		}

		// Find all original hostnames that resolved to this IP
		hostnamesForThisIP := ipToHostnames[info.Ip]
		if hostnamesForThisIP == nil { // Should not happen if logic is correct, but safe check
			fmt.Fprintf(os.Stderr, "Warning: No hostname found for IP %s\n", info.Ip)
			continue
		}

		// Create a row for each original hostname associated with this IP
		for _, hostname := range hostnamesForThisIP {
			if oneRow {
				row = []string{hostname, info.Ip, info.Hostname, info.Org, currentCity, currentRegion, info.Country, currentLoc, distanceStr}
			} else {
				row = []string{fmt.Sprintf("%v\n%v", hostname, info.Ip), fmt.Sprintf("%v\n%v", info.Hostname, info.Org), fmt.Sprintf("%v\n%v\n%v", currentCity, currentRegion, info.Country), fmt.Sprintf("%v\n%v", locParts[0], locParts[1]), distanceStr}
			}
			allRows = append(allRows, row)
		}
	}

	// sort rows by input hostname (first part of the first column)
	sort.Slice(allRows, func(a, b int) bool {
		hostA := strings.Split(allRows[a][0], "\n")[0]
		hostB := strings.Split(allRows[b][0], "\n")[0]
		return hostA < hostB
	})

	table := tablewriter.NewWriter(os.Stdout)
	if oneRow {
		table.SetHeader([]string{"Input", "IP", "Hostname", "Org", "City", "Region", "Country", "Lat/Lon", "Dist"})
	} else {
		table.SetHeader([]string{"Input/IP", "Hostname/Org", "City/Region/Country", "Lat/Lon", "Dist"})
	}
	if merge == true {
		table.SetAutoMergeCells(true)
	}
	if wrap {
		table.SetAutoWrapText(true)
	} else {
		table.SetAutoWrapText(false)
	}
	if len(allRows) == 0 {
		fmt.Println("\nNo results found.")
		return
	}
	table.AppendBulk(allRows)
	table.Render()
}

// stringInSlice checks to see if a string is located in the given slice.
// See also: https://stackoverflow.com/a/15323988/452281
//
// Parameters:
//   - a: the string to search for
//   - list: a slice of strings
//
// Returns:
//   - true if a is in list, false otherwise
func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

// runDNS uses N number of workers to concurrently query a DNS server for all
// entries in the hostnames slice.
//
// Parameters:
//   - workers: the number of threads to use
//   - hostnames: a slice containing the hostnames to look up
//
// Returns:
//   - a slice containing *unique* IP addresses for all hostnames
//   - a map with key=ip, value=list of hostnames that resolved to this IP
func runDNS(workers int, hostnames []string) ([]string, map[string][]string) {
	ipm, errors := resolveAllDNS(workers, hostnames)
	var ipAddrs []string // Stores unique IPs found
	ipAddrs = nil

	var ipToHostnames map[string][]string // Map IP -> list of hostnames
	ipToHostnames = make(map[string][]string)

	for _, val := range ipm { // val is dnsResponse {hostname, addresses, err}
		for _, ip := range val.addresses {
			// Append hostname to the list for this IP
			// Check if hostname is already in the list for this IP to avoid duplicates if LookupHost returns the same host multiple times (unlikely but possible)
			found := false
			for _, existingHost := range ipToHostnames[ip] {
				if existingHost == val.hostname {
					found = true
					break
				}
			}
			if !found {
				ipToHostnames[ip] = append(ipToHostnames[ip], val.hostname)
			}

			// Still track unique IPs for lookup efficiency
			if !stringInSlice(ip, ipAddrs) {
				ipAddrs = append(ipAddrs, ip)
			}
		}
	}
	if len(errors) > 0 {
		var errBuilder strings.Builder
		for _, err := range errors {
			errBuilder.WriteString(fmt.Sprintf("%s\n", err.Error()))
		}
		fmt.Fprintf(os.Stderr, "\nDNS Errors:\n%s\n\n", errBuilder.String())
	}
	return ipAddrs, ipToHostnames
}

// resolveAllDNS returns a slice containing all IP addresses for each given hostname.
// The concurrency is limited by the workers value using the "send all -> close -> receive all" pattern.
//
// Parameters:
//   - workers: the number of concurrent go routines to execute
//   - hostnames: a slice containing all hostnames (or IP addresses)
//
// Returns:
//   - a slice of dnsResponse structures (only for successful lookups with addresses)
//   - a slice of errors encountered during DNS resolution
func resolveAllDNS(workers int, hostnames []string) ([]dnsResponse, []error) {
	// Use send-all -> close -> receive-all pattern for worker coordination.
	workCh := make(chan string)
	dnsResponseCh := make(chan dnsResponse)
	defer close(dnsResponseCh) // Ensure response channel is closed eventually

	// Start workers
	actualWorkers := workers
	if len(hostnames) < workers {
		actualWorkers = len(hostnames) // Don't start more workers than needed
	}
	for i := 0; i < actualWorkers; i++ {
		go workDNS(workCh, dnsResponseCh)
	}

	// Send all hostnames to the workers
	for _, host := range hostnames {
		workCh <- host
	}
	close(workCh) // Signal workers that no more work is coming

	// Collect all results
	allDnsReplies := []dnsResponse{} // Initialize slice for successful replies
	errors := []error{}              // Initialize slice for errors
	numResultsExpected := len(hostnames)
	for i := 0; i < numResultsExpected; i++ {
		dnsResponse := <-dnsResponseCh
		if dnsResponse.err != nil {
			// Handle cases like "no such host" gracefully
			errors = append(errors, fmt.Errorf("DNS lookup failed for %s: %w", dnsResponse.hostname, dnsResponse.err))
		} else if len(dnsResponse.addresses) > 0 {
			// Only add if we got valid addresses
			allDnsReplies = append(allDnsReplies, dnsResponse)
		} else {
			// Handle cases where lookup succeeds but returns no addresses (less common)
			errors = append(errors, fmt.Errorf("DNS lookup for %s returned no addresses", dnsResponse.hostname))
		}
	}

	return allDnsReplies, errors
}

// workDNS is a worker function that performs DNS lookups for hostnames
// received through the workCh channel and sends results back through dnsResponseCh.
//
// Parameters:
//   - workCh: channel for receiving hostnames to look up
//   - dnsResponseCh: channel for sending back DNS lookup results
func workDNS(workCh chan string, dnsResponseCh chan dnsResponse) {
	for hostname := range workCh { // Reads until workCh is closed
		addresses, err := net.LookupHost(hostname)
		dnsResponseCh <- dnsResponse{
			hostname:  hostname,
			addresses: addresses,
			err:       err,
		}
	}
}

// resolveAllIpInfo returns a slice containing IP info for each IP address in ipAddrs.
// The concurrency is limited by the workers value.
//
// Parameters:
//   - workers: the number of concurrent go routines to execute
//   - ipAddrs: a slice of *unique* IP addresses
//
// Returns:
//   - a slice containing the IP info for each given IP address
func resolveAllIpInfo(workers int, ipAddrs []string) []ipInfoResult {
	if len(ipAddrs) == 0 {
		return []ipInfoResult{} // Return empty slice if no IPs to look up
	}

	workCh := make(chan string)
	resultsCh := make(chan ipInfoResult)
	defer close(resultsCh) // Close results channel when function exits

	// Start workers
	actualWorkers := workers
	if len(ipAddrs) < workers {
		actualWorkers = len(ipAddrs) // Don't start more workers than needed
	}
	for i := 0; i < actualWorkers; i++ {
		go workIpInfoLookup(workCh, resultsCh)
	}

	// Send work
	for _, ip := range ipAddrs {
		workCh <- ip
	}
	close(workCh) // Signal workers no more IPs are coming

	// Collect results
	var iir []ipInfoResult
	numResultsExpected := len(ipAddrs)
	for i := 0; i < numResultsExpected; i++ {
		result := <-resultsCh
		// Optionally check for errors within the result if callRemoteService sets them
		if result.ErrMsg != nil {
			// Print error but still include the result (it might have partial info or indicate the error type)
			fmt.Fprintf(os.Stderr, "Error fetching info for %s: %v\n", result.Ip, result.ErrMsg)
		}
		iir = append(iir, result)
	}

	return iir
}

// callRemoteService issues a web query to https://ipinfo.io.
// The JSON result is converted to an ipInfoResult struct.
//
// Parameters:
//   - ip: an IPv4 address (empty string for local IP address)
//
// Returns:
//   - an ipInfoResult struct containing the information returned by the service
func callRemoteService(ip string) ipInfoResult {
	var obj ipInfoResult
	obj.Ip = ip // Store the requested IP in the result object

	api := "/json"
	if 0 == len(ip) {
		api = "json" // Endpoint for self IP lookup
	}
	url := "https://ipinfo.io/" + ip + api

	// Use a client with a timeout
	client := http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		// fmt.Fprintf(os.Stderr, "HTTP GET error for %s: %v\n", url, err)
		obj.ErrMsg = fmt.Errorf("HTTP GET error: %w", err)
		return obj
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// fmt.Fprintf(os.Stderr, "HTTP error status for %s: %s\n", url, resp.Status)
		bodyBytes, _ := ioutil.ReadAll(resp.Body) // Try to read body for more info
		obj.ErrMsg = fmt.Errorf("HTTP error status: %s, Body: %s", resp.Status, string(bodyBytes))
		return obj
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		// fmt.Fprintf(os.Stderr, "Error reading response body for %s: %v\n", url, err)
		obj.ErrMsg = fmt.Errorf("error reading response body: %w", err)
		return obj
	}

	// Check for specific error messages from the API
	if strings.Contains(string(body), "Rate limit exceeded") {
		fmt.Fprintf(os.Stderr, "\nRate limit exceeded for: %s\n", url)
		obj.ErrMsg = fmt.Errorf("rate limit exceeded")
		return obj
	}
	if strings.Contains(string(body), "Wrong ip") || strings.Contains(string(body), "invalid IP address") {
		// fmt.Fprintf(os.Stderr, "API reported invalid IP for: %s\n", ip)
		obj.ErrMsg = fmt.Errorf("API reported invalid IP")
		// Set fields to indicate error or N/A
		obj.City = "Invalid IP"
		obj.Region = "N/A"
		obj.Country = "N/A"
		obj.Loc = "N/A"
		obj.Org = "N/A"
		obj.Hostname = "N/A"
		return obj
	}

	// Unmarshal the JSON response
	err = json.Unmarshal(body, &obj)
	if err != nil {
		// fmt.Fprintf(os.Stderr, "Error unmarshalling JSON for %s: %v\nBody: %s\n", url, err, string(body))
		obj.ErrMsg = fmt.Errorf("error unmarshalling JSON: %w", err)
		return obj
	}

	return obj
}

// workIpInfoLookup is a worker function that retrieves IP information
// for IP addresses received through the workCh channel and sends results
// back through resultCh.
//
// Parameters:
//   - workCh: channel for receiving IP addresses to look up
//   - resultCh: channel for sending back IP info lookup results
func workIpInfoLookup(workCh chan string, resultCh chan ipInfoResult) {
	for ip := range workCh { // Reads until workCh is closed
		obj := callRemoteService(ip)
		resultCh <- obj
	}
}

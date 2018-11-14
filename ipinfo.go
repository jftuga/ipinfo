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
	"math"
	"net"
	"net/http"
	"os"
	"strconv"
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

	localIpInfo := callRemoteService("")
	outputTable(ipInfo, reverseIP, localIpInfo.Loc)

	elapsed := time.Since(time_start)
	fmt.Println("\n")
	fmt.Printf("your location: %s\n", localIpInfo.Loc)
	fmt.Printf("elapsed time : %s\n", elapsed)

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

// Coord represents a geographic coordinate.
type Coord struct {
	lat float64
	lon float64
}

// these constants are used for vincentyDistance()
const a = 6378137
const b = 6356752.3142
const f = 1 / 298.257223563 // WGS-84 ellipsiod

// ported from JavaScript:
// http://www.5thandpenn.com/GeoMaps/GMapsExamples/distanceComplete2.html
func vincentyDistance(p1, p2 Coord) (float64, bool) {

	// convert from degrees to radians
	p1.lat = p1.lat * math.Pi / 180
	p1.lon = p1.lon * math.Pi / 180
	p2.lat = p2.lat * math.Pi / 180
	p2.lon = p2.lon * math.Pi / 180

	L := p2.lon - p1.lon

	U1 := math.Atan((1 - f) * math.Tan(p1.lat))
	U2 := math.Atan((1 - f) * math.Tan(p2.lat))

	sinU1 := math.Sin(U1)
	cosU1 := math.Cos(U1)
	sinU2 := math.Sin(U2)
	cosU2 := math.Cos(U2)

	lambda := L
	lambdaP := 2 * math.Pi
	iterLimit := 20

	//fmt.Println(L, U1, U2, sinU1, cosU1, sinU2, cosU2)
	//fmt.Println(lambda, lambdaP, iterLimit)

	var sinLambda, cosLambda, sinSigma float64
	var cosSigma, sigma, sinAlpha, cosSqAlpha, cos2SigmaM, C float64

	for {
		if math.Abs(lambda-lambdaP) > 1e-12 && (iterLimit > 0) {
			iterLimit -= 1
		} else {
			break
		}
		sinLambda = math.Sin(lambda)
		cosLambda = math.Cos(lambda)

		sinSigma = math.Sqrt((cosU2*sinLambda)*(cosU2*sinLambda) + (cosU1*sinU2-sinU1*cosU2*cosLambda)*(cosU1*sinU2-sinU1*cosU2*cosLambda))
		if sinSigma == 0 {
			return 0, true // co-incident points
		}
		//fmt.Println(sinSigma)

		cosSigma = sinU1*sinU2 + cosU1*cosU2*cosLambda
		sigma = math.Atan2(sinSigma, cosSigma)
		sinAlpha = cosU1 * cosU2 * sinLambda / sinSigma
		cosSqAlpha = 1 - sinAlpha*sinAlpha
		cos2SigmaM = cosSigma - 2*sinU1*sinU2/cosSqAlpha
		if math.IsNaN(cos2SigmaM) {
			cos2SigmaM = 0 // equatorial line: cosSqAlpha=0
		}

		C = f / 16 * cosSqAlpha * (4 + f*(4-3*cosSqAlpha))
		lambdaP = lambda
		lambda = L + (1-C)*f*sinAlpha*(sigma+C*sinSigma*(cos2SigmaM+C*cosSigma*(-1+2*cos2SigmaM*cos2SigmaM)))
		//fmt.Println(cosSigma, sigma, sinAlpha, cosSqAlpha, cos2SigmaM, C)

		lambda = L + (1-C)*f*sinAlpha*(sigma+C*sinSigma*(cos2SigmaM+C*cosSigma*(-1+2*cos2SigmaM*cos2SigmaM)))
	}
	if iterLimit == 0 {
		return -1, false // formula failed to converge
	}

	uSq := cosSqAlpha * (a*a - b*b) / (b * b)
	A := 1 + uSq/16384*(4096+uSq*(-768+uSq*(320-175*uSq)))
	B := uSq / 1024 * (256 + uSq*(-128+uSq*(74-47*uSq)))

	//fmt.Println(uSq, A, B)

	deltaSigma := B * sinSigma * (cos2SigmaM + B/4*(cosSigma*(-1+2*cos2SigmaM*cos2SigmaM)-B/6*cos2SigmaM*(-3+4*sinSigma*sinSigma)*(-3+4*cos2SigmaM*cos2SigmaM)))
	s := b * A * (sigma - deltaSigma)
	miles := s / 1609.344
	return miles, true
}

func latlon2coord(latlon string) (float64, float64) {
	slots := strings.Split(latlon, ",")
	lat, _ := strconv.ParseFloat(slots[0], 64)
	lon, _ := strconv.ParseFloat(slots[1], 64)
	return lat, lon
}

func outputTable(ipInfo []ipInfoResult, reverseIP map[string]string, loc string) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Input", "IP", "Hostname", "Org", "City", "Region", "Country", "Loc", "Distance"})
	for i, _ := range ipInfo {
		if strings.Contains(ipInfo[i].Ip, ":") { // skip IPv6
			continue
		}

		lat1, lon1 := latlon2coord(loc)
		lat2, lon2 := latlon2coord(ipInfo[i].Loc)
		//fmt.Printf("loc1: %v %v\nloc2: %v %v\n", lat1, lon1, lat2, lon2)
		distance, _ := vincentyDistance(Coord{lat1, lon1}, Coord{lat2, lon2})
		distanceStr := fmt.Sprintf("%.2f", distance)
		row := []string{reverseIP[ipInfo[i].Ip], ipInfo[i].Ip, ipInfo[i].Hostname, ipInfo[i].Org, ipInfo[i].City, ipInfo[i].Region, ipInfo[i].Country, ipInfo[i].Loc, distanceStr}
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

	json.Unmarshal(body, &obj)
	return obj
}

func workIpInfoLookup(workCh chan string, resultCh chan ipInfoResult) {
	for ip := range workCh {
		obj := callRemoteService(ip)
		resultCh <- obj
	}
}

# ipinfo
Return IP address info including geographic location and distance when given IP address, email address, host name or URL

IP geolocation is retrieved from https://ipinfo.io/ who allows for 1000 unauthenticated API calls per day.

## Usage

```
Usage of ipinfo:
  -1	display each entry on one row
  -m	merge identical hosts
  -t int
    	number of simultaneous threads (default 30)
  -v	display program version and then exit
  -w	wrap output to better fit the screen width
  -x	only display your external IP and then exit
```

## Installation

* macOS: `brew update; brew install jftuga/tap/ipinfo`
* Binaries for Linux, macOS and Windows are provided in the [releases](https://github.com/jftuga/ipinfo/releases) section.

## Examples

```
macbook:ipinfo jftuga$ ./ipinfo amazon.com https://cisco.com user@github.com

+-----------------+-----------------------------------------------+---------------------+----------+--------+
|    INPUT/IP     |                 HOSTNAME/ORG                  | CITY/REGION/COUNTRY | LAT/LON  |  DIST  |
+-----------------+-----------------------------------------------+---------------------+----------+--------+
| amazon.com      | s3-console-us-standard.console.aws.amazon.com | Ashburn             |  39.0437 | 482.55 |
| 205.251.242.103 | AS16509 Amazon.com, Inc.                      | Virginia            | -77.4875 |        |
|                 |                                               | US                  |          |        |
| amazon.com      |                                               | Ashburn             |  39.0437 | 482.55 |
| 52.94.236.248   | AS16509 Amazon.com, Inc.                      | Virginia            | -77.4875 |        |
|                 |                                               | US                  |          |        |
| amazon.com      |                                               | Ashburn             |  39.0437 | 482.55 |
| 54.239.28.85    | AS16509 Amazon.com, Inc.                      | Virginia            | -77.4875 |        |
|                 |                                               | US                  |          |        |
| cisco.com       | redirect-ns.cisco.com                         | Richardson          |  32.9482 | 770.84 |
| 72.163.4.185    | AS109 CISCO SYSTEMS, INC.                     | Texas               | -96.7297 |        |
|                 |                                               | US                  |          |        |
| github.com      | lb-140-82-112-4-iad.github.com                | South Riding        |  38.9209 | 475.94 |
| 140.82.112.4    | AS36459 GitHub, Inc.                          | Virginia            | -77.5039 |        |
|                 |                                               | US                  |          |        |
+-----------------+-----------------------------------------------+---------------------+----------+--------+

your IP addr : *redacted*
your location: *redacted*
elapsed time : 450.60ms

macbook:ipinfo jftuga$ ./ipinfo -1 amazon.com https://cisco.com user@github.com

+------------+-----------------+-----------------------------------------------+---------------------------+--------------+----------+---------+------------------+--------+
|   INPUT    |       IP        |                   HOSTNAME                    |            ORG            |     CITY     |  REGION  | COUNTRY |     LAT/LON      |  DIST  |
+------------+-----------------+-----------------------------------------------+---------------------------+--------------+----------+---------+------------------+--------+
| amazon.com | 205.251.242.103 | s3-console-us-standard.console.aws.amazon.com | AS16509 Amazon.com, Inc.  | Ashburn      | Virginia | US      | 39.0437,-77.4875 | 482.55 |
| amazon.com | 52.94.236.248   |                                               | AS16509 Amazon.com, Inc.  | Ashburn      | Virginia | US      | 39.0437,-77.4875 | 482.55 |
| amazon.com | 54.239.28.85    |                                               | AS16509 Amazon.com, Inc.  | Ashburn      | Virginia | US      | 39.0437,-77.4875 | 482.55 |
| cisco.com  | 72.163.4.185    | redirect-ns.cisco.com                         | AS109 CISCO SYSTEMS, INC. | Richardson   | Texas    | US      | 32.9482,-96.7297 | 770.84 |
| github.com | 140.82.113.3    | lb-140-82-113-3-iad.github.com                | AS36459 GitHub, Inc.      | South Riding | Virginia | US      | 38.9209,-77.5039 | 475.94 |
+------------+-----------------+-----------------------------------------------+---------------------------+--------------+----------+---------+------------------+--------+

your IP addr : *redacted*
your location: *redacted*
elapsed time : 450.60ms
```

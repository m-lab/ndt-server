# ndt7 protocol specification

This specification describes version 7 of the Network Diagnostic
Tool (NDT) protocol (ndt7). Ndt7 is a non-backwards compatible
redesign of the [original NDT network performance measurement
protocol](https://github.com/ndt-project/ndt). Ndt7 is based on
WebSocket and TLS, and takes advantage of TCP BBR, where this
flavour of TCP is available.

This is version v0.7.3 of the ndt7 specification.

## Protocol description

The client connects to the server using HTTPS and requests to upgrade the
connection to WebSockets. The same connection will be used to exchange
control and measurement messages. The upgrade request URL will indicate
the type of subtest that the client wants to perform. Two subtests and
hence two URLs are defined:

- `/ndt/v7/download`, which selects the download subtest;

- `/ndt/v7/upload`, which selects the upload subtest.

The upgrade message MUST also contain the WebSocket subprotocol that
identifies ndt7, which is `net.measurementlab.ndt.v7`. The URL in the
upgrade request MAY contain optional parameters, which will be saved
as metadata describing the client (see below).  An upgrade request
could look like this:

```
GET /ndt/v7/download HTTP/1.1\r\n
Host: localhost\r\n
Connection: Upgrade\r\n
Sec-WebSocket-Key: DOdm+5/Cm3WwvhfcAlhJoQ==\r\n
Sec-WebSocket-Version: 13\r\n
Sec-WebSocket-Protocol: net.measurementlab.ndt.v7\r\n
Upgrade: websocket\r\n
\r\n
```

Upon receiving the upgrade request, the server MUST inspect the
request (including the optional query string) and either upgrade
the connection to WebSocket or return a 400 failure if the
request does not look correct (e.g. if the WebSocket subprotocol
is missing). The upgrade response MUST contain the selected
subprotocol in compliance with RFC6455. A possible upgrade response
could look like this:

```
HTTP/1.1 101 Switching Protocols\r\n
Sec-WebSocket-Protocol: net.measurementlab.ndt.v7\r\n
Sec-WebSocket-Accept: Nhz+x95YebD6Uvd4nqPC2fomoUQ=\r\n
Upgrade: websocket\r\n
Connection: Upgrade\r\n
\r\n
```

Once the WebSocket channel is established, the client and the server
exchange ndt7 messages using the WebSocket framing. An implementation MAY
choose to limit the maximum WebSocket message size, but such limit MUST
NOT be smaller than 1 << 24 bytes. Note that this message size is a maximum. It
is expected that most clients will complete the entire ndt7 test with much
smaller messages and that large messages would only be sent to clients on
very fast links.


Both textual and binary WebSocket messages are allowed. Textual WebSocket
messages will contain serialised JSON structures containing measurements
results (see below). When downloading, the server is expected to send
measurement to the client, and when uploading, conversely, the client is
expected to send measurements to the server. Measurements SHOULD
NOT be sent more frequently than every 250 ms, to avoid generating too
much unnecessary processing load on the receiver. A party receiving
too frequent measurements MAY decide to close the connection.

To generate network load, the party that is currently sending (i.e. the
server during a download subtest) MUST send, in addition to textual
WebSocket messages, binary WebSocket messages carrying a random payload;
the receiver (i.e. the client during a download subtest) MUST discard
these messages without processing them.

As far as binary messages are concerned, ndt7 subtests are strictly
half duplex. During the download, the client MUST NOT send any binary
message to the server. During the upload, the server MUST NOT send
any binary message to the client. If a party receives a binary message
when that is not expected, it MUST close the connection.

All other messages are permitted. Implementations should be prepared
to receive such messages during any subtest. Processing these messages
isn't mandatory and an implementation MAY choose to ignore them. An
implementation SHOULD NOT send this kind of messages more frequently
than every 250 millisecond. An implementation MAY close the connection
if receiving such messages too frequently. The reason why we allow
this kind of messages is so that the server could sent to the client
download speed measurements during the upload test. This provides
clients that do not have BBR support with a reasonably good estimation
of the real upload speed, which is certainly more informative and
stable than any application level sender side estimation.

The expected transfer time of each subtest is ten seconds (unless BBR
is used, in which case it may be shorter, as explained below). The sender
(i.e. the server during the download subtest) SHOULD stop sending after
ten seconds. The receiver (i.e. the client during the download subtest) MAY
close the connection if the elapsed time exceeds fifteen seconds.

When the expected transfer time has expired, the parties SHOULD close
the WebSocket channel by sending a Close WebSocket frame. The client
SHOULD NOT close the TCP connection immediately, so that the server can
close it first. This allows to reuse ports more efficiently on the
server because we avoid the `TIME_WAIT` TCP state.

## Stopping the transfer earlier using BBR

If TCP BBR is available, a compliant server MAY choose to enable it
for the client connection and terminate the download test early when
it believes that BBR parameters become stable. Before v1.0 of this
spec is out, we hope to specify a mechanism allowing a client to opt out
of terminating the download early.

Clients can detect whether BBR is enabled by checking whether the measurement
returned by the server contains a `bbr_info` field (see below).

## Query string parameters

The client SHOULD send metadata using the query string. The server
SHOULD process the query string, returning 400 if the query string is
not parseable or not acceptable (see below). The server SHOULD store
the metadata sent by the client using the query string.

The following restrictions apply to the query string. It MUST NOT be
longer than 4096 bytes. Of course, both the name and the value of the
URL query string MUST be valid URL-encoded UTF-8 strings.

Clients MUST NOT send duplicate keys; servers SHOULD ignore them.

The `"^server_"` prefix is reserved for the server. Clients MUST not send any
metadata starting with such prefix. Servers MUST ignore all the entries that
start with such prefix.

## Measurements message

As mentioned above, the server and the client exchange JSON measurements
using Textual WebSocket messages. Such JSON measurements have the following
structure:

```json
{
  "app_info": {
    "num_bytes": 17,
  },
  "connection_info": {
    "client": "1.2.3.4:5678",
    "server": "[::1]:2345",
    "uuid": "urn:<platform>:<platform-specific-string>"
  },
  "bbr_info": {
    "max_bandwidth": 12345,
    "min_rtt": 123.4
  },
  "elapsed": 1.2345,
  "tcp_info": {
    "rtt_var": 123.4,
    "smoothed_rtt": 567.8
  }
}
```

Where:

- `app_info` is an _optional_ JSON object only included in the measurement
  when an application-level measurement is available:

    - `num_bytes` (a `int64`) is the number of bytes sent (or received) since
      the beginning of the specific subtest. Note that this counter tracks the
      amount of data sent at application level. It does not account for the
      protocol overheaded of WebSockets, TLS, TCP, IP, and link layer;

- `connection_info` is an _optional_ JSON object that the server SHOULD
  provide as part of the first measurement message sent to clients to
  inform them about:

    - `client` (a `string`), which contains the serialization of the client
      endpoint according to the server. Note that the general format of
      this field is `<address>:<port>` where IPv4 addresses are provided
      verbatim, while IPv6 addresses are quoted by `[` and `]` as shown
      in the above example;

    - `server` (a `string`), which contains the serialization of the server
      endpoint according to the server, following the same general format
      specified above for the client `field`;

    - `uuid` (a `string`), which contains an internal unique identifier
      for this test within the Measurement Lab platform, following the
      following format `urn:<platform>:<platform-specific-string>` where
      the `mlab` string is reserved for tests run in the Measurement
      Lab platform.

- `bbr_info` is an _optional_ JSON object only included in the measurement
  when it is possible to access `TCP_CC_INFO` stats for BBR:

    - `bbr_info.max_bandwidth` (a `int64`) is the max-bandwidth measured by
       BBR, in bits per second;

    - `bbr_info.min_rtt` (a `float64`) is the min-rtt measured by BBR,
      in millisecond;

- `elapsed` (a `float64`) is the number of seconds elapsed since the beginning
  of the specific subtest and marks the moment in which the measurement has
  been performed by the client or by the server;

- `tcp_info` is an _optional_ JSON object only included in the measurement
  when it is possible to access `TCP_INFO` stats:

    - `tcp_info.rtt_var` (a `float64`) is RTT variance in milliseconds;

    - `tcp_info.smoothed_rtt` (a `float64`) is the smoothed RTT in milliseconds.

Note that JSON and JavaScript actually define integers as `int53` but existing
implementations will likely round bigger (or smaller) numbers to the nearest
`float64` value. A pedantic implementation MAY want to be overly defensive and
make sure that it does not mit values that a `int53` cannot represent. The
proper action to take in this case is currently unspecified.

## Server discovery

This section explains how a client can get the FQDN of a ndt7-enabled
M-Lab server for the purpose of running tests. Of course, this
section only applies to clients using M-Lab infrastructure. Clients
using other server infrastructure MUST refer to the documentation
provided by such infrastructure instead.

Clients:

1. SHOULD use the [locate.measurementlab.net](
https://locate.measurementlab.net/) server-discovery web API;

2. MUST query for the `ndt7` locate.measurementlab.net tool (see below for a
description of how a real request would look like);

3. MUST set `User-Agent` to identify themselves;

4. MUST correctly handle `3xx` redirects, interpret `200` as success, `204`
as the no-capacity signal (see below) and any other status as failure.

The no-capacity signal is emitted by the locate.measurementlab.net service
when M-Lab is out of capacity. In such cases, the return code is `204`,
to indicate that there is no (ndt7 server) content for the requesting client.

The following example shows a request to locate.measurementlab.net originating
from a well-behaved ndt7 client:

```
* Connected to locate.measurementlab.net (216.58.205.84) port 443 (#0)
> GET /ndt7 HTTP/1.1
> Host: locate.measurementlab.net
> User-Agent: MKEngine/0.1.0
> Accept: application/json
>
```

A possible successful response from a server could look like the
following (where some irrelevant JSON fields have been omitted for
the sake of brevity and thus content-length is now wrong):

```
< HTTP/1.1 200
< cache-control: no-cache
< access-control-allow-origin: *
< content-type: application/json
< date: Thu, 02 May 2019 13:46:32 GMT
< server: Google Frontend
< content-length: 622
<
{ "fqdn": "ndt-iupui-mlab2-tun01.measurement-lab.org" }
```

In case of capacity issues (as specified above), the server response
would instead look like the following:

```
< HTTP/2 204
< cache-control: no-cache
< access-control-allow-origin: *
< content-type: application/json
< date: Thu, 02 May 2019 13:46:32 GMT
< server: Google Frontend
< content-length: 0
<
```

### Requirements for non-interactive clients

Non-interactive clients SHOULD schedule tests according
to the following algorithm:

1. run a test

2. make sure that the RNG is correctly seeded;

3. extract `t` from an exponential distribution with average
equal to 21'600 seconds (i.e. six hours);

4. if `t` is smaller than 2'160 seconds, set `t` to 2'160 seconds;

5. if `t` is larger 54'000 seconds, set `t` to 54'000 seconds;

6. sleep for `t` seconds;

7. goto step 1.

An hypothetical non-interactive ndt7 client written in Go SHOULD do:

```Go
import (
	"math/rand"
	"sync"
	"time"
)

var once sync.Once

func sleepTime() time.Duration {
	once.Do(func() {
		rand.Seed(time.Now().UTC().UnixNano())
	})
	t := rand.ExpFloat64() * 21600
	if t < 2160 {
		t = 2160
	} else if t > 54000 {
		t = 54000
	}
	return time.Duration(t * float64(time.Second))
}

func mainLoop() {
	for {
		tryPerformanceTest()
		time.Sleep(sleepTime())
	}
}
```

The locate.measurementlab.net service will return an empty result if
M-Lab is out of capacity, as mentioned above. In such case, a non-interactive
client SHOULD:

1. either skip the test and wait until it's time to run the next test; or

2. retry contacting locate.measurementlab.net applying an exponential
backoff by extracting from a normal distribution with increasing average (60,
120, 240, 480, 960, ... seconds) and standard deviation equal to 5% of
the average value.

In our hypotethical Go client, the exponential backoff would be
implementation like this:

```Go
func tryPerformanceTest() {
	for mean := 60.0; mean <= 960.0; mean *= 2.0 {
		fqdn, err := locateServer()
		if err != nil {
			// Note: RNG already seeded as shown above
			stdev := 0.05 * mean
			seconds := rand.NormFloat64() * stdev + mean
			time.Sleep(time.Duration(seconds * float64(time.Second)))
			continue
		}
		runWithServer(fqdn)
		return
	}
}
```

## Reference implementation

The reference _server_ implementation is [github.com/m-lab/ndt-server](
https://github.com/m-lab/ndt-server).

The reference _Go client_ is [github.com/m-lab/ndt7-client-go](
https://github.com/m-lab/ndt7-client-go).

The reference _JavaScript client_ is [github.com/m-lab/ndt7-client-javascript](
https://github.com/m-lab/ndt7-client-javascript).

# ndt7 protocol specification

This specification describes ndt7, i.e. version 7 of the Network
Diagnostic Tool (NDT) protocol. Ndt7 is a non-backwards compatible
redesign of the [original NDT network performance measurement
protocol](https://github.com/ndt-project/ndt). Ndt7 is based on
WebSocket and TLS, and takes advantage of TCP BBR, where this
flavour of TCP is available.

This is version v0.9.0 of the ndt7 specification.

## Design choices

(This section is non-normative.)

Ndt7 measures the application-level download and upload performance
using WebSockets over TLS. Each test type is independent, and there are
three types of test: the download, the upload tests, and the latency
test. Ndt7 always uses a single new TCP connection for each type of
test. Whenever possible, ndt7 uses a recent version of TCP BBR. Writing
an ndt7 client is designed to be as simple as possible. [A complete Go
language ndt7 client](
https://github.com/bassosimone/ndt7-client-go-minimal) has been implemented
in just 151 lines. We used 26 lines for the download, 33 for the upload, and
17 for establishing a connections. No code from the NDT server has been
reused. [A complete JavaScript client](
https://github.com/bassosimone/ndt7-server-go-minimal/tree/master/static)
has been implemented in just 122 lines (32 for the download, 51 for the
upload, and 39 for controlling the maximum test duration). It is a design
goal of ndt7 that the size of a minimal client remains small over time.

Ndt7 answers the question of how fast you could pull/push data
from your device to a typically-nearby, well-provisioned web
server by means of commonly-used web technologies. This is not necessarily
a measurement of your last mile speed. Rather it is a measurement
of what performance is possible with your device, your current internet
connection (landline, Wi-Fi, 4G, etc.), the characteristics of
your ISP and possibly of other ISPs in the middle, and the server
being used. The main metric measured by ndt7 is the goodput, i.e.,
the speed measured at application level, without including the
overheads of WebSockets, TLS, TCP/IP, and link layer headers. But we
also provide kernel-level information from `TCP_INFO` where available. For
all these reasons we say that ndt7 performs application-level measurements.

The presence of network issues (e.g. interference or congestion) should
cause ndt7 to yield worse measurement results, relative to the expected speed
of the end-to-end connection. The expected speed of a BBR connection on an
unloaded network is the bitrate of the slowest hop in the measurement
path, and the slowest hop is usually also the last hop.
Extra information obtained using `TCP_INFO` should help an expert
reading the results of a ndt7 experiment to better understand what could
be the root cause of such performance issues.

To ensure that clients continue to work, we make the design choice that
clients should be maximally simple, and that all complexity should
implemented on the server side.

Ndt7 should consume few resources. The maximum runtime of a test should
be ten seconds, but the server should be able to determine if the performance
if performance has stabilized in less than ten seconds and end the test early.

## Protocol description

This section describes how ndt7 uses WebSockets, and how clients and
servers should behave during the download and the upload tests.

### The WebSocket handshake

The client connects to the server using HTTPS and requests to upgrade the
connection to WebSockets. The same connection will be used to exchange
control and measurement messages. The upgrade request URL will indicate
the type of test that the client wants to perform. Three tests and
hence three URLs are defined:

- `/ndt/v7/download`, which selects the download test;

- `/ndt/v7/upload`, which selects the upload test;

- `/ndt/v7/ping`, which selects the ping test.

The upgrade message MUST also contain the WebSocket subprotocol that
identifies ndt7, which is `net.measurementlab.ndt.v7`. The URL in the
upgrade request MAY contain optional query-string parameters, which
will be saved as metadata describing the client. The client SHOULD also
include a meaningful, non-generic User-Agent string.

The query string MUST NOT be longer than 4096 bytes. Both the name and
the value of the query string parameters MUST of course be valid URL-encoded
UTF-8 strings. Clients MUST NOT send duplicate keys and MUST NOT send
keys without a value. Servers SHOULD ignore duplicate keys. Servers SHOULD
treat keys without a value as having an empty value.

An upgrade request could look like this:

```
GET /ndt/v7/download HTTP/1.1\r\n
Host: localhost\r\n
Connection: Upgrade\r\n
Sec-WebSocket-Key: DOdm+5/Cm3WwvhfcAlhJoQ==\r\n
Sec-WebSocket-Version: 13\r\n
Sec-WebSocket-Protocol: net.measurementlab.ndt.v7\r\n
Upgrade: websocket\r\n
User-Agent: ooniprobe/3.0.0 ndt7-client-go/0.1.0\r\n
\r\n
```

Upon receiving an upgrade request, the server MUST inspect the
request (including the optional query string) and either upgrade
the connection to WebSocket or return a 4xx failure if the
request does not look correct (e.g. if the WebSocket subprotocol
is missing). Of course, also the query string MUST be parsed
and the upgrade MUST fail if the query string is not parseable
or not acceptable. The server SHOULD store the metadata sent by
the client using the query string.

The upgrade response MUST contain the selected subprotocol in compliance
with RFC6455. A possible upgrade response could look like this:

```
HTTP/1.1 101 Switching Protocols\r\n
Sec-WebSocket-Protocol: net.measurementlab.ndt.v7\r\n
Sec-WebSocket-Accept: Nhz+x95YebD6Uvd4nqPC2fomoUQ=\r\n
Upgrade: websocket\r\n
Connection: Upgrade\r\n
\r\n
```

### WebSocket channel usage

Once the WebSocket channel is established, the client and the server
exchange ndt7 messages using the WebSocket framing. An implementation MAY
choose to limit the maximum WebSocket message size, but such limit MUST
NOT be smaller than 1 << 24 bytes. Note that this message size is a maximum
designed to support clients on very fast, short end-to-end paths.

Textual, binary, and control WebSocket messages are used.

Textual messages contain JSON-serialized measurements, they are OPTIONAL, and
are always permitted. A ndt7 implementation MAY choose to ignore all textual
messages. This provision allows one to easily implement ndt7 with languages
such as C where reading and writing messages at the same time significantly
increases the implementation complexity. A ndt7 implementation SHOULD NOT send
more than ten textual messages per second on the average. A ndt7 implementation
MAY choose to discard incoming textual messages at random, if it is receiving
too many textual messages in a given time interval.

Binary messages SHOULD contain random data and are used to generate network
load. Therefore, during the download test the server sends binary
messages and the client MUST NOT send them. Likewise, during the upload
test, the client sends binary messages and the server MUST NOT send
them. An implementation receiving a binary message when it is not expected
SHOULD close the underlying TLS connection.

Binary messages MUST contain between 1 << 10 and 1 << 24 bytes, and
SHOULD be a power of two. An implementation SHOULD initially be sending
binary messages containing 1 << 13 bytes, and it
MAY change the size of such messages to accommodate for fast clients, as
mentioned above. See the appendix for a possible algorithm to dynamically
change the message size.

The expected duration of a test is _up to_ ten seconds. If a test has
been running for at least thirteen seconds, an implementation MAY close the
underlying TLS connection. This is allowed to keep the overall duration
of each test within a thirteen second upper bound. Ideally this SHOULD
be implemented so that immediately after thirteen seconds have elapsed, the
underlying TLS connection is closed. This can be implemented, e.g., in C/C++
using alarm(3) to cause pending I/O operations to fail with `EINTR`.

As regards ping and pong messages, a ndt7 server MAY send periodic
ping messages during any test. Servers MUST NOT send ping messages more
frequently than they would send textual messages. Clients MUST be prepared
to receive ping messages. They MUST reply to such messages with pong
messages containing the same payload, and they SHOULD do that as soon
as practical. If a client is receiving too many ping messages in a specific
time interval, it MAY drop them at random.

The server MAY initiate a WebSocket closing handshake at any time
and during any test. This tells the client that either the specific
test has run for too much time, or that some other criteria suggests
that the measured speed would not change significantly in the future,
hence continuing the test would waste resources. The client SHOULD be
prepared to receive such closing handshake and respond accordingly. In
accordance with RFC 6455 Sect 7.1.1, once the WebSocket closing handshake
is complete, the server SHOULD close the underlying TLS connection,
while the client SHOULD wait and see whether the server closes it first.

In practice, both the client and the server SHOULD tolerate any
abrupt EOF, RST, or timeout/alarm error received when doing I/O with the
underlying TLS connection. Such events SHOULD be logged as warnings
and SHOULD be recorded into the results. For robustness, we do not
want such events to cause a whole test to fail. This provision
gives the ndt7 protocol bizantine robustness, and acknowledges the
fact that the web is messy and a test may terminate more abruptly
than it happened in the past in our controlled experiments.

### Measurement message

As mentioned above, the server and the client exchange JSON measurements
using textual WebSocket messages. The purpose of these measurements is to
provide information useful to diagnose performance issues.

While in theory we could specify all `TCP_INFO` and `BBR_INFO` variables,
different kernel versions provide different subsets of these measurements
and we do not want to be needlessly restrictive regarding the underlying
kernel for the server. Instead,
our guiding principle is to describe only the variables that in our
experience are useful to understand performance issues. More variables
could be added in the future. No variables should be removed, but, if
some are removed, we should document them as being removed rather than
removing them from this specification.

Since version v0.9.0 of this specification, the measurement message
has the following structure:

```json
{
  "AppInfo": {
    "ElapsedTime": 1234,
    "NumBytes": 1234,
  },
  "ConnectionInfo": {
    "Client": "1.2.3.4:5678",
    "Server": "[::1]:2345",
    "UUID": "<platform-specific-string>"
  },
  "Origin": "server",
  "Test": "download",
  "WSPingInfo": {
    "ElapsedTime": 1234,
    "LastRTT": 134,
    "MinRTT": 1234
  },
  "TCPInfo": {
    "BusyTime": 1234,
    "BytesAcked": 1234,
    "BytesReceived": 1234,
    "BytesSent": 1234,
    "BytesRetrans": 1234,
    "ElapsedTime": 1234,
    "MinRTT": 1234,
    "RTT": 1234,
    "RTTVar": 1234,
    "RWndLimited": 1234,
    "SndBufLimited": 1234
  }
}
```

Where:

- `AppInfo` is an _optional_ `object` only included in the measurement
  when an application-level measurement is available:

    - `ElapsedTime` (a `int64`) is the time elapsed since the beginning of
      this test, measured in microseconds.

    - `NumBytes` (a `int64`) is the number of bytes sent (or received) since
      the beginning of the specific test. Note that this counter tracks the
      amount of data sent at application level. It does not account for the
      overheaded of the WebSockets, TLS, TCP/IP, and link layers.

- `ConnectionInfo` is an _optional_ `object` used to provide information
  about the connection four tuple. Clients MUST NOT send this message. Servers
  MUST send this message exactly once. Clients SHOULD cache the first
  received instance of this message, and discard any subsequently received
  instance of this message. The contents of the object are:

    - `Client` (a `string`), which contains the serialization of the client
      endpoint according to the server. Note that the general format of
      this field is `<address>:<port>` where IPv4 addresses are provided
      verbatim, while IPv6 addresses are quoted by `[` and `]` as shown
      in the above example.

    - `Server` (a `string`), which contains the serialization of the server
      endpoint according to the server, following the same general format
      specified above for the `Client` field.

    - `UUID` (a `string`), which contains an internal unique identifier
      for this test within the Measurement Lab (M-Lab) platform. This field
      SHOULD be omitted by servers running on other platforms, unless they
      also have the concept of a UUID bound to a TCP connection.

- `Origin` is an _optional_ `string` that indicates whether the measurement
  has been performed by the client or by the server. This field SHOULD
  only be used when the entity that performed the measurement would otherwise
  be ambiguous.

- `Test` is an _optional_ `string` that indicates the name of the
  current test. This field SHOULD only be used when the current test
  should otherwise not be obvious.

- `WSPingInfo` is an _optional_ `object` only included in the measurement
  when a reasonable websocket-level measurement is available:

    - `ElapsedTime` (a `int64`) is the time elapsed since the beginning of
      this test, measured in microseconds.

    - `LastRTT` (an _optional_ `int64`), the last observed RTT for the websocket
      ping-pong exchange, measured in microseconds.

    - `MinRTT` (an _optional_ `int64`), the minimum observed RTT for the websocket
      ping-pong exchange, measured in microseconds.

- `TCPInfo` is an _optional_ `object` only included in the measurement
  when it is possible to access `TCP_INFO` stats. It contains:

    - `BusyTime` aka `tcpi_busy_time` (an _optional_ `int64`), i.e. the number of
       microseconds spent actively sending data because the write queue
       of the TCP socket is non-empty.

    - `BytesAcked` aka `tcpi_bytes_acked` (an _optional_ `int64`), i.e. the number
      of bytes for which we received acknowledgment. Note that this field,
      and all other `TCPInfo` fields, contain the number of bytes measured
      at TCP/IP level (i.e. including the WebSocket and TLS overhead).

    - `BytesReceived` aka `tcpi_bytes_received` (an _optional_ `int64`), i.e. the number
      of bytes for which we sent acknowledgment.

    - `BytesSent` aka `tcpi_bytes_sent` (an _optional_ `int64`), i.e. the number of bytes
      which have been transmitted _or_ retransmitted.

    - `BytesRetrans` aka `tcpi_bytes_retrans` (an _optional_ `int64`), i.e. the number
      of bytes which have been retransmitted.

    - `ElapsedTime` (an _optional_ `int64`), i.e. the time elapsed since the beginning of
      this test, measured in microseconds.

    - `MinRTT` aka `tcpi_min_rtt` (an _optional_ `int64`), i.e. the minimum RTT seen
       by the kernel, measured in microseconds.

    - `RTT` aka `tcpi_rtt` (an _optional_ `int64`), i.e. the current smoothed RTT
      value, measured in microseconds.

    - `RTTVar` aka `tcpi_rtt_var` (an _optional_ `int64`), i.e. the variance or `RTT`.

    - `RWndLimited` aka `tcpi_rwnd_limited` (an _optional_ `int64`), i.e. the amount
      of microseconds spent stalled because there is not enough
      buffer at the receiver.

    - `SndBufLimited` aka `tcpi_sndbuf_limited` (an _optional_ `int64`), i.e. the amount
      of microseconds spent stalled because there is not enough buffer at
      the sender.

Note that the JSON exchanged on the wire, or saved on disk, MAY possibly
contain more `TCP_INFO` fields. Yet, only the fields described in this
specification SHOULD BE returned by a compliant, `TCP_INFO` enabled
implementation of ndt7. A client MAY use other fields, but the absence of
those other fields in a server response MUST NOT be a fatal client error.

The `TCP_INFO` variables mentioned by this specification are supported by the
Linux kernel used by M-Lab, which is >= 4.19. An implementation running on
a kernel where a specific `TCP_INFO` variable mentioned in this specification
is missing SHOULD NOT include such variable in the `TCPInfo` object sent to
the client. In this regard, clients should be robust to missing data. Missing
data SHOULD NOT cause a client to crash, although it is allowable for it to
cause an inferior user experience.

Moreover, note that all the variables presented above increase or otherwise
change consistently during a test. Therefore, the most recent measurement sample
is a suitable summary of all prior measurements, and the last measurement received
should be treated as the summary message for the test.

Finally, note that JSON and JavaScript actually define integers as `int53` but
existing implementations will likely round bigger (or smaller) numbers to
the nearest `float64` value. A pedantic implementation MAY want to be overly
defensive and make sure that it does not emit values that a `int53` cannot
represent.

### Examples

This section is non normative. It shows the messages seen by a client
in various circumstances. The `>` prefix indicates a sent message. The
`<` prefix indicates a received message.

In this first example, the client and the server do not send any
text message. These are the simplest ndt7 client and server you can
write. This is what the download looks like:

```
> GET /ndt/v7/download Upgrade: websocket
< 101 Switching Protocols
< BinaryMessage
< BinaryMessage
< BinaryMessage
< BinaryMessage
...
< CloseMessage
> CloseMessage
```

This is the upload:

```
> GET /ndt/v7/upload Upgrade: websocket
< 101 Switching Protocols
> BinaryMessage
> BinaryMessage
> BinaryMessage
> BinaryMessage
...
> CloseMessage
< CloseMessage
```

When the server sends measurement messages, the download becomes:

```
> GET /ndt/v7/download Upgrade: websocket
< 101 Switching Protocols
< PingMessage
< BinaryMessage
< BinaryMessage
< TextMessage    clientElapsedTime=0.30 s
> PongMessage
< BinaryMessage
< BinaryMessage
< TextMessage    clientElapsedTime=0.55 s
< BinaryMessage
...
< CloseMessage
> CloseMessage
```

Note that we have assumed that measurements arrive with a little of delay
caused by the queuing delay in the sender's buffer. In some cases, this
delay may be large enough that clients SHOULD NOT rely on server measurements
to timely update their user interface during this test.

During the upload, the server could help a client by sending messages
containing its application-level measurements:

```
> GET /ndt/v7/upload Upgrade: websocket
< 101 Switching Protocols
> BinaryMessage
> BinaryMessage
> BinaryMessage
< TextMessage    clientElapsedTime=0.25 s
> BinaryMessage
> BinaryMessage
< TextMessage    clientElapsedTime=0.51 s
> BinaryMessage
...
> CloseMessage
< CloseMessage
```

In this case, because messages travel on an otherwise only used for pure
ACKs return path, we expect less queuing delay. Still also in
this case it is RECOMMENDED to always use application level measurements
at the sender to update the user interface.

## Server discovery

This section explains how a client can get the FQDN of a ndt7-enabled
M-Lab server for the purpose of running tests. Of course, this
section only applies to clients using M-Lab infrastructure. Clients
using other server infrastructure MUST refer to the documentation
provided by such infrastructure instead.

Clients using the M-Lab ndt7 infrastructure:

1. SHOULD use the [locate.measurementlab.net](
https://locate.measurementlab.net/) server-discovery web API;

2. SHOULD query for the `ndt7` locate.measurementlab.net tool (see below for a
description of how a real request would look like);

3. SHOULD set `User-Agent` to identify themselves;

4. MUST correctly handle `3xx` redirects, interpret `200` as success, `204`
as the no-capacity signal (see below) and any other status as failure;

5. MUST NOT assume that the locate service will always geographically
locate them to the same server, or always to the strictly nearest server,
or to a server running a particular version of the code, or to a server
that will always run a 10 second test instead of a shorter test. Rollouts
on the M-Lab platform happen gradually, and canary tests of new server
versions happen regularly, which means that different servers may be
running different code versions. Well-written clients should already be
robust to these scenarios, but we note them here as an additional reminder.

The no-capacity signal is emitted by the locate.measurementlab.net service
when M-Lab is out of capacity. In such cases, the return code is `204`,
to indicate that there is no ndt7-server "content" for the requesting client.

The following example shows a request to locate.measurementlab.net originating
from a well-behaved ndt7 client:

```
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
< Cache-Control: no-cache
< Access-Control-Allow-Origin: *
< Content-Type: application/json
< Date: Thu, 02 May 2019 13:46:32 GMT
< Server: Google Frontend
< Content-Length: 622
<
{ "fqdn": "ndt-iupui-mlab2-tun01.measurement-lab.org" }
```

In case of capacity issues (as specified above), the server response
would instead look like the following:

```
< HTTP/1.1 204 No Content
< Cache-Control: no-cache
< Access-Control-Allow-Origin: *
< Content-Type: application/json
< Date: Thu, 02 May 2019 13:46:32 GMT
< Server: Google Frontend
< Content-Length: 0
<
```

In such case, an interactive client SHOULD report an error to the
user, while a non-interactive client MAY retry (see below).

### Requirements for non-interactive clients

Non-interactive clients SHOULD schedule tests according
to the following algorithm:

1. run a test

2. make sure that the RNG is correctly seeded;

3. extract `t` from an exponential distribution with average
equal to 21600 seconds (i.e. six hours);

4. if `t` is smaller than 2160 seconds, set `t` to 2160 seconds;

5. if `t` is larger 54000 seconds, set `t` to 54000 seconds;

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

The locate.measurementlab.net service will return a no-content result if
M-Lab is out of capacity, as mentioned above. In such case, a non-interactive
client SHOULD:

1. either skip the test and wait until it's time to run the next test; or

2. retry contacting locate.measurementlab.net applying an exponential
backoff by extracting from a normal distribution with increasing average (60,
120, 240, 480, 960, ... seconds) and standard deviation equal to 5% of
the average value.

In our hypotethical Go client, the exponential backoff would be
implementated like this:

```Go
func tryPerformanceTest() {
	for mean := 60.0; mean <= 960.0; mean *= 2.0 {
		fqdn, err := locatesvc.LocateServer()
		if err != nil && err != locatesvc.ErrNoContent {
			return
		}
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

## Reference implementations

The reference _server_ implementation is [github.com/m-lab/ndt-server](
https://github.com/m-lab/ndt-server). Such repository also contains
the reference JavaScript implementation, in [html/ndt7-core.js](
html/ndt7-core.js), which is served by default by the NDT server. This
JavaScript code may be useful to others in building their own speed test
websites. It SHOULD be used to test server implementations, and as a
very basic client for occasional use.

The reference _Go client_ is [github.com/m-lab/ndt7-client-go](
https://github.com/m-lab/ndt7-client-go).

## Appendix

This section is non-normative. Here we describe possible implementations
and provide other information that could be useful to implementors.

### Adapting binary message size

We can double the message size when it becomes smaller than a fixed percentage
of the number of bytes queued so far. In the following example, we double the
message size when it is smaller than `1/16` of the queued bytes:

```Go
	var total int64
	msg.Resize(1 << 13)
	for {
		if err := conn.Send(msg); err != nil {
			return err
		}
		total += msg.Size()
		if msg.Size() >= (1 << 24) || msg.Size() >= (total / 16) {
			continue
		}
		msg.Resize(msg.Size() * 2)
	}
```

This algorithm is such that only faster clients are exposed to larger
messages, therefore reducing the overhead of receiving many small messages,
which is especially critical to browser-based clients.

### Using TCPInfo variables

The `ElapsedTime` field indicates the moment in which `TCP_INFO` data has
been generated, and therefore is generally useful.

The `MinRTT`, `RTT`, and `RTTVar` fields can be used to compute the statistics
of the round-trip time. The buildup of a large queue is unexpected when using
BBR. It generally indicates the presence of a bottleneck with a large buffer
that's filling as the test proceeds. The `MinRTT` can also be useful to verify
we're using a reasonably nearby-server. Also, an unreasonably small RTT when
the link is 2G or 3G could indicate a performance enhancing proxy, one can
compare `TCPInfo.MinRTT` against `WSPingInfo.MinRTT` to get additional evidence
supporing this case.

The times (`BusyTime`, `RWndLimited`, and `SndBufLimited`) are useful to
understand where the bottleneck could be. In general we would like to see
that we've been busy for most of the test runtime. If `RWndLimited` is
large, then it means that the receiver does not have enough
buffering to go faster and it is limiting our performance. Likewise, when
`SndBufLimited` is large, the sender's buffer is too small. Also, if adding up
these three times gives us less time than the duration of the test, it
generally means that the sender was not filling the send buffer fast enough
for keeping TCP busy, thus slowing us down.

The amount of `BytesAcked` combined with the `ElapsedTime` gives us the
average speed at which we've been sending, measured in the kernel. Because
of the measurement unit used, by default the speed will be in bytes per
microsecond. Typically the speed of file transfers is measured instead in
bytes per second, so you need to multiply by `10^6`. To obtain the speed
in bits per second, which is the typical unit with which we measure the
speed of links (e.g. Wi-Fi), also multiply by `8`.

`BytesReceived` is just like `BytesAcked`, except that `BytesAcked` makes
sense for the sender (e.g. the server during the download), while
`BytesReceived` makes sense for the receiver (e.g. the server during
the upload).

`BytesSent` and `BytesRetrans` can be used to compute the percentage
of bytes overall that have been retransmitted. In turn, this is value
approximates the packet loss rate, i.e. the (unknown) probability
with which the network is likely to drop a packet. This approximation
is really bad because it assumes that the probability of dropping a
packet is uniformly distributed, which isn't likely the case. Yet, it
may be an useful first order information to characterise a network
as possibly very lossy. Some packet loss is normal and healthy, but
too much packet loss is the sign of a network path with systemic problems.

### Measuring latency

The presence of TCP-level proxies leads to L7 means being needed to
measure end-to-end latency in addition to end-to-end bandwidth. Such
proxies may include ISP-level performance-enhancing proxies, OpenSSH,
Tor anonymity network and many others.

`WSPingInfo.LastRTT` samples may be affected by the payload during download
and upload tests, as the queue of BinaryMessage may delay either ping
or pong frame. Ping test does not send BinaryMessage payload, so WSPingInfo
RTT measurements should be reasonably accurate (unless it's practical
for the client to delay pong frames). The very first `WSPingInfo` sample
collected during the download test also has a chance to be accurate as
the ping frame SHOULD precede any BinaryMessages in the case.

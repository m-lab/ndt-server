# ndt7 protocol specification

This specification describes version 7 of the Network Diagnostic
Tool (NDT) protocol (ndt7). Ndt7 is a non-backwards compatible
redesign of the [original NDT network performance measurement
protocol](https://github.com/ndt-project/ndt). Ndt7 is based on
WebSocket and TLS, and takes advantage of TCP BBR, where this
flavour of TCP is available.

This is version v0.2.0 of the ndt7 specification.

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
NOT be smaller than 1 << 17 bytes.

Binary WebSocket messages will carry a body composed of random bytes and
will be used to measure the network performance. In the download subtest
these messages are sent by the server to the client. In the upload
subtest the client will send binary messages to the server. If a binary
message is received when it is not expected (i.e. the server receives
a binary message during the download) the connection SHOULD be closed.

Textual WebSocket messages will contain serialized JSON stuctures
containing measurement results (see below). This kind of messages
MAY be sent by both the client and the server throughout the subtest,
regardless of the test type, because both parties run network
measurements they MAY want to share. Note: the bytes exhanged as
part of the textual messages SHOULD themselves be useful to measure
the network performance. An implementation MAY decide to close the
connection if it is receiving Textual WebSocket messages more frequently
than one every 250 millisecond.

When the configured duration time has expired, the parties SHOULD close
the WebSocket channel by sending a Close WebSocket frame. The client
SHOULD not close the TCP connection immediately, so that the server can
close it first. This allows to reuse ports more efficiently on the
server because we avoid `TIME_WAIT`.

## Availability of TCP BBR

If TCP BBR is available, a compliant server MAY choose to enable it
for the client connection and terminate the download test early when
it believes that BBR parameters become stable.

Client can detect whether BBR is enabled by checking whether the measurement
returned by the server contains a `bbr_info` field (see below).

## Query string parameters

The client SHOULD send metadata using the query string. The server
SHOULD process the query string, returning 400 if the query string is
not parseable or not acceptable (see below). The server SHOULD store
the metadata sent by the client using the query string.

The following restrictions apply to the query string. It MUST NOT be
longer than 4096 bytes. Both the name and the value of the query string
parameters MUST match the `[0-9A-Za-z._]+` regular expression.

## Measurements message

As mentioned above, the server and the client exchange JSON measurements
using Textual WebSocket messages. Such JSON measurements have the following
structure:

```json
{
  "elapsed": 1.2345,
  "num_bytes": 17.0,
  "bbr_info": {
    "bandwidth": 12345.4,
    "rtt": 123.4
  }
}
```

Where:

- `elapsed` (a `float64`) is the number of seconds elapsed since the beginning
  of the specific subtest;

- `num_bytes` (a `float64`) is the number of bytes sent (or received) since the
  beginning of the specific subtest;

- `bbr_info` is an optional JSON object only included in the measurement
  when it is possible to access TCP BBR stats;

- `bbr_info.bandwidth` (a `float64`) is the max-bandwidth measured by BBR, in
   bits per second;

- `bbr_info.rtt` (a `float64`) is the min-rtt measured by BBR, in millisecond.

The reason why we always use `float64` (i.e. `double`) for all variables is
that this allows also 32 bit systems to handle such variables easily.

# Reference implementation

The reference implementation is [github.com/m-lab/ndt-cloud](
https://github.com/m-lab/ndt-cloud).

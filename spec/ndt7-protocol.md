# ndt7 protocol specification

This specification describes version 7 of the Network Diagnostic
Tool (NDT) protocol (ndt7). Ndt7 is a non-backwards compatible
redesign of the [original NDT network performance measurement
protocol](https://github.com/ndt-project/ndt). Ndt7 is based on
WebSocket and TLS, and takes advantage of TCP BBR, where this
flavour of TCP is available.

This is version v0.3.0 of the ndt7 specification.

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

Both textual and binary WebSocket messages are allowed. Textual WebSocket
messages will contain serialised JSON structures containing measurements
results (see below). Since both parties MAY in principle perform measurements
during any subtest, both parties MAY send such textual messages.

To generate network load, the party that is currently sending (i.e. the
server during a download subtest) has two options:

1. add a random padding string to each measurement JSON message;

2. send, in addition to textual WebSocket messages, binary WebSocket
   messages carrying random binary data.

The party that is receiving MUST ignore random padding included in
textual messages and binary messages. The party that is sending SHOULD
close the connection if it receives textual messages with padding
or binary messages, since such messages SHOULD only be used by the
party that is sending to generate network load.

The expected transfer time of each subtest is ten seconds (unless BBR
is used, in which case it may be shorter, as explained below). The sender
SHOULD stop sending after ten seconds. The receiver MAY close the
connection if the transfer runs for more than fifteen seconds.

When the expected transfer time has expired, the parties SHOULD close
the WebSocket channel by sending a Close WebSocket frame. The client
SHOULD NOT close the TCP connection immediately, so that the server can
close it first. This allows to reuse ports more efficiently on the
server because we avoid the `TIME_WAIT` TCP state.

## Stopping the transfer earlier using BBR

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
  "bbr_info": {
    "bandwidth": 12345.4,
    "rtt": 123.4
  },
  "elapsed": 1.2345,
  "num_bytes": 17.0,
  "padding": "ABFHFghghghhghgFLLF..."
}
```

Where:

- `bbr_info` is an _optional_ JSON object only included in the measurement
  when it is possible to access TCP BBR stats:

    - `bbr_info.bandwidth` (a `float64`) is the max-bandwidth measured by BBR,
       in bits per second;

    - `bbr_info.rtt` (a `float64`) is the min-rtt measured by BBR,
      in millisecond;

- `elapsed` (a `float64`) is the number of seconds elapsed since the beginning
  of the specific subtest;

- `num_bytes` (a `float64`) is the number of bytes sent (or received) since the
  beginning of the specific subtest;

- `padding` is an _optional_ string containing random uppercase and/or
  lowercase letters that the sending party MAY choose to add to measurement
  messages to generate network load, as explained above.

The reason why we always use `float64` (i.e. `double`) for numeric variables is
that this allows also 32 bit systems to handle such variables easily.

# Reference implementation

The reference implementation is [github.com/m-lab/ndt-cloud](
https://github.com/m-lab/ndt-cloud).

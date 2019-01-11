# Data format

This specification describes how ndt-server serializes ndt7 data
on disk. Other implementations of the ndt7 protocol MAY use other
data serialization formats.

This is version v0.1.0 of the data-format specification.

For each subtest, ndt7 writes on the current working directory a Gzip
compressed JSONL file (i.e. a file where each line is a JSON). The file
name MUST match the following pattern:

```
ndt7-<year>-<month>-<day>T<hour>:<minute>:<second>.<nanoseconds>Z.jsonl.gz
```

The first JSONL file line contains metadata, subsequent lines are measurements.

## The metadata JSON

The first line in the JSONL file contains metadata. It consists of
an object mapping keys to string values. The keys contained in this
JSON are the ones provided by the client in the query string plus
the ones reserved to the server, as specified in the "Query string
parameters" section of [ndt7-protocol.md](ndt7-protocol.md).

The server MUST always include the following variables:

- `"server_local_endpoint"`: the server local IP address and port;

- `"server_name"`: the name of the software implementing the server;

- `"server_remote_endpoint"`: the client IP address and port;

- `"server_subtest"`: whether this is a "download" or "upload" test;

- `"server_version"`: the version of the software implementing the server.

A valid JSON metadata document could look like this:

```JSON
{
  "client_library_name":"libndt7.js",
  "client_library_version":"0.4",
  "server_local_endpoint":"127.0.0.1:443",
  "server_name":"ndt-server",
  "server_remote_endpoint":"127.0.0.1:58142",
  "server_subtest": "download",
  "server_version":"v0.4.0-beta.2-26-ga90a780"
}
```

## Measurements JSON

Subsequent lines in the JSONL file contain measurements. A measurement is a
JSON object containing the following keys:

- `"origin"`: a `string` indicating whether this is a measurement performed
  by the client or by the server;

- `"measurement"`: an `object` containing the fields specified by
  [ndt7-protocol.md](ndt7-protocol.md) in the "Measurements message" section,
  except that the "padding" (if any) MUST be removed from the measurement.

A valid measurement JSON could be:

```JSON
{
  "measurement": {
    "bbr_info": {
      "max_bandwidth": 12345.4,
      "min_rtt": 123.4
    },
    "elapsed": 1.2345,
    "num_bytes": 17.0,
    "tcp_info": {
      "rtt_var": 123.4,
      "smoothed_rtt": 567.8
    }
  },
  "origin": "server"
}
```

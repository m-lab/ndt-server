# Data Format

This specification describes how ndt-server serializes ndt7 data
on disk. Other implementations of the ndt7 protocol MAY use other
data serialization formats.

This is version v0.3.0 of the data-format specification.

For each subtest, ndt7 writes to the current working directory a Gzip
compressed JSON file. The file name MUST match the following pattern:

```
ndt7-<subtest>-<year><month><day>T<hour><minute><second>.<nanoseconds>Z.<uuid>.json.gz
```

The only JSON value contains all metadata and measurements.

## Result JSON

The result JSON value is complete record of the test. It consists of an
object with fields for client and server IP and port, start and end time, and
for ndt7 either an upload or download summary data.

Both upload and download data use the same schema. Only "Upload" is shown below.

```JSON
{
    "GitShortCommit": "773d318",
    "Version": "v0.9.1-20-g773d318",
    "ClientIP": "::1",
    "ClientPort": 40910,
    "ServerIP": "::1",
    "ServerPort": 443,
    "StartTime": "2019-07-16T15:26:05.987748459-04:00",
    "EndTime": "2019-07-16T15:26:16.008714743-04:00",
    "Upload": {
        "StartTime": "2019-07-16T15:26:05.987853779-04:00",
        "EndTime": "2019-07-16T15:26:16.008677965-04:00",
        "UUID": "soltesz99.nyc.corp.google.com_1563200740_unsafe_00000000000157C6",
        "ClientMeasurements": [
        ],
        "ClientMetadata": {
        },
        "ServerMeasurements": [
        ]
    }
}
```

## Client Metadata

The keys contained in the ClientMetadata JSON are the ones provided by the client
in the query string as specified in the "Query string parameters" section of
[ndt7-protocol.md](ndt7-protocol.md).

Valid JSON metadata object in ClientMetadata could look like this:

```JSON
{
  "client_library_name":"libndt7.js",
  "client_library_version":"0.4"
}
```

## Client and Server Measurements

The elements of the ClientMeasurements and ServerMeasurements arrays
represent individual measurements recorded by the client or server.

A measurement is a JSON object containing the fields specified by
[ndt7-protocol.md](ndt7-protocol.md) in the "Measurements message" section,
except that a server MAY choose to remove the "connection_info" optional
object to avoid storing duplicate information.

A valid measurement JSON could be:

```JSON
{
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

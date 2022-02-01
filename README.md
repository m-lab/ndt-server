[![GoDoc](https://godoc.org/github.com/m-lab/ndt-server?status.svg)](https://godoc.org/github.com/m-lab/ndt-server) [![Build Status](https://travis-ci.org/m-lab/ndt-server.svg?branch=master)](https://travis-ci.org/m-lab/ndt-server) [![Coverage Status](https://coveralls.io/repos/github/m-lab/ndt-server/badge.svg?branch=master)](https://coveralls.io/github/m-lab/ndt-server?branch=master) [![Go Report Card](https://goreportcard.com/badge/github.com/m-lab/ndt-server)](https://goreportcard.com/report/github.com/m-lab/ndt-server)

# ndt-server

This repository contains a [ndt5](
https://github.com/ndt-project/ndt/wiki/NDTProtocol) and [ndt7](
spec/ndt7-protocol.md) server written in Go. This code may compile under
many systems, including macOS and Windows, but is specifically designed
and tested for running on Linux 4.17+.

## Clients

Depending on your needs, there are several ways to perform a client measurement
using the NDT7 protocol.

Officially supported by members of M-Lab staff.

* https://github.com/m-lab/ndt7-js (javascript)
* https://github.com/m-lab/ndt7-client-go (golang)

Unofficially supported by members of the M-Lab community.

* https://github.com/m-lab/ndt7-client-ios (swift)
* https://github.com/m-lab/ndt7-client-android (kotlin)
* https://github.com/m-lab/ndt7-client-android-java (java)

## Setup

### Primary setup & running (Linux)

Prepare the runtime environment

```bash
install -d certs datadir
```

To run the server locally, generate local self signed certificates (`key.pem`
and `cert.pem`) using bash and OpenSSL

```bash
./gen_local_test_certs.bash
```

build the docker container for `ndt-server`

```bash
docker build . -t ndt-server
```

enable BBR (with which ndt7 works much better)

```
sudo modprobe tcp_bbr
```

and run the `ndt-server` binary container

```bash
docker run --network=bridge                \
           --publish 443:4443              \
           --publish 80:8080               \
           --volume `pwd`/certs:/certs:ro  \
           --volume `pwd`/datadir:/datadir \
           --read-only                     \
           --user `id -u`:`id -g`          \
           --cap-drop=all                  \
           ndt-server                      \
           -cert /certs/cert.pem           \
           -key /certs/key.pem             \
           -datadir /datadir               \
           -ndt7_addr :4443                \
           -ndt7_addr_cleartext :8080
```

### Alternate setup & running (Windows & MacOS)

These instructions assume you have Docker for Windows/Mac installed.

**Note: NDT5 does not work on Docker for Windows/Mac as it requires using the host's network, which is only supported on Linux**

```
docker-compose run ndt-server ./gen_local_test_certs.bash
docker-compose up
```

After making changes you will have to run `docker-compose up --build` to rebuild the ntd-server binary.

## Accessing the service

Once you have done that, you should have a ndt5 server running on ports
`3001` (legacy binary flavour), `3002` (WebSocket flavour), and `3010`
(secure WebSocket flavour); a ndt7 server running on port `443` (over TLS
and using the ndt7 WebSocket protocol); and Prometheus metrics available
on port `9990`.

Try accessing these URLs in your browser (for URLs using HTTPS, certs will
appear invalid to your browser, but everything is safe because this is a test
deployment, hence you should ignore this warning and continue):

* ndt7: http://localhost/ndt7.html or https://localhost/ndt7.html
* ndt5+wss: https://localhost:3010/widget.html
* prometheus: http://localhost:9090/metrics

Replace `localhost` with the IP of the server to access them externally.

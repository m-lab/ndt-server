[![GoDoc](https://godoc.org/github.com/m-lab/ndt-server?status.svg)](https://godoc.org/github.com/m-lab/ndt-server) [![Build Status](https://travis-ci.org/m-lab/ndt-server.svg?branch=master)](https://travis-ci.org/m-lab/ndt-server) [![Coverage Status](https://coveralls.io/repos/github/m-lab/ndt-server/badge.svg?branch=master)](https://coveralls.io/github/m-lab/ndt-server?branch=master) [![Go Report Card](https://goreportcard.com/badge/github.com/m-lab/ndt-server)](https://goreportcard.com/report/github.com/m-lab/ndt-server)

# ndt-server

To run the server locally, generate local self signed certificates (`key.pem`
and `cert.pem`) using bash and OpenSSL

```bash
./gen_local_test_certs.bash
```

build the docker container for `ndt-server`

```bash
docker build . -t ndt-server
```

prepare the runtime environment

```bash
install -d certs data
mv key.pem cert.pem certs
```

and invoke the `ndt-server` binary container

```bash
docker run --network=bridge                \
           --publish 443:4443              \
           --volume `pwd`/certs:/certs:ro  \
           --volume `pwd`/data:/data       \
           --read-only                     \
           --user `id -u`:`id -g`          \
           --cap-drop=all                  \
           ndt-server                      \
           -cert /certs/cert.pem           \
           -key /certs/key.pem             \
           -datadir /data                  \
           -ndt7_addr :4443
```

Once you have done that, you should have a ndt5 server running on ports
`3001` (cleartext) and `3010` (encrypted), a ndt7 server running on
port `443`), and metrics available on port 9090.

Try running a test in your browser (certs will appear invalid to your
browser, but everything is safe because this is a test deployment):

* ndt5: https://localhost:3001/static/widget.html
* ndt5+tls: https://localhost:3010/static/widget.html
* ndt7: https://localhost/static/ndt7.html
* prometheus: http://localhost:9090/metrics

Replace `localhost` with the IP of the server to access it externally.

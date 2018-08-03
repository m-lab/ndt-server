[![GoDoc](https://godoc.org/github.com/m-lab/ndt-cloud?status.svg)](https://godoc.org/github.com/m-lab/ndt-cloud) [![Build Status](https://travis-ci.org/m-lab/ndt-cloud.svg?branch=master)](https://travis-ci.org/m-lab/ndt-cloud) [![Coverage Status](https://coveralls.io/repos/github/m-lab/ndt-cloud/badge.svg?branch=master)](https://coveralls.io/github/m-lab/ndt-cloud?branch=master) [![Go Report Card](https://goreportcard.com/badge/github.com/m-lab/ndt-cloud)](https://goreportcard.com/report/github.com/m-lab/ndt-cloud)

# ndt-cloud

To run the server locally, first run `gen_local_test_certs.sh`, and then run the
commands
```bash
docker build . -t ndtgo
```
and
```bash
docker run --net=host -v `pwd`:/certs -it -t ndtgo
```

Once you have done that, you should have a server running on port 3010 on
localhost.

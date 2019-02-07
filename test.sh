#!/bin/bash
# Script to test ndt-server with the correct `go get` flags.  This script
# should be run inside a container.
set -ex

GOPATH=/go

# Test the NDT binary
go get -v gopkg.in/m-lab/pipe.v3
go get github.com/m-lab/ndt-server/cmd/ndt-cloud-client
PATH=${PATH}:${GOPATH}/bin
if [[ -z ${TRAVIS_PULL_REQUEST} ]]; then
  go test -v -cover=1 -coverpkg=./... -tags netgo ./...
else
  go test -v -coverprofile=ndt.cov -coverpkg=./... -tags netgo ./...
  go get github.com/mattn/goveralls
  /go/bin/goveralls -coverprofile=ndt.cov -service=travis-ci
fi

#!/bin/bash
# Script to test ndt-server with the correct `go get` flags.  This script
# should be run inside a container.
set -ex

# Test the NDT binary
PATH=${PATH}:${GOPATH}/bin
if [[ -z ${TRAVIS_PULL_REQUEST} ]]; then
  go test -v -cover=1 -coverpkg=./... -tags netgo ./...
else
  go test -v -coverprofile=ndt.cov -coverpkg=./... -tags netgo ./...
  /go/bin/goveralls -coverprofile=ndt.cov -service=travis-ci
fi

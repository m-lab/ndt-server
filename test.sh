#!/bin/bash

GOPATH=$(pwd | sed -e "s#/go/.*#/go#")

# Test the NDT binary
go get -v gopkg.in/m-lab/pipe.v3
go get github.com/m-lab/ndt-server/cmd/ndt-cloud-client
PATH=${PATH}:${GOPATH}/bin
go test -v -coverprofile=ndt.cov -coverpkg=./... -tags netgo ./...

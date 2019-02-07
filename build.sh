#!/bin/sh
# Script to build ndt-server with the correct `go get` flags.
set -ex
TOPDIR=`cd $(dirname $0) && pwd -P`
cd $TOPDIR
VERSION=`git describe --tags`
GOPATH=$(pwd | sed -e "s#/go/.*#/go#")
go get -v                                                              \
    -tags netgo                                                        \
    -ldflags "-X github.com/m-lab/ndt-server/version.Version=$VERSION"  \
    .

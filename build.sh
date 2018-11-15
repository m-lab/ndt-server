#!/bin/sh
# Script to build ndt-cloud with the correct `go get` flags.
set -ex
TOPDIR=`cd $(dirname $0) && pwd -P`
cd $TOPDIR
VERSION=`git describe --tags`
go get -v                                                              \
    -tags netgo                                                        \
    -ldflags "-X github.com/m-lab/ndt-cloud/version.Version=$VERSION"  \
    .

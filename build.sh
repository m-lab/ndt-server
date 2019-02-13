#!/bin/sh
# Script to build ndt-server with the correct `go get` flags.  This script
# was designed and tested to run as part of the container build process.
set -ex

TOPDIR=`cd $(dirname $0) && pwd -P`
cd $TOPDIR
VERSION=`git describe --tags`
go get -v                                                              \
    -tags netgo                                                        \
    -ldflags "-X github.com/m-lab/ndt-server/version.Version=$VERSION"  \
    .

#!/bin/sh
# Script to build ndt-server with the correct `go get` flags.  This script
# should be run inside a container.
set -ex

TOPDIR=`cd $(dirname $0) && pwd -P`
cd $TOPDIR
VERSION=`git describe --tags`
go get -v                                                              \
    -tags netgo                                                        \
    -ldflags "-X github.com/m-lab/ndt-server/version.Version=$VERSION"  \
    .

#!/bin/sh
# Script to build ndt-server with the correct `go get` flags.  This script
# was designed and tested to run as part of the container build process.
set -ex

topdir () {
  cd $(dirname "$0") && pwd -P
}
cd "$(topdir)"

VERSION=$(git describe --tags)
versionflags="-X github.com/m-lab/ndt-server/version.Version=$VERSION"

COMMIT=$(git log -1 --format=%h)
versionflags="${versionflags} -X github.com/m-lab/go/prometheusx.GitShortCommit=${COMMIT}"

go env -w GO111MODULE=auto
go get -v -t                                                           \
    -tags netgo                                                        \
    -ldflags "$versionflags -extldflags \"-static\""                   \
    .

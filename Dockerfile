FROM golang:1.16.7-alpine3.14 as ndt-server-build
RUN apk add --no-cache git gcc linux-headers musl-dev
ADD . /go/src/github.com/m-lab/ndt-server
RUN /go/src/github.com/m-lab/ndt-server/build.sh

# Now copy the built image into the minimal base image
FROM alpine:3.14
COPY --from=ndt-server-build /go/bin/ndt-server /
ADD ./html /html
WORKDIR /
ENTRYPOINT ["/ndt-server"]

FROM golang:1.10
ADD . /go/src/github.com/m-lab/ndt-cloud
RUN go get -v github.com/m-lab/ndt-cloud
ADD ./html /html
WORKDIR /
ENTRYPOINT ["/go/bin/ndt-cloud"]

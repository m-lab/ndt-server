package main

import (
	"flag"
	"io/ioutil"

	"github.com/m-lab/go/cloud/bqx"
	"github.com/m-lab/go/rtx"
	"github.com/m-lab/ndt-server/data"

	"cloud.google.com/go/bigquery"
)

var (
	ndt7schema string
	ndt5schema string
)

func init() {
	flag.StringVar(&ndt7schema, "ndt7", "/var/spool/datatypes/ndt7.json", "filename to write ndt7 schema")
	flag.StringVar(&ndt5schema, "ndt5", "/var/spool/datatypes/ndt5.json", "filename to write ndt5 schema")
}

func main() {
	flag.Parse()
	// Generate and save ndt7 schema for autoloading.
	row7 := data.NDT7Result{}
	sch, err := bigquery.InferSchema(row7)
	rtx.Must(err, "failed to generate ndt7 schema")
	sch = bqx.RemoveRequired(sch)
	b, err := sch.ToJSONFields()
	rtx.Must(err, "failed to marshal schema")
	ioutil.WriteFile(ndt7schema, b, 0o644)

	// Generate and save ndt5 schema for autoloading.
	row5 := data.NDT7Result{}
	sch, err = bigquery.InferSchema(row5)
	rtx.Must(err, "failed to generate ndt5 schema")
	sch = bqx.RemoveRequired(sch)
	b, err = sch.ToJSONFields()
	rtx.Must(err, "failed to marshal schema")
	ioutil.WriteFile(ndt5schema, b, 0o644)
}

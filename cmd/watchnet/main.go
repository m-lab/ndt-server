package main

import (
	"flag"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/prometheus/procfs"
	"src/github.com/m-lab/go/rtx"
)

var (
	procPath = "/proc"
	device   string
)

func init() {
	flag.StringVar(&device, "device", "eno1", "watch device usage to limit ndt connections")
}

type TxWatcher struct {
	Device  string
	current uint64
}

func (w *TxWatcher) Watch(init uint64, pfs procfs.FS) {
	t := time.NewTicker(time.Second)

	for prev := init; ; <-t.C {
		nd, err := pfs.NetDev()
		if err != nil {
			log.Println(err)
			continue
		}
		v, ok := nd[w.Device]
		if ok {
			cur := (v.TxBytes - prev) * 8
			atomic.StoreUint64(&w.current, cur)
			prev = v.TxBytes
		}
	}

}

func main() {
	flag.Parse()

	pfs, err := procfs.NewFS(procPath)
	rtx.Must(err, "Failed to create a new procfs reader")
	nd, err := pfs.NetDev()
	rtx.Must(err, "Failed to get netdev")

	for k, v := range nd {
		fmt.Println(k, v)
	}

	w := TxWatcher{
		Device: device,
	}
	go w.Watch(nd[device].TxBytes, pfs)

	t := time.NewTicker(time.Second)
	for ; ; <-t.C {
		cur := atomic.LoadUint64(&w.current)
		fmt.Println(cur)
	}
}

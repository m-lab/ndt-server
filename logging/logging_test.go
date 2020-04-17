package logging

import (
	"bytes"
	"log"
	"net/http"
	"testing"

	"github.com/m-lab/go/httpx"
	"github.com/m-lab/go/rtx"
)

type fakeHandler struct{}

func (s *fakeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
}

func TestMakeAccessLogHandler(t *testing.T) {
	buff := &bytes.Buffer{}
	old := log.Writer()
	defer func() {
		log.SetOutput(old)
	}()
	log.SetOutput(buff)
	f := MakeAccessLogHandler(&fakeHandler{})
	log.SetOutput(old)
	srv := http.Server{
		Addr:    ":0",
		Handler: f,
	}
	rtx.Must(httpx.ListenAndServeAsync(&srv), "Could not start server")
	defer srv.Close()
	_, err := http.Get("http://" + srv.Addr + "/")
	rtx.Must(err, "Could not get")
	s, err := buff.ReadString('\n')
	if s == "" {
		t.Error("We should not have had an empty string")
	}
}

package main

import (
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"log"
	"net/http"
	"strconv"
	"time"
	"github.com/orian/instrumentation"
)

func ParamFunc(w http.ResponseWriter, req *http.Request) {
	if durS := req.FormValue("dur"); len(durS) > 0 {
		dur, err := strconv.ParseFloat(durS, 32)
		if err != nil {
			log.Printf("cannot parse `dur` query param: %s", err)
			return
		}
		log.Printf("waiting for: %f seconds", dur)
		time.Sleep(time.Duration(dur * float64(time.Second)))
	}
	if codeS := req.FormValue("code"); len(codeS) > 0 {
		code, err := strconv.ParseInt(codeS, 10, 64)
		if err != nil {
			log.Printf("cannot parse `code` query param: %s", err)
			return
		}
		log.Printf("returning code: %d (%s)", code, http.StatusText(int(code)))
		w.WriteHeader(int(code))
	}
	w.Write([]byte("done."))
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", ParamFunc)
	h := instrumentation.InstrumentHandler("test", mux)

	// don't count /metrics to stats
	mux = http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/", h)
	log.Print(http.ListenAndServe(":8080", mux))
}

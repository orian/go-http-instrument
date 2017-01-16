package instrumentation

import (
	"github.com/prometheus/client_golang/prometheus"

	"bufio"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// InstrumentHandler wraps the given HTTP handler for instrumentation. It
// registers four metric collectors (if not already done) and reports HTTP
// metrics to the (newly or already) registered collectors: http_requests_total
// (CounterVec), http_request_duration_seconds (Summary),
// http_request_size_bytes (Summary), http_response_size_bytes (Summary). Each
// has a constant label named "handler" with the provided handlerName as
// value. http_requests_total is a metric vector partitioned by HTTP method
// (label name "method") and HTTP status code (label name "code").
//
// Deprecated: InstrumentHandler has several issues:
//
// - It uses Summaries rather than Histograms. Summaries are not useful if
// aggregation across multiple instances is required.
//
// - The size of the request is calculated in a separate goroutine. Since this
// calculator requires access to the request header, it creates a race with
// any writes to the header performed during request handling.
// httputil.ReverseProxy is a prominent example for a handler
// performing such writes.
//
// Upcoming versions of this package will provide ways of instrumenting HTTP
// handlers that are more flexible and have fewer issues. Please prefer direct
// instrumentation in the meantime.
func InstrumentHandler(handlerName string, handler http.Handler) http.HandlerFunc {
	return InstrumentHandlerFunc(handlerName, handler.ServeHTTP)
}

// InstrumentHandlerFunc wraps the given function for instrumentation. It
// otherwise works in the same way as InstrumentHandler (and shares the same
// issues).
//
// Deprecated: InstrumentHandlerFunc is deprecated for the same reasons as
// InstrumentHandler is.
func InstrumentHandlerFunc(handlerName string, handlerFunc func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return InstrumentHandlerFuncWithOpts(
		prometheus.Opts{
			Subsystem:   "http",
			ConstLabels: prometheus.Labels{"handler": handlerName},
		},
		handlerFunc,
	)
}

// InstrumentHandlerWithOpts works like InstrumentHandler (and shares the same
// issues) but provides more flexibility (at the cost of a more complex call
// syntax). As InstrumentHandler, this function registers four metric
// collectors, but it uses the provided SummaryOpts to create them. However, the
// fields "Name" and "Help" in the SummaryOpts are ignored. "Name" is replaced
// by "requests_total", "request_duration_seconds", "request_size_bytes",
// and "response_size_bytes", respectively. "Help" is replaced by an appropriate
// help string. The names of the variable labels of the http_requests_total
// CounterVec are "method" (get, post, etc.), and "code" (HTTP status code).
//
// If InstrumentHandlerWithOpts is called as follows, it mimics exactly the
// behavior of InstrumentHandler:
//
//     prometheus.InstrumentHandlerWithOpts(
//         prometheus.SummaryOpts{
//              Subsystem:   "http",
//              ConstLabels: prometheus.Labels{"handler": handlerName},
//         },
//         handler,
//     )
//
// Technical detail: "requests_total" is a CounterVec, not a SummaryVec, so it
// cannot use SummaryOpts. Instead, a CounterOpts struct is created internally,
// and all its fields are set to the equally named fields in the provided
// SummaryOpts.
//
// Deprecated: InstrumentHandlerWithOpts is deprecated for the same reasons as
// InstrumentHandler is.
func InstrumentHandlerWithOpts(opts prometheus.Opts, handler http.Handler) http.HandlerFunc {
	return InstrumentHandlerFuncWithOpts(opts, handler.ServeHTTP)
}

var instLabels = []string{"method", "code"}

// InstrumentHandlerFuncWithOpts works like InstrumentHandlerFunc (and shares
// the same issues) but provides more flexibility (at the cost of a more complex
// call syntax). See InstrumentHandlerWithOpts for details how the provided
// Opts are used.
//
// Deprecated: InstrumentHandlerFuncWithOpts is deprecated for the same reasons
// as InstrumentHandler is.
func InstrumentHandlerFuncWithOpts(opts prometheus.Opts, handlerFunc func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	inFlightReq := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: opts.Namespace,
			Subsystem: opts.Subsystem,
			Name:      "inflight_requests",
			Help:      "In-flight HTTP requests.",
		},
		[]string{"method"},
	)
	if err := prometheus.Register(inFlightReq); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			inFlightReq = are.ExistingCollector.(*prometheus.GaugeVec)
		} else {
			panic(err)
		}
	}

	reqCnt := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   opts.Namespace,
			Subsystem:   opts.Subsystem,
			Name:        "requests_total",
			Help:        "Total number of HTTP requests made.",
			ConstLabels: opts.ConstLabels,
		},
		instLabels,
	)
	if err := prometheus.Register(reqCnt); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			reqCnt = are.ExistingCollector.(*prometheus.CounterVec)
		} else {
			panic(err)
		}
	}

	reqDur := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: opts.Namespace,
		Subsystem: opts.Subsystem,
		Name:      "request_duration_seconds",
		Help:      "The HTTP request latencies in seconds.",
		Buckets: []float64{0.05, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7,
			0.8, 0.9, 1., 2., 5., 10., 20., 30., 40., 50.},
	})
	if err := prometheus.Register(reqDur); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			reqDur = are.ExistingCollector.(prometheus.Histogram)
		} else {
			panic(err)
		}
	}

	//opts.Name = "request_size_bytes"
	//opts.Help = "The HTTP request sizes in bytes."
	//reqSz := prometheus.NewSummary(opts)
	//if err := prometheus.Register(reqSz); err != nil {
	//	if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
	//		reqSz = are.ExistingCollector.(prometheus.Summary)
	//	} else {
	//		panic(err)
	//	}
	//}

	resSz := prometheus.NewSummary(prometheus.SummaryOpts{
		Namespace:   opts.Namespace,
		Subsystem:   opts.Subsystem,
		Name:        "response_size_bytes",
		Help:        "The HTTP response sizes in bytes.",
		ConstLabels: opts.ConstLabels,
		Objectives:  map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
	})
	if err := prometheus.Register(resSz); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			resSz = are.ExistingCollector.(prometheus.Summary)
		} else {
			panic(err)
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		method := sanitizeMethod(r.Method)
		f := inFlightReq.WithLabelValues(r.Method)
		f.Inc()
		defer f.Dec()

		delegate := &responseWriterDelegator{ResponseWriter: w}
		//out := computeApproximateRequestSize(r)

		_, cn := w.(http.CloseNotifier)
		_, fl := w.(http.Flusher)
		_, hj := w.(http.Hijacker)
		_, rf := w.(io.ReaderFrom)
		var rw http.ResponseWriter
		if cn && fl && hj && rf {
			rw = &fancyResponseWriterDelegator{delegate}
		} else {
			rw = delegate
		}
		handlerFunc(rw, r)

		elapsed := float64(time.Since(now)) / float64(time.Second)
		code := sanitizeCode(delegate.status)
		reqCnt.WithLabelValues(method, code).Inc()
		reqDur.Observe(elapsed)
		resSz.Observe(float64(delegate.written))
		//reqSz.Observe(float64(<-out))
	})
}

type responseWriterDelegator struct {
	http.ResponseWriter

	handler, method string
	status          int
	written         int64
	wroteHeader     bool
}

func (r *responseWriterDelegator) WriteHeader(code int) {
	r.status = code
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseWriterDelegator) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	n, err := r.ResponseWriter.Write(b)
	r.written += int64(n)
	return n, err
}

type fancyResponseWriterDelegator struct {
	*responseWriterDelegator
}

func (f *fancyResponseWriterDelegator) CloseNotify() <-chan bool {
	return f.ResponseWriter.(http.CloseNotifier).CloseNotify()
}

func (f *fancyResponseWriterDelegator) Flush() {
	f.ResponseWriter.(http.Flusher).Flush()
}

func (f *fancyResponseWriterDelegator) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return f.ResponseWriter.(http.Hijacker).Hijack()
}

func (f *fancyResponseWriterDelegator) ReadFrom(r io.Reader) (int64, error) {
	if !f.wroteHeader {
		f.WriteHeader(http.StatusOK)
	}
	n, err := f.ResponseWriter.(io.ReaderFrom).ReadFrom(r)
	f.written += n
	return n, err
}

func computeApproximateRequestSize(r *http.Request) <-chan int {
	// Get URL length in current go routine for avoiding a race condition.
	// HandlerFunc that runs in parallel may modify the URL.
	s := 0
	if r.URL != nil {
		s += len(r.URL.String())
	}

	out := make(chan int, 1)

	go func() {
		s += len(r.Method)
		s += len(r.Proto)
		for name, values := range r.Header {
			s += len(name)
			for _, value := range values {
				s += len(value)
			}
		}
		s += len(r.Host)

		// N.B. r.Form and r.MultipartForm are assumed to be included in r.URL.

		if r.ContentLength != -1 {
			s += int(r.ContentLength)
		}
		out <- s
		close(out)
	}()

	return out
}

func sanitizeMethod(m string) string {
	switch m {
	case "GET", "get":
		return "get"
	case "PUT", "put":
		return "put"
	case "HEAD", "head":
		return "head"
	case "POST", "post":
		return "post"
	case "DELETE", "delete":
		return "delete"
	case "CONNECT", "connect":
		return "connect"
	case "OPTIONS", "options":
		return "options"
	case "NOTIFY", "notify":
		return "notify"
	default:
		return strings.ToLower(m)
	}
}

func sanitizeCode(s int) string {
	switch s {
	case 100:
		return "100"
	case 101:
		return "101"

	case 200:
		return "200"
	case 201:
		return "201"
	case 202:
		return "202"
	case 203:
		return "203"
	case 204:
		return "204"
	case 205:
		return "205"
	case 206:
		return "206"

	case 300:
		return "300"
	case 301:
		return "301"
	case 302:
		return "302"
	case 304:
		return "304"
	case 305:
		return "305"
	case 307:
		return "307"

	case 400:
		return "400"
	case 401:
		return "401"
	case 402:
		return "402"
	case 403:
		return "403"
	case 404:
		return "404"
	case 405:
		return "405"
	case 406:
		return "406"
	case 407:
		return "407"
	case 408:
		return "408"
	case 409:
		return "409"
	case 410:
		return "410"
	case 411:
		return "411"
	case 412:
		return "412"
	case 413:
		return "413"
	case 414:
		return "414"
	case 415:
		return "415"
	case 416:
		return "416"
	case 417:
		return "417"
	case 418:
		return "418"

	case 500:
		return "500"
	case 501:
		return "501"
	case 502:
		return "502"
	case 503:
		return "503"
	case 504:
		return "504"
	case 505:
		return "505"

	case 428:
		return "428"
	case 429:
		return "429"
	case 431:
		return "431"
	case 511:
		return "511"

	default:
		return strconv.Itoa(s)
	}
}

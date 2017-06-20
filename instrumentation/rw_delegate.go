package instrumentation

import (
	"bufio"
	"io"
	"net"
	"net/http"
)

type stats struct {
	handler, method string
	status          int
	written         int64
	wroteHeader     bool
}

type rwShared struct {
	inner http.ResponseWriter
	stats *stats
}

type rwH1 struct{ rwShared }
type rwH2 struct{ rwShared }

type stringWriter interface {
	WriteString(s string) (n int, err error)
}

func (r *rwShared) Header() http.Header {
	return r.inner.Header()
}

func (r *rwShared) WriteHeader(code int) {
	r.stats.status = code
	r.stats.wroteHeader = true
	r.inner.WriteHeader(code)
}

func (r *rwShared) Write(b []byte) (int, error) {
	if !r.stats.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	n, err := r.inner.Write(b)
	r.stats.written += int64(n)
	return n, err
}

func (r *rwShared) WriteString(s string) (int, error) {
	if !r.stats.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	n, err := r.inner.(stringWriter).WriteString(s)
	r.stats.written += int64(n)
	return n, err
}

func (r *rwShared) CloseNotify() <-chan bool {
	return r.inner.(http.CloseNotifier).CloseNotify()
}

func (r *rwShared) Flush() {
	r.inner.(http.Flusher).Flush()
}

func (r *rwH1) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return r.inner.(http.Hijacker).Hijack()
}

func (r *rwH1) ReadFrom(reader io.Reader) (int64, error) {
	if !r.stats.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	n, err := r.inner.(io.ReaderFrom).ReadFrom(reader)
	r.stats.written += n
	return n, err
}

func (r *rwH2) Push(target string, opts *http.PushOptions) error {
	return r.inner.(http.Pusher).Push(target, opts)
}

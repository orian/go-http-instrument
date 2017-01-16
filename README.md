# Simple Go `net/http` instrumentation for Prometheus

This library is a fork of Prometheus golang_client/prometheus/http handler proxy:
https://github.com/prometheus/client_golang/blob/master/prometheus/http.go#L152

The documentation of `InstrumentHandler` mentions the following problems:
```
 - It uses Summaries rather than Histograms. Summaries are not useful if
 aggregation across multiple instances is required.

 - It uses microseconds as unit, which is deprecated and should be replaced by
 seconds.

 - The size of the request is calculated in a separate goroutine. Since this
 calculator requires access to the request header, it creates a race with
 any writes to the header performed during request handling.
 httputil.ReverseProxy is a prominent example for a handler
 performing such writes.

```

The first two problems are mitigated. The third one is hard and as a moment of writing 
I didn't find a need to implement it, thus for a sake of race condition detection it
was disabled.

Additionally a gauge `inflight_requests` was added.

The histogram buckets: 

```
    []float64{0.05, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7,
              0.8, 0.9, 1., 2., 5., 10., 20., 30., 40., 50.}
```

## Get the library:
```
go get github.com/orian/go-http-instrument/instrumentation
```

## For developers
For a testing purposes I've prepared a super simple http server to run in Docker. 

The `bin` directory contains a trivial binary. To compile:

```
cd bin
CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o simpleserver simpleserver.go
```

Than one needs to execute:

```
docker build -t instrument/demoserver .
docker run --rm -it --net promnet -p 8080:8080 \
    --name httpexample instrument/demoserver
```

Prometheus:
```
docker pull prom/prometheus

docker run --rm -it -p 9090:9090 --net promnet \ 
    --name prom -v $PWD/prom/prometheus.yaml:/etc/prometheus/prometheus.yml \
    prom/prometheus
```

To see changes in metrics run some queries. The demo server accepts two query params:
  
  - `dur` - how long the request should take.
  - `code` - what HTTP status code to return.

```
curl -v "localhost:8080/" &
curl -v "localhost:8080/?code=404&dur=1.5" &
curl -v "localhost:8080/?code=404&dur=15" &
curl -v "localhost:8080/?dur=105" &
curl -v "localhost:8080/?code=500&dur=50" & 
curl -v "localhost:8080/?dur=34" &
curl -v "localhost:8080/?dur=20" &
```
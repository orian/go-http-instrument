docker run --rm -it -p 9090:9090 --net promnet --name prom -v /Users/orian/workspace/go/gopath/src/github.com/orian/instrumentation/prometheus.yaml:/etc/prometheus/prometheus.yml prom/prometheus
curl -v "localhost:8080/?code=404"

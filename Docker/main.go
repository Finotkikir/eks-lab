package main

import (
"encoding/json"
"net/http"
"os"
"strings"

"github.com/gorilla/mux"
"github.com/prometheus/client_golang/prometheus"
"github.com/prometheus/client_golang/prometheus/promauto"
"github.com/prometheus/client_golang/prometheus/promhttp"
negroni "github.com/urfave/negroni/v3"
simpleredis "github.com/xyproto/simpleredis/v2"
)

var (
masterPool *simpleredis.ConnectionPool

// Prometheus metrics
httpRequestsTotal = promauto.NewCounterVec(
prometheus.CounterOpts{
Name: "http_requests_total",
Help: "Total number of HTTP requests",
},
[]string{"method", "endpoint"},
)

httpRequestDuration = promauto.NewHistogramVec(
prometheus.HistogramOpts{
Name:    "http_request_duration_seconds",
Help:    "Duration of HTTP requests in seconds",
Buckets: prometheus.DefBuckets,
},
[]string{"method", "endpoint"},
)

redisOperationsTotal = promauto.NewCounterVec(
prometheus.CounterOpts{
Name: "redis_operations_total",
Help: "Total number of Redis operations",
},
[]string{"operation"},
)
)

func ListRangeHandler(rw http.ResponseWriter, req *http.Request) {
timer := prometheus.NewTimer(httpRequestDuration.WithLabelValues(req.Method, "/lrange"))
defer timer.ObserveDuration()

key := mux.Vars(req)["key"]
list := simpleredis.NewList(masterPool, key)
members := HandleError(list.GetAll()).([]string)
membersJSON := HandleError(json.MarshalIndent(members, "", "  ")).([]byte)
rw.Write(membersJSON)

redisOperationsTotal.WithLabelValues("lrange").Inc()
}

func ListPushHandler(rw http.ResponseWriter, req *http.Request) {
timer := prometheus.NewTimer(httpRequestDuration.WithLabelValues(req.Method, "/rpush"))
defer timer.ObserveDuration()

key := mux.Vars(req)["key"]
value := mux.Vars(req)["value"]
list := simpleredis.NewList(masterPool, key)
HandleError(nil, list.Add(value))
ListRangeHandler(rw, req)

redisOperationsTotal.WithLabelValues("rpush").Inc()
}

func InfoHandler(rw http.ResponseWriter, req *http.Request) {
timer := prometheus.NewTimer(httpRequestDuration.WithLabelValues(req.Method, "/info"))
defer timer.ObserveDuration()

info := HandleError(masterPool.Get(0).Do("INFO")).([]byte)
rw.Write(info)

redisOperationsTotal.WithLabelValues("info").Inc()
}

func EnvHandler(rw http.ResponseWriter, req *http.Request) {
timer := prometheus.NewTimer(httpRequestDuration.WithLabelValues(req.Method, "/env"))
defer timer.ObserveDuration()

environment := make(map[string]string)
for _, item := range os.Environ() {
splits := strings.Split(item, "=")
key := splits[0]
val := strings.Join(splits[1:], "=")
environment[key] = val
}

envJSON := HandleError(json.MarshalIndent(environment, "", "  ")).([]byte)
rw.Write(envJSON)
}

func HealthHandler(rw http.ResponseWriter, req *http.Request) {
timer := prometheus.NewTimer(httpRequestDuration.WithLabelValues(req.Method, "/healthz"))
defer timer.ObserveDuration()

if err := masterPool.Ping(); err != nil {
rw.WriteHeader(http.StatusInternalServerError)
rw.Write([]byte(err.Error()))
} else {
rw.WriteHeader(http.StatusOK)
}

redisOperationsTotal.WithLabelValues("ping").Inc()
}

func HandleError(result interface{}, err error) (r interface{}) {
if err != nil {
panic(err)
}
return result
}

func main() {
masterPool = simpleredis.NewConnectionPoolHost(os.Getenv("REDIS_HOST") + ":6379")
defer masterPool.Close()

r := mux.NewRouter()
r.Path("/lrange/{key}").Methods("GET").HandlerFunc(ListRangeHandler)
r.Path("/rpush/{key}/{value}").Methods("GET").HandlerFunc(ListPushHandler)
r.Path("/info").Methods("GET").HandlerFunc(InfoHandler)
r.Path("/env").Methods("GET").HandlerFunc(EnvHandler)
r.Path("/healthz").Methods("GET").HandlerFunc(HealthHandler)

// Add Prometheus metrics endpoint
r.Path("/metrics").Handler(promhttp.Handler())

n := negroni.Classic()
n.UseHandler(r)

// Middleware to count total HTTP requests
n.Use(negroni.HandlerFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
httpRequestsTotal.WithLabelValues(r.Method, r.URL.Path).Inc()
next(w, r)
}))

n.Run(":3000")
}
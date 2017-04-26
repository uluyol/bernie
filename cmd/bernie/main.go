package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

var (
	debug    = flag.Bool("debug", false, "should enable debugging logs")
	port     = flag.Int("port", 0, "port to serve on")
	maxTries = flag.Int("maxtries", 4, "max allowable tries for a task")
	maxFails = flag.Int("maxfailures", 3, "max allowed failures on worker")
)

func main() {
	flag.Parse()

	logger := logrus.New()
	logger.Formatter = &logrus.TextFormatter{
		DisableColors: true,
	}
	if *debug {
		logger.Level = logrus.DebugLevel
	}
	handler := handler{
		log: logger.WithField("elem", "http-handler"),
		bernie: bernieServer{
			log: logger.WithField("elem", "bernie"),
		},
	}
	handler.bernie.init()

	httpLW := logger.WithField("elem", "http").Writer()
	defer httpLW.Close()
	r := mux.NewRouter()
	httpServer := &http.Server{
		Handler:  r,
		ErrorLog: log.New(httpLW, "", 0),
	}
	r.HandleFunc("/", handler.rootHandler).Methods("GET")
	r.HandleFunc("/groups/add", handler.groupsAddHandler).Methods("POST")
	r.HandleFunc("/tasks/{group}/add", handler.tasksAddHandler).Methods("POST")
	r.HandleFunc("/tasks/{group}/{task}", handler.tasksDeleteHandler).Methods("DELETE")
	r.HandleFunc("/tasks/{group}/{task}", handler.tasksPatchHandler).Methods("PATCH")
	r.HandleFunc("/tasks/{group}/{task}/out", handler.tasksOutHandler).Methods("GET")
	r.HandleFunc("/workers/{group}/add", handler.workersAddHandler).Methods("POST")
	r.HandleFunc("/workers/{group}/{worker}", handler.workersDeleteHandler).Methods("DELETE")
	r.HandleFunc("/workers/{group}/{worker}", handler.workersPatchHandler).Methods("PATCH")
	r.HandleFunc("/workers/{group}/{worker}/manifest", handler.workersManifestHandler).Methods("GET")
	r.HandleFunc("/workers/{group}/{worker}/initout", handler.workersInitOutHandler).Methods("GET")
	l, err := net.Listen("tcp", ":"+strconv.Itoa(*port))
	if err != nil {
		logger.Fatalf("failed to listen on port %d: %v", *port, err)
	}
	defer l.Close()
	logger.Infof("listening on %s", l.Addr())
	httpServer.Serve(l)
}

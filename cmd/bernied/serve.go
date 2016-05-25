package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/uluyol/bernie/cmd/internal"
)

var (
	port    = flag.Int("port", 0, "port to serve on")
	context = flag.String("ctx", "bernie-"+internal.RandStr(5), "name of this instance")
	muxAddr = flag.String("mux", "", "location of the mux (optional)")
)

var procManager struct {
	mu    sync.RWMutex
	Procs map[string]*Proc
}

func main() {
	procManager.Procs = make(map[string]*Proc)
	prefix := "/bernie/" + *context
	http.HandleFunc("/", rootHandler)
	http.HandleFunc(prefix+"/", rootHandler)
	http.HandleFunc(prefix+"/newproc/", newprocHandler)
	http.HandleFunc(prefix+"/proc/", procHandler)
	http.HandleFunc(prefix+"/run/", runHandler)
	http.HandleFunc(prefix+"/kill/", killHandler)
	http.HandleFunc(prefix+"/out/", outHandler)
	l, err := net.Listen("tcp", ":"+strconv.Itoa(*port))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start listening: %v\n", err)
		os.Exit(2)
	}
	defer l.Close()
	fmt.Fprintf(os.Stderr, "listening on %s\n", l.Addr())
	if *muxAddr != "" {
		tcpAddr := l.Addr().(*net.TCPAddr)
		u, err := url.Parse(*muxAddr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to parse url: %v\n", err)
			os.Exit(3)
		}
		u.Path = path.Join(u.Path, "register/")
		u.RawQuery = "context=" + *context + "&port=" + strconv.Itoa(tcpAddr.Port)
		r := http.Request{
			Method: "GET",
			URL:    u,
		}
		resp, err := http.DefaultClient.Do(&r)
		if err != nil {
			fmt.Fprintf(os.Stderr, "was not able to register with mux: %v\n", err)
			os.Exit(4)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			fmt.Fprintf(os.Stderr, "couldn't register with mux: got status: %v\n", resp.StatusCode)
			os.Exit(5)
		}
	}
	http.Serve(l, nil)
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	procManager.mu.RLock()
	defer procManager.mu.RUnlock()
	rootTempl.Execute(w, struct {
		Context string
		Procs   map[string]*Proc
	}{
		Context: *context,
		Procs:   procManager.Procs,
	})
}

func getProcName(prefix, path string) (string, bool) {
	procNameSlice := strings.Split(path[len(prefix):], "/")
	if len(procNameSlice) < 1 {
		return "", false
	}
	return procNameSlice[0], true
}

func procHandler(w http.ResponseWriter, r *http.Request) {
	procName, ok := getProcName(procDir(), r.URL.Path)

	if !ok {
		http.Error(w, "need to provide proc name", http.StatusBadRequest)
		return
	}

	procManager.mu.RLock()
	defer procManager.mu.RUnlock()

	p, ok := procManager.Procs[procName]
	if !ok {
		http.Error(w, "proc no longer exists", http.StatusNotFound)
		return
	}
	viewTempl.Execute(w, struct {
		*Proc
		Context string
	}{
		Proc:    p,
		Context: *context,
	})
}

func newprocHandler(w http.ResponseWriter, r *http.Request) {
	dec := json.NewDecoder(r.Body)
	var procReq internal.ProcReq
	if err := dec.Decode(&procReq); err != nil {
		http.Error(w, "error decoding request", http.StatusBadRequest)
		return
	}

	procManager.mu.Lock()
	defer procManager.mu.Unlock()

	if _, ok := procManager.Procs[procReq.Name]; ok {
		http.Error(w, "proc already exists", http.StatusConflict)
		return
	}

	procManager.Procs[procReq.Name] = &Proc{
		Name: procReq.Name,
		Cmd:  procReq.Cmd,
		Env:  procReq.Env,
		PWD:  procReq.PWD,
	}

	if procReq.NoStart {
		return
	}

	procManager.Procs[procReq.Name].Start()
	w.Write([]byte("success"))
}

func procDir() string {
	return "/bernie/" + *context + "/proc/"
}

func runHandler(w http.ResponseWriter, r *http.Request) {
	prefix := "/bernie/" + *context + "/run/"
	procName, ok := getProcName(prefix, r.URL.Path)

	if !ok {
		http.Error(w, "need to provide proc name", http.StatusBadRequest)
		return
	}

	procManager.mu.RLock()
	defer procManager.mu.RUnlock()
	p, ok := procManager.Procs[procName]
	if !ok {
		http.Error(w, "proc no longer exists", http.StatusNotFound)
		return
	}
	p.Start()
	http.Redirect(w, r, procDir()+procName+"/", http.StatusSeeOther)
}

func killHandler(w http.ResponseWriter, r *http.Request) {
	prefix := "/bernie/" + *context + "/kill/"
	procName, ok := getProcName(prefix, r.URL.Path)

	if !ok {
		http.Error(w, "need to provide proc name", http.StatusBadRequest)
		return
	}

	procManager.mu.RLock()
	defer procManager.mu.RUnlock()
	p, ok := procManager.Procs[procName]
	if !ok {
		http.Error(w, "proc no longer exists", http.StatusNotFound)
		return
	}
	p.Kill()
	http.Redirect(w, r, procDir()+procName+"/", http.StatusSeeOther)
}

func outHandler(w http.ResponseWriter, r *http.Request) {
	prefix := "/bernie/" + *context + "/out/"
	procName, ok := getProcName(prefix, r.URL.Path)

	if !ok {
		http.Error(w, "need to provide proc name", http.StatusBadRequest)
		return
	}

	procManager.mu.RLock()
	defer procManager.mu.RUnlock()
	p, ok := procManager.Procs[procName]
	if !ok {
		http.Error(w, "proc no longer exists", http.StatusNotFound)
		return
	}
	p.ServeHTTP(w, r)
}

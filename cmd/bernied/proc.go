package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"os/exec"
	"strconv"
	"sync"

	"github.com/yhat/wsutil"
)

type Proc struct {
	Name string

	Cmd []string
	Env []string // key=value
	PWD string

	mu      sync.RWMutex
	running bool
	execCmd *exec.Cmd
	proxy   http.Handler
	wsProxy http.Handler
}

func (p *Proc) ProcPath() string {
	return fmt.Sprintf("/bernie/%s/proc/%s/", *context, p.Name)
}

func (p *Proc) OutPath() string {
	return fmt.Sprintf("/bernie/%s/out/%s/", *context, p.Name)
}

func (p *Proc) RunPath() string {
	return fmt.Sprintf("/bernie/%s/run/%s/", *context, p.Name)
}

func (p *Proc) KillPath() string {
	return fmt.Sprintf("/bernie/%s/kill/%s/", *context, p.Name)
}

func (p *Proc) IsRunning() bool {
	p.mu.RLock()
	b := p.running
	p.mu.RUnlock()
	return b
}

func pickPort() int {
	const bottom = 49152
	const top = 65535
	return bottom + (rand.Int() % (top - bottom))
}

func (p *Proc) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running {
		return
	}

	gottyPort := strconv.Itoa(pickPort())
	args := make([]string, len(p.Cmd)+2)
	args[0] = "-p"
	args[1] = gottyPort
	copy(args[2:], p.Cmd)

	cmd := exec.Command("gotty", args...)
	if err := cmd.Start(); err != nil {
		log.Printf("failed to start process correctly: %v", err)
		return
	}

	director := func(r *http.Request) {
		r.URL.Scheme = "http"
		r.URL.Host = "127.0.0.1:" + gottyPort
		prefix := p.OutPath()
		prefix = prefix[:len(prefix)-1] // remove trailing slash
		r.URL.Path = r.URL.Path[len(prefix):]
	}
	p.proxy = &httputil.ReverseProxy{Director: director}
	p.wsProxy = &wsutil.ReverseProxy{Director: director}
	p.execCmd = cmd
	p.running = true
}

func (p *Proc) Kill() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.execCmd.Process.Kill(); err != nil {
		log.Printf("failed to kill process: %v", err)
		return
	}
	if err := p.execCmd.Process.Release(); err != nil {
		log.Printf("failed to release proc resources: %v", err)
		// can proceed
	}
	p.running = false
	p.proxy = nil
	p.execCmd = nil
}

func (p *Proc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// actually copy proxies so that a websocket conn doesn't
	// block mutations
	p.mu.RLock()
	wsProxy := p.wsProxy
	proxy := p.proxy
	p.mu.RUnlock()
	if wsutil.IsWebSocketRequest(r) {
		if wsProxy == nil {
			w.Write([]byte("proc not running"))
			return
		}
		wsProxy.ServeHTTP(w, r)
		return
	}
	if proxy == nil {
		w.Write([]byte("proc not running"))
		return
	}
	proxy.ServeHTTP(w, r)
}

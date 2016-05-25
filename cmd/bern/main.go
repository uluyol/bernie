package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/uluyol/bernie/cmd/internal"
)

var (
	name       = flag.String("name", "", "name of process, defaults to name + random identifier")
	autostart  = flag.Bool("autostart", true, "start process on creation")
	bernieAddr = flag.String("addr", "http://127.0.0.1:8080", "address of bernie")
)

func usage() {
	log.Printf("usage %s [options] bernieName command...", os.Args[0])
	flag.PrintDefaults()
}

func main() {
	log.SetPrefix("bern: ")
	log.SetFlags(0)
	flag.Usage = usage
	flag.Parse()
	if flag.NArg() < 2 {
		usage()
		os.Exit(1)
	}
	bernie := flag.Arg(0)
	command := flag.Args()[1:]
	if *name == "" {
		*name = command[0] + "-" + internal.RandStr(5)
	}
	wd, err := os.Getwd()
	if err != nil {
		log.Printf("error getting wd: %v", err)
		os.Exit(1)
	}
	procReq := internal.ProcReq{
		Name:    *name,
		Cmd:     command,
		Env:     os.Environ(),
		PWD:     wd,
		NoStart: !*autostart,
	}

	data, err := json.Marshal(&procReq)
	if err != nil {
		log.Printf("error marshalling request: %v", err)
		os.Exit(2)
	}
	url := *bernieAddr + "/bernie/" + bernie + "/newproc/"
	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		log.Printf("error sending request: %v", err)
		os.Exit(4)
	}
	if resp.StatusCode != http.StatusOK {
		log.Printf("request did not succeed: got status %v", resp.Status)
		os.Exit(5)
	}
}

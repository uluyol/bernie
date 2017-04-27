package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/google/subcommands"
	"github.com/uluyol/bernie/internal"
)

var (
	group string
	addr  string
)

type Task struct {
	Name string   `json:"name"`
	Cmd  []string `json:"cmd"`
	Env  []string `json:"env"`
	WD   string   `json:"wd"`
}

type GroupAddReq struct {
	Name string `json:"name'`
	Init Task   `json:"init"`
}

type TasksAddReq struct {
	Tasks []Task `json:"tasks"`
}

type Worker struct {
	Manifest string `json:"manifest"`
}

type WorkersAddReq struct {
	Workers []Worker `json:"workers"`
}

type groupAddCmd struct {
	wd string
}

func (c *groupAddCmd) Name() string     { return "group-add" }
func (c *groupAddCmd) Synopsis() string { return "add a new group" }
func (c *groupAddCmd) Usage() string    { return "bern group-add [options] initcmd args...\n" }

func (c *groupAddCmd) SetFlags(fs *flag.FlagSet) {
	fs.StringVar(&c.wd, "wd", "", "working directory for init task, empty for current dir")
}

func (c *groupAddCmd) Execute(ctx context.Context, fs *flag.FlagSet, args ...interface{}) subcommands.ExitStatus {
	if fs.NArg() == 0 {
		log.Print("no init command specified")
		return subcommands.ExitUsageError
	}

	refURL, err := url.Parse(addr)
	if err != nil {
		log.Printf("invalid addr: %v", err)
		return subcommands.ExitUsageError
	}

	wd := c.wd
	if wd == "" {
		t, err := os.Getwd()
		if err != nil {
			log.Printf("unable to get working dir: %v", err)
			return subcommands.ExitFailure
		}
		wd = t
	}

	req := GroupAddReq{
		Name: group,
		Init: Task{
			Name: "init",
			Cmd:  fs.Args(),
			Env:  os.Environ(),
			WD:   wd,
		},
	}

	b, err := json.Marshal(&req)
	if err != nil {
		log.Printf("unable to encode request: %v", err)
		return subcommands.ExitFailure
	}

	u, err := refURL.Parse("groups/add")
	if err != nil {
		log.Printf("unable to construct request url: %v", err)
		return subcommands.ExitFailure
	}

	resp, err := http.Post(u.String(), "application/json", bytes.NewReader(b))
	if err != nil {
		log.Printf("unable to issue POST request: %v", err)
		return subcommands.ExitFailure
	}
	var buf bytes.Buffer
	_, err = io.Copy(&buf, resp.Body)
	if err != nil {
		log.Printf("error while reading body: %v", err)
		return subcommands.ExitFailure
	}
	data := make(map[string]interface{})
	if err := json.Unmarshal(buf.Bytes(), &data); err != nil {
		log.Printf("unable to decode response: %v", err)
		return subcommands.ExitFailure
	}

	io.Copy(os.Stdout, &buf)
	if v := data["success"]; v == nil || v != true {
		return subcommands.ExitFailure
	}

	return subcommands.ExitSuccess
}

type taskAddCmd struct {
	name string
	wd   string
}

func (c *taskAddCmd) Name() string     { return "task-add" }
func (c *taskAddCmd) Synopsis() string { return "create a new task" }
func (c *taskAddCmd) Usage() string {
	return `bern task-add [options] cmd args...

Create a task in the group and schedule it for execution.
The task will inherit the environment of this process (use env -i to reset this)
and will run in the current directory by default.
`
}

func (c *taskAddCmd) SetFlags(fs *flag.FlagSet) {
	fs.StringVar(&c.name, "name", "", "name to give the task, by default cmd-RANDSTR")
	fs.StringVar(&c.wd, "wd", "", "working directory for the task, empty for current dir")
}

func (c *taskAddCmd) Execute(ctx context.Context, fs *flag.FlagSet, args ...interface{}) subcommands.ExitStatus {
	if fs.NArg() == 0 {
		log.Print("no command specified")
		return subcommands.ExitUsageError
	}

	refURL, err := url.Parse(addr)
	if err != nil {
		log.Printf("invalid addr: %v", err)
		return subcommands.ExitUsageError
	}

	wd := c.wd
	if wd == "" {
		t, err := os.Getwd()
		if err != nil {
			log.Printf("unable to get working dir: %v", err)
			return subcommands.ExitFailure
		}
		wd = t
	}

	name := c.name
	if name == "" {
		name = fs.Arg(0) + "-" + internal.Base62(rand.Int31())
	}

	req := TasksAddReq{
		Tasks: []Task{
			{
				Name: name,
				Cmd:  fs.Args(),
				Env:  os.Environ(),
				WD:   wd,
			},
		},
	}

	b, err := json.Marshal(&req)
	if err != nil {
		log.Printf("unable to encode request: %v", err)
		return subcommands.ExitFailure
	}

	u, err := refURL.Parse("tasks/" + group + "/add")
	if err != nil {
		log.Printf("unable to construct request url: %v", err)
		return subcommands.ExitFailure
	}

	resp, err := http.Post(u.String(), "application/json", bytes.NewReader(b))
	if err != nil {
		log.Printf("unable to issue POST request: %v", err)
		return subcommands.ExitFailure
	}
	var buf bytes.Buffer
	_, err = io.Copy(&buf, resp.Body)
	if err != nil {
		log.Printf("error while reading body: %v", err)
		return subcommands.ExitFailure
	}
	data := make(map[string]interface{})
	if err := json.Unmarshal(buf.Bytes(), &data); err != nil {
		log.Printf("unable to decode response: %v", err)
		return subcommands.ExitFailure
	}

	io.Copy(os.Stdout, &buf)
	if v := data["success"]; v == nil || v != true {
		return subcommands.ExitFailure
	}

	return subcommands.ExitSuccess
}

type workersAddCmd struct {
	lines bool
}

func (c *workersAddCmd) Name() string     { return "workers-add" }
func (c *workersAddCmd) Synopsis() string { return "add workers to a group" }
func (c *workersAddCmd) Usage() string {
	return `bern workers-add [options] [manifest...]

Creates workers in the group.
Worker manifests can be passed in as separate files or as separate lines within a file (see -lines).
If no manifest file is passed, stdin is read.
`
}

func (c *workersAddCmd) SetFlags(fs *flag.FlagSet) {
	fs.BoolVar(&c.lines, "lines", true, "each line within the input is a manifest")
}

func (c *workersAddCmd) Execute(ctx context.Context, fs *flag.FlagSet, args ...interface{}) subcommands.ExitStatus {
	refURL, err := url.Parse(addr)
	if err != nil {
		log.Printf("invalid addr: %v", err)
		return subcommands.ExitUsageError
	}

	var ws []Worker
	rcs := []io.ReadCloser{os.Stdin}
	if fs.NArg() > 0 {
		rcs = make([]io.ReadCloser, fs.NArg())
		for i, p := range fs.Args() {
			f, err := os.Open(p)
			if err != nil {
				log.Printf("unable to open %s: %v", p, err)
				return subcommands.ExitFailure
			}
			rcs[i] = f
		}
	}
	if c.lines {
		rs := make([]io.Reader, len(rcs))
		for i, r := range rcs {
			rs[i] = r
		}
		s := bufio.NewScanner(io.MultiReader(rs...))
		for s.Scan() {
			ws = append(ws, Worker{Manifest: s.Text()})
		}
		if s.Err() != nil {
			log.Printf("error while reading manifests: %v", err)
			return subcommands.ExitFailure
		}
	} else {
		for _, r := range rcs {
			b, err := ioutil.ReadAll(r)
			if err != nil {
				log.Printf("error while reading manifests: %v", err)
				return subcommands.ExitFailure
			}
			ws = append(ws, Worker{Manifest: string(b)})
		}
	}
	for _, c := range rcs {
		c.Close()
	}

	req := WorkersAddReq{Workers: ws}
	b, err := json.Marshal(&req)
	if err != nil {
		log.Printf("unable to encode request: %v", err)
		return subcommands.ExitFailure
	}

	u, err := refURL.Parse("workers/" + group + "/add")
	if err != nil {
		log.Printf("unable to construct request url: %v", err)
		return subcommands.ExitFailure
	}

	resp, err := http.Post(u.String(), "application/json", bytes.NewReader(b))
	if err != nil {
		log.Printf("unable to issue POST request: %v", err)
		return subcommands.ExitFailure
	}
	var buf bytes.Buffer
	_, err = io.Copy(&buf, resp.Body)
	if err != nil {
		log.Printf("error while reading body: %v", err)
		return subcommands.ExitFailure
	}
	data := make(map[string]interface{})
	if err := json.Unmarshal(buf.Bytes(), &data); err != nil {
		log.Printf("unable to decode response: %v", err)
		return subcommands.ExitFailure
	}

	io.Copy(os.Stdout, &buf)
	if v := data["success"]; v == nil || v != true {
		return subcommands.ExitFailure
	}

	return subcommands.ExitSuccess
}

func main() {
	rand.Seed(time.Now().UnixNano())
	flag.StringVar(&group, "group", "default", "group to operate in")
	flag.StringVar(&addr, "addr", "http://127.0.0.1:8080", "address of bernie server")
	log.SetPrefix("bern: ")
	log.SetFlags(0)
	subcommands.Register(subcommands.HelpCommand(), "")
	subcommands.Register(subcommands.FlagsCommand(), "")
	subcommands.Register(subcommands.CommandsCommand(), "")
	subcommands.Register(new(groupAddCmd), "")
	subcommands.Register(new(taskAddCmd), "")
	subcommands.Register(new(workersAddCmd), "")

	flag.Parse()
	os.Exit(int(subcommands.Execute(context.Background())))
}

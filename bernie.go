/*

THIS PACKAGE DOES NOT HAVE A STABLE API.

Package bernie provides functionality to manage task queues and workers.

*/
package bernie

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var rng = struct {
	mu  sync.Mutex
	gen *rand.Rand
}{
	gen: rand.New(rand.NewSource(time.Now().UnixNano())),
}

func randInt32() int32 {
	rng.mu.Lock()
	defer rng.mu.Unlock()
	return rng.gen.Int31()
}

type Logger interface {
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Errorf(format string, args ...interface{})
}

type responsiveSleeper struct {
	Max time.Duration
	Cur time.Duration
}

func (s *responsiveSleeper) Sleep() {
	if s.Cur > s.Max {
		s.Cur = s.Max
	}
	time.Sleep(s.Cur)
	s.Cur *= 2
}

type WorkerStatus struct {
	RunningTask *Task
	FailedTasks int
	InitTask    *Task
	Initialized bool
	Killed      bool
}

func (s WorkerStatus) IsFree() bool {
	return s.RunningTask == nil
}

func (s WorkerStatus) HumanFriendly(maxFails int) string {
	if s.Initialized {
		if s.FailedTasks > maxFails {
			return "Dead"
		}
		if s.RunningTask == nil {
			return "Ready"
		}
		return "Busy"
	}
	if s.InitTask == nil {
		return "Created"
	}
	return "Initializing"
}

func NewWorker(log Logger, name string, manifest string) *Worker {
	return &Worker{
		log:      log,
		name:     name,
		manifest: manifest,
	}
}

type Worker struct {
	log      Logger
	name     string
	manifest string

	mu     sync.Mutex
	status WorkerStatus

	initMu sync.Mutex
}

func (w *Worker) Kill(ctx context.Context) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.status.Killed = true
	t := w.status.RunningTask
	if t != nil {
		t.Kill(ctx, true)
	}
}

func (w *Worker) Status() WorkerStatus {
	w.mu.Lock()
	s := w.status
	w.mu.Unlock()
	return s
}

func (w *Worker) ResetFailures() {
	w.mu.Lock()
	w.status.FailedTasks = 0
	w.mu.Unlock()
}

func (w *Worker) Name() string     { return w.name }
func (w *Worker) Manifest() string { return w.manifest }

func (w *Worker) Reinit(t *Task) {
	w.initMu.Lock()
	defer w.initMu.Unlock()
	w.mu.Lock()
	w.status.FailedTasks = 0
	w.status.Initialized = false
	w.mu.Unlock()
	w.init(t)
}

func (w *Worker) Init(t *Task) {
	w.initMu.Lock()
	defer w.initMu.Unlock()
	if w.Status().Initialized {
		return
	}
	w.init(t)
}

func (w *Worker) init(t *Task) {
	w.mu.Lock()
	w.status.InitTask = t
	w.mu.Unlock()
	w.log.Debugf("init")
	w.Run(t)
	w.log.Debugf("ran init")
	if err := t.Status().Err; err != nil {
		w.log.Errorf("failed to initilize: %v", err)
	} else {
		w.mu.Lock()
		w.status.Initialized = true
		w.mu.Unlock()
	}
	w.log.Debugf("updated status")
}

var errWorkerKilled = errors.New("worker killed")
var errTaskKilled = errors.New("task killed")

func (w *Worker) Run(t *Task) {
	w.log.Debugf("setup run env")
	wdir, setupFine := w.setupRun(t)
	if !setupFine {
		return
	}
	w.log.Debugf("worker dir is: %s", wdir)

	session := "bernie-task+" + base62(randInt32())
	cmd := exec.Command("tmux",
		"new-session", "-d", "-s", session, filepath.Join(wdir, "do.sh"), ";",
		"set", "remain-on-exit", "on")
	cmd.Env = append(os.Environ(), []string{
		"WORKER_MANIFEST=" + filepath.Join(wdir, "wmanifest"),
	}...)
	if t.Env != nil {
		cmd.Env = append(cmd.Env, t.Env...)
	}
	cmd.Dir = t.WD

	w.log.Debugf("run command")
	err := cmd.Run()
	w.log.Debugf("command returned")
	st := t.Status()
	w.log.Debugf("set new status")
	st.Err = err
	st.Tmux.Session = session
	t.setStatus(st)
	w.log.Debugf("finished setting new status")

	var retErr error
	sleeper := responsiveSleeper{
		Max: 10 * time.Second,
		Cur: 125 * time.Millisecond,
	}
	for {
		if st := t.Status(); st.Killed || st.Err == errTaskKilled {
			retErr = errTaskKilled
			break
		}

		if _, err := os.Stat(filepath.Join(wdir, "done")); !os.IsNotExist(err) {
			if err == nil {
				b, err := ioutil.ReadFile(filepath.Join(wdir, "done"))
				if err != nil {
					retErr = fmt.Errorf("unable to read done file: %v", err)
				} else {
					code, err := strconv.Atoi(strings.TrimSpace(string(b)))
					if err != nil {
						retErr = fmt.Errorf("unable to parse error code from done file: %v", err)
					} else if code != 0 {
						retErr = fmt.Errorf("exit status %d", code)
					}
				}
				break
			}
			w.log.Errorf("error not nil or NotExist for done file: %s: %v", t.Name, err)
			break
		}

		w.mu.Lock()
		wkilled := w.status.Killed
		w.mu.Unlock()
		if wkilled {
			retErr = errWorkerKilled
			break
		}

		sleeper.Sleep()
	}
	st = t.Status()
	st.Done = true
	st.Err = retErr
	if st.Err == errWorkerKilled {
		st.Tries--
	}
	t.setStatus(st)

	if err := os.RemoveAll(wdir); err != nil {
		w.log.Infof("unable to remove wd %s: %v", wdir, err)
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	w.status.RunningTask = nil
	if err != nil && err != errTaskKilled {
		w.status.FailedTasks++
	}
}

func (w *Worker) setupRun(t *Task) (wdir string, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.status.RunningTask != nil {
		// already running something
		return "", false
	}
	if w.status.Killed {
		panic("trying to run on killed node")
		return "", false
	}

	st := t.Status()
	st.Done = false
	st.Runner = w
	st.Tries++
	defer t.setStatus(st)
	wdir, err := ioutil.TempDir("", "bernie-task")
	if err != nil {
		st.Err = err
		return "", false
	}
	err = ioutil.WriteFile(filepath.Join(wdir, "wmanifest"), []byte(w.Manifest()), 0666)
	if err != nil {
		st.Err = err
		return "", false
	}
	var buf bytes.Buffer
	buf.WriteString("#!/bin/sh\n")
	for _, p := range t.Cmd {
		buf.WriteString(strconv.Quote(p))
		buf.WriteByte(' ')
	}
	buf.WriteByte('\n')
	buf.WriteString("st=$?\necho exit status $st\necho $st > '")
	buf.WriteString(filepath.Join(wdir, "done"))
	buf.WriteString("'\n")
	err = ioutil.WriteFile(filepath.Join(wdir, "do.sh"), buf.Bytes(), 0777)
	if err != nil {
		st.Err = err
		return "", false
	}

	w.status.RunningTask = t
	return wdir, true
}

// circular, doubly linked list
type taskNode struct {
	t          *Task
	prev, next *taskNode
}

func listPushHead(list *taskNode, t *Task) {
	n := &taskNode{
		t:    t,
		prev: list,
		next: list.next,
	}
	list.next.prev = n
	list.next = n
}

func listPushTail(list *taskNode, t *Task) {
	n := &taskNode{
		t:    t,
		prev: list.prev,
		next: list,
	}
	list.prev.next = n
	list.prev = n
}

type WorkerPool struct {
	log               Logger
	maxTaskTries      int
	maxWorkerFailures int

	mu       sync.Mutex
	queued   taskNode
	initTask *Task
	pool     []*Worker
	free     []*Worker
}

func NewWorkerPool(log Logger, maxFailures, maxTries int, initTask *Task) *WorkerPool {
	p := &WorkerPool{
		log:               log,
		maxTaskTries:      maxTries,
		maxWorkerFailures: maxFailures,
		initTask:          initTask,
	}
	p.queued.next = &p.queued
	p.queued.prev = &p.queued
	return p
}

func (p *WorkerPool) AllowableTaskTries() int { return p.maxTaskTries }

func (p *WorkerPool) AllowableWorkerFailures() int {
	return p.maxWorkerFailures
}

func (p *WorkerPool) Remove(selector func([]*Worker) []int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	toremove := selector(p.pool)

	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	var wg sync.WaitGroup
	wg.Add(len(toremove))
	for _, i := range toremove {
		go func() {
			p.pool[i].Kill(ctx)
			wg.Done()
		}()
	}
	wg.Wait()

	sort.Sort(sort.Reverse(sort.IntSlice(toremove)))
	for _, i := range toremove {
		p.pool = append(p.pool[:i], p.pool[i+1:]...)
	}

	toremove = selector(p.free)
	sort.Sort(sort.Reverse(sort.IntSlice(toremove)))
	for _, i := range toremove {
		p.free = append(p.free[:i], p.free[i+1:]...)
	}

	p.schedule()
}

func (p *WorkerPool) WorkersCopy() []*Worker {
	p.mu.Lock()
	defer p.mu.Unlock()
	ws := make([]*Worker, len(p.pool))
	for i, w := range p.pool {
		ws[i] = w
	}
	return ws
}

func (p *WorkerPool) Grow(ws []*Worker) {
	p.mu.Lock()
	defer p.mu.Unlock()
	defer p.schedule()

	p.pool = append(p.pool, ws...)
	for _, w := range ws {
		go func(w *Worker, t *Task, maxFail int) {
			for i := 0; i <= maxFail; i++ {
				w.Init(t)
				if w.Status().Initialized {
					p.mu.Lock()
					defer p.mu.Unlock()
					defer p.schedule()
					addFree(&p.free, []*Worker{w}, p.maxWorkerFailures)
					return
				}
			}
		}(w, p.initTask.FreshCopy(), p.maxWorkerFailures)
	}
}

func addFree(dst *[]*Worker, ws []*Worker, maxFailures int) {
	for _, w := range ws {
		wst := w.Status()
		if wst.IsFree() && wst.FailedTasks < maxFailures && !wst.Killed {
			*dst = append(*dst, ws...)
		}
	}
}

func (p *WorkerPool) Submit(ts ...*Task) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	defer p.schedule()
	for _, t := range ts {
		p.submit(t)
	}
	return nil
}

func (p *WorkerPool) submit(t *Task) error {
	listPushTail(&p.queued, t)
	return nil
}

// schedule schedules currently queued tasks on workers.
//
// Make sure that p.mu is held before calling this method!
func (p *WorkerPool) schedule() {
	p.log.Infof("scheduler: has tasks: %t nworkers: %d", p.queued.next != &p.queued, len(p.free))
	for p.queued.next != &p.queued && len(p.free) != 0 {
		t := p.queued.next.t
		p.queued.next = p.queued.next.next
		p.queued.prev = &p.queued
		if t.Status().IsRunning() || t.Status().Killed {
			continue
		}
		w := p.free[len(p.free)-1]
		p.free = p.free[:len(p.free)-1]
		go func(w *Worker, t *Task) {
			w.Run(t)
			p.mu.Lock()
			addFree(&p.free, []*Worker{w}, p.maxWorkerFailures)
			p.schedule()
			p.mu.Unlock()
			st := t.Status()
			if st.Err != nil && st.Tries < p.maxTaskTries && !st.Killed {
				if st.Err != errWorkerKilled {
					t.Kill(context.Background(), false)
				}
				p.Submit(t)
			}
		}(w, t)
	}
}

type Task struct {
	Name string   `json:"name"`
	Cmd  []string `json:"cmd"`
	Env  []string `json:"env"`
	WD   string   `json:"wd"`

	mu     sync.Mutex
	status TaskStatus
}

func (t *Task) FreshCopy() *Task {
	return &Task{
		Name: t.Name,
		Cmd:  t.Cmd,
		Env:  t.Env,
		WD:   t.WD,
	}
}

func (t *Task) Status() TaskStatus {
	t.mu.Lock()
	s := t.status
	t.mu.Unlock()
	return s
}

func (t *Task) setStatus(s TaskStatus) {
	t.mu.Lock()
	t.status = s
	t.mu.Unlock()
}

func (t *Task) ResetTries() {
	t.mu.Lock()
	t.status.Tries = 0
	t.mu.Unlock()
}

func (t *Task) Kill(ctx context.Context, workerKilled bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	session := t.status.Tmux.Session
	if session != "" {
		exec.CommandContext(ctx, "tmux", "kill-session", "-t", session).Run()
	}
	if workerKilled {
		t.status.Err = errWorkerKilled
		t.status.Killed = false
	} else {
		t.status.Err = errTaskKilled
		t.status.Killed = true
	}
}

type TaskStatus struct {
	Done   bool
	Killed bool
	Err    error
	Tries  int
	Runner *Worker

	Tmux struct {
		Session string
	}
}

func (s TaskStatus) IsNew() bool {
	return s.Err == nil && s.Tries == 0
}

func (s TaskStatus) GetOutput() (string, error) {
	if s.Tmux.Session == "" {
		return "", nil
	}
	pt := s.Tmux.Session + ":0.0"
	b, err := exec.Command("tmux", "capture-pane", "-pt", pt, "-S", "-10000").CombinedOutput()
	return string(b), err
}

func (s TaskStatus) IsRunning() bool {
	return s.Err == nil && !s.Killed && !s.Done && s.Runner != nil
}

func (s TaskStatus) HumanFriendly(maxTries int) string {
	if s.Done {
		if s.Err == nil {
			return "Ran on " + s.Runner.Name()
		} else {
			return fmt.Sprintf("Got err: %v, %d fails", s.Err, s.Tries)
		}
	}
	if s.Runner != nil {
		return "Running on " + s.Runner.Name()
	}
	if s.Tries > maxTries {
		return "Too many failed tries"
	}
	if s.Killed {
		return "Killed"
	}
	return fmt.Sprintf("Queued, %d fails", s.Tries)
}

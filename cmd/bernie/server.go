package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/uluyol/bernie"
)

type Group struct {
	Name     string
	Pool     *bernie.WorkerPool
	Tasks    []*bernie.Task
	TasksSet map[string]struct{}
}

func (s *bernieServer) newGroup(name string, maxFails, maxTries int, initTask *bernie.Task) *Group {
	pl := s.log.WithFields(logrus.Fields{
		"group": name,
		"elem":  "wpool",
	})
	return &Group{
		Name:     name,
		Pool:     bernie.NewWorkerPool(pl, maxFails, maxTries, initTask),
		TasksSet: make(map[string]struct{}),
	}
}

func (g *Group) HasTask(name string) bool {
	_, ok := g.TasksSet[name]
	return ok
}

type bernieServer struct {
	log logrus.FieldLogger

	mu      sync.RWMutex
	groups  map[string]*Group
	nameGen batchNameGen
}

func (s *bernieServer) init() {
	s.groups = make(map[string]*Group)
}

var errGroupNotExist = errors.New("group does not exist")

func (s *bernieServer) addGroup(group string, init *bernie.Task) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.groups[group]; ok {
		return false
	}
	s.groups[group] = s.newGroup(group, *maxFails, *maxTries, init)
	return true
}

func (s *bernieServer) addWorkers(group string, manifests []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	batch := s.nameGen.next()
	ws := make([]*bernie.Worker, len(manifests))
	for i, m := range manifests {
		wname := fmt.Sprintf("%s-%03d", batch, i)
		ws[i] = bernie.NewWorker(s.log.WithField("worker", wname), wname, m)
	}
	g, ok := s.groups[group]
	if !ok {
		return errGroupNotExist
	}
	g.Pool.Grow(ws)
	return nil
}

func (s *bernieServer) rmWorker(group string, name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g, ok := s.groups[group]
	if !ok {
		return false
	}
	g.Pool.Remove(func(ws []*bernie.Worker) []int {
		var idx []int
		for i, w := range ws {
			if w.Name() == name {
				idx = append(idx, i)
			}
		}
		return idx
	})
	return true
}

func (s *bernieServer) rmTask(group string, name string) error {
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.groups[group]
	if !ok {
		return errGroupNotExist
	}
	for i, t := range g.Tasks {
		if t.Name == name {
			g.Tasks = append(g.Tasks[:i], g.Tasks[i+1:]...)
			t.Kill(ctx, false)
		}
		delete(g.TasksSet, name)
	}
	return ctx.Err()
}

func (s *bernieServer) addTasks(group string, tasks []*bernie.Task) (succ, fail []*bernie.Task, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.groups[group]
	if !ok {
		return nil, nil, errGroupNotExist
	}
	added, notAdded := g.addNewTasks(tasks)
	g.Pool.Submit(added...)
	s.log.WithFields(logrus.Fields{
		"group": group,
		"succ":  len(added),
		"fail":  len(notAdded),
	}).Info("added tasks")
	return added, notAdded, nil
}

func (g *Group) addNewTasks(toAdd []*bernie.Task) (succ, fail []*bernie.Task) {
	for _, t := range toAdd {
		if g.HasTask(t.Name) {
			fail = append(fail, t)
		} else {
			succ = append(succ, t)
			g.Tasks = append(g.Tasks, t)
			g.TasksSet[t.Name] = struct{}{}
		}
	}
	return succ, fail
}

func (s *bernieServer) Workers(group string) []*bernie.Worker {
	s.mu.RLock()
	g := s.groups[group]
	s.mu.RUnlock()
	if g == nil {
		return nil
	}
	return g.Pool.WorkersCopy()
}

func (s *bernieServer) Tasks(group string) []*bernie.Task {
	s.mu.RLock()
	defer s.mu.RUnlock()

	g := s.groups[group]
	if g == nil {
		return nil
	}
	return append([]*bernie.Task(nil), g.Tasks...)
}

func (s *bernieServer) Groups() []Group {
	s.mu.RLock()
	gs := make([]Group, 0, len(s.groups))
	for _, g := range s.groups {
		gs = append(gs, *g)
	}
	s.mu.RUnlock()
	sort.Slice(gs, func(i, j int) bool {
		return gs[i].Name < gs[j].Name
	})
	return gs
}

type batchNameGen struct {
	nbatch int
}

func (g *batchNameGen) next() string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	n := g.nbatch
	res := make([]byte, 0, 4)
	for n >= 26 {
		q := n / 26
		r := n % 26
		n = q
		res = append(res, letters[r])
	}
	res = append(res, letters[n])
	g.nbatch++
	return string(res)
}

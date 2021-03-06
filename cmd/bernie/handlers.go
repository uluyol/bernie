package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"github.com/uluyol/bernie"
)

type handler struct {
	log    logrus.FieldLogger
	bernie bernieServer
}

func (s *handler) rootHandler(w http.ResponseWriter, r *http.Request) {
	rootTempl.Execute(w, struct {
		Groups []Group
	}{
		s.bernie.Groups(),
	})
}

type groupsAddReq struct {
	Name string       `json:"name"`
	Init *bernie.Task `json:"init"`
}

// Possible paths:
// /groups/add
func (s *handler) groupsAddHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("content-type", "application/json")
	dec := json.NewDecoder(r.Body)
	var reqData groupsAddReq
	if err := dec.Decode(&reqData); err != nil {
		s.log.WithFields(logrus.Fields{
			"err":  err,
			"path": r.URL.Path,
		}).Error("unable to decode")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, `{"success": false, "reason": "error decoding request"}`)
		return
	}
	if reqData.Init == nil {
		s.log.WithField("path", r.URL.Path).Info("missing init task")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, `{"success": false, "reason": "need init task"}`)
		return
	}
	if !s.bernie.addGroup(reqData.Name, reqData.Init) {
		w.WriteHeader(http.StatusConflict)
		fmt.Fprintln(w, `{"success": false, "reason": "cannot create existing group"}`)
		return
	}
	fmt.Fprintln(w, `{"success": true}`)
}

type tasksAddReq struct {
	Tasks []*bernie.Task `json:"tasks"`
}

func (s *handler) tasksAddHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("content-type", "application/json")
	vars := mux.Vars(r)
	group := vars["group"]
	var reqData tasksAddReq
	if !s.decodeBodyInto(w, r, &reqData) {
		return
	}

	succ, fail, err := s.bernie.addTasks(group, reqData.Tasks)
	resp := struct {
		Success bool     `json:"success"`
		Reason  string   `json:"reason"`
		Added   []string `json:"added"`
		Exist   []string `json:"exist"`
	}{
		Success: len(fail) == 0 && err == nil,
		Added:   make([]string, len(succ)),
		Exist:   make([]string, len(fail)),
	}
	if err == errGroupNotExist {
		resp.Reason = err.Error()
	}
	for i, t := range succ {
		resp.Added[i] = t.Name
	}
	for i, t := range fail {
		resp.Exist[i] = t.Name
	}

	b, err := json.Marshal(resp)
	if err != nil {
		s.log.WithField("err", err).Error("unable to marshal response")
		w.WriteHeader(http.StatusInternalServerError)
		b = []byte(`{"success": false, "reason": "unable to marshal response"}`)
	}
	fmt.Fprintln(w, string(b))
}

func (s *handler) tasksDeleteHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("content-type", "application/json")
	vars := mux.Vars(r)
	group := vars["group"]
	task := vars["task"]
	err := s.bernie.rmTask(group, task)
	switch err {
	case nil:
		fmt.Fprintln(w, `{"success": true}`)
	case errGroupNotExist:
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "{\"success\": false, \"reason\": %q}\n", err.Error())
	default:
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, `{"success": false}`)
	}
}

func getTask(ts []*bernie.Task, name string) (*bernie.Task, bool) {
	for _, t := range ts {
		if t.Name == name {
			return t, true
		}
	}
	return nil, false
}

func getWorker(ws []*bernie.Worker, name string) (*bernie.Worker, bool) {
	for _, w := range ws {
		if w.Name() == name {
			return w, true
		}
	}
	return nil, false
}

func (s *handler) tasksPatchHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("content-type", "application/json")
	vars := mux.Vars(r)
	group := vars["group"]
	task := vars["task"]
	if r.URL.RawQuery == "status-tries=0" {
		if t, ok := getTask(s.bernie.Tasks(group), task); ok {
			t.ResetTries()
			fmt.Fprintln(w, `{"success": true}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintln(w, `{"success": false, "reason": "unknown group or task"}`)
		return
	}
	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprintln(w, `{"success": false, "reason": "unknown or unprovided field"}`)
}

func (s *handler) tasksManifestHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("content-type", "text/plain")
	vars := mux.Vars(r)
	group := vars["group"]
	task := vars["task"]
	if t, ok := getTask(s.bernie.Tasks(group), task); ok {
		manifest := struct {
			Name   string   `json:"name"`
			Cmd    []string `json:"cmd"`
			Env    []string `json:"env"`
			WD     string   `json:"wd"`
			Status struct {
				Tmux struct {
					Session string `json:"session"`
				} `json:"tmux"`
			} `json:"status"`
		}{
			Name: t.Name,
			Cmd:  t.Cmd,
			Env:  t.Env,
			WD:   t.WD,
		}
		manifest.Status.Tmux.Session = t.Status().Tmux.Session
		b, err := json.Marshal(&manifest)
		if err != nil {
			s.log.WithFields(logrus.Fields{
				"err":  err,
				"path": r.URL.Path,
			}).Error("unable to encode task manifest")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, "unable to encode task manifest")
			return
		}
		var buf bytes.Buffer
		if err := json.Indent(&buf, b, "", "  "); err != nil {
			s.log.WithFields(logrus.Fields{
				"err":  err,
				"path": r.URL.Path,
			}).Error("unable to indent json")
			buf.Reset()
			buf.Write(b)
		}
		buf.WriteByte('\n')
		w.Header().Add("content-type", "application/json")
		buf.WriteTo(w)
		return
	}
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprintln(w, "unknown group or task")
}

func (s *handler) tasksOutHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("content-type", "text/plain")
	vars := mux.Vars(r)
	group := vars["group"]
	task := vars["task"]
	if t, ok := getTask(s.bernie.Tasks(group), task); ok {
		out, err := t.Status().GetOutput()
		if err == nil {
			fmt.Fprintln(w, out)
			return
		}
		s.log.WithFields(logrus.Fields{
			"err":  err,
			"path": r.URL.Path,
		}).Error("unable to get output")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, "unable to get output")
		return
	}
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprintln(w, "unknown group or task")
}

type workersAddReq struct {
	Workers []struct {
		Manifest string `json:"manifest"`
	} `json:"workers"`
}

func (s *handler) workersAddHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("content-type", "application/json")
	vars := mux.Vars(r)
	group := vars["group"]
	var reqData workersAddReq
	if !s.decodeBodyInto(w, r, &reqData) {
		return
	}

	manifests := make([]string, len(reqData.Workers))
	for i := range reqData.Workers {
		manifests[i] = reqData.Workers[i].Manifest
	}
	err := s.bernie.addWorkers(group, manifests)
	if err == nil {
		fmt.Fprintln(w, `{"success": true}`)
		return
	}
	if err != errGroupNotExist {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, `{"success": false}`)
	} else {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "{\"success\": false, \"reason\": %q}\n", err.Error())
	}
}

func (s *handler) workersDeleteHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("content-type", "application/json")
	vars := mux.Vars(r)
	group := vars["group"]
	worker := vars["worker"]
	if !s.bernie.rmWorker(group, worker) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintln(w, `{"success": false, "reason": "unknown group"}`)
		return
	}
	fmt.Fprintln(w, `{"success": true}`)
}

func (s *handler) workersPatchHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("content-type", "application/json")
	vars := mux.Vars(r)
	group := vars["group"]
	worker := vars["worker"]
	if r.URL.RawQuery == "status-failedtasks=0" {
		if worker, ok := getWorker(s.bernie.Workers(group), worker); ok {
			worker.ResetFailures()
			fmt.Fprintln(w, `{"success": true}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintln(w, `{"success": false, "reason": "unknown group or worker"}`)
		return
	}
	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprintln(w, `{"success": false, "reason": "unknown or unprovided field"}`)
}

func (s *handler) workersManifestHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("content-type", "text/plain")
	vars := mux.Vars(r)
	group := vars["group"]
	worker := vars["worker"]
	if worker, ok := getWorker(s.bernie.Workers(group), worker); ok {
		fmt.Fprintln(w, worker.Manifest())
		return
	}
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprintln(w, "unknown group or worker")
	return
}

func (s *handler) workersInitOutHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("content-type", "text/plain")
	vars := mux.Vars(r)
	group := vars["group"]
	worker := vars["worker"]
	if worker, ok := getWorker(s.bernie.Workers(group), worker); ok {
		t := worker.Status().InitTask
		if t == nil {
			fmt.Fprintln(w, "init task not yet created")
			return
		}
		out, err := t.Status().GetOutput()
		if err == nil {
			fmt.Fprintln(w, out)
			return
		}
		s.log.WithFields(logrus.Fields{
			"err":  err,
			"path": r.URL.Path,
		}).Error("unable to get output")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, "unable to get output")
		return
	}
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprintln(w, "unknown group or worker")
	return
}

func (s *handler) decodeBodyInto(w http.ResponseWriter, r *http.Request, out interface{}) (ok bool) {
	if err := json.NewDecoder(r.Body).Decode(out); err != nil {
		s.log.WithFields(logrus.Fields{
			"err":  err,
			"path": r.URL.Path,
		}).Error("unable to decode")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, `{"success": false, "reason": "error decoding request"}`)
		return false
	}
	return true
}

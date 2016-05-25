package main

import (
	"flag"
	"html/template"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const rootTemplateStr = `
<!doctype html>
<html>
	<title>Bernie Mux</title>
	<style>
		body {
			margin: 10px 40px;
			font-size: 1.2em;
		}
	</style>
</head>
<body>
<pre>
<b>Bernies</b>
{{- range .Contexts}}
  <a href="/bernie/{{.Name | ToLower}}/">{{.Name}}</a> | {{.Start | FmtTime}}
{{end -}}
</pre>
</body>
</html>
`

var rootTemplateFuncs = template.FuncMap{
	"ToLower": strings.ToLower,
	"FmtTime": func(t time.Time) string { return t.Format(time.RFC3339) },
}

var rootTempl = template.Must(template.New("root").Funcs(rootTemplateFuncs).Parse(rootTemplateStr))

var port = flag.Int("p", 8080, "port to listen on")

var bernies struct {
	mu       sync.RWMutex
	Contexts map[string]Proxy
}

type Proxy struct {
	Name   string
	Start  time.Time
	RProxy http.Handler
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		split := strings.Split(r.URL.Path, "/")
		if len(split) < 3 {
			http.Error(w, "invalid url, must have context", http.StatusBadRequest)
			return
		}
		context := split[2]
		bernies.mu.RLock()
		defer bernies.mu.RUnlock()
		proxy, ok := bernies.Contexts[context]
		if !ok {
			http.Error(w, "unable to find context", http.StatusNotFound)
		}
		proxy.RProxy.ServeHTTP(w, r)
		return
	}
	bernies.mu.RLock()
	defer bernies.mu.RUnlock()
	rootTempl.Execute(w, &bernies)
}

func registerHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "error decoding form input", http.StatusInternalServerError)
		return
	}
	context := r.Form.Get("context")
	port := r.Form.Get("port")
	bernies.mu.Lock()
	defer bernies.mu.Unlock()
	if _, ok := bernies.Contexts[context]; ok {
		http.Error(w, "context already exists", http.StatusConflict)
		return
	}
	bernies.Contexts[context] = Proxy{
		Name:  context,
		Start: time.Now(),
		RProxy: httputil.NewSingleHostReverseProxy(&url.URL{
			Scheme: "http",
			Host:   "127.0.0.1:" + port,
		}),
	}
	w.Write([]byte("success"))
}

func main() {
	bernies.Contexts = make(map[string]Proxy)
	http.HandleFunc("/", rootHandler)
	http.HandleFunc("/register", registerHandler)
	http.ListenAndServe(":"+strconv.Itoa(*port), nil)
}

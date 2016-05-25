package main

import (
	"html/template"
	"strings"
	"time"
)

const rootTemplateStr = `
<!doctype html>
<html>
<head>
	<title>Bernie</title>
	<style>
		body {
			margin: 10px 40px;
			font-size: 1.2em;
		}
	</style>
</head>
<body>
<pre>
<a href="/">← back</a>

<b>Procs · Bernie {{.Context}}</b>
{{- range .Procs}}
  <a href="{{.ProcPath}}">{{.Name}}</a>  {{if .IsRunning}}<a href="{{.KillPath}}">[kill]</a> [run]{{else}}[kill]  <a href="{{.RunPath}}">[run]</a>{{end}}
{{end -}}
</pre>
</body>
</html>
`

var rootTemplFuncs = template.FuncMap{
	"ToLower":    strings.ToLower,
	"FormatTime": func(t time.Time) string { return t.Format(time.RFC3339) },
}

var rootTempl = template.Must(template.New("root").Funcs(rootTemplFuncs).Parse(rootTemplateStr))

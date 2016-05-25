package main

import (
	"html/template"
	"strings"
)

const viewTemplateStr = `
<!doctype html>
<html>
<head>
	<title>Berni</title>
	<style>
		body {
			margin: 10px 40px;
			font-size: 1.2em;
		}
	</style>
</head>
<body>
<pre>
<a href="/bernie/{{.Context}}/">← back</a>

<b>{{.Name}}</b>  {{if .IsRunning}}<a href="{{.KillPath}}">[kill]</a> [run]{{else}}[kill]  <a href="{{.RunPath}}">[run]</a>{{end}}
<iframe src="{{.OutPath}}" style="width: 900px; height: 600px;">
</pre>
</body>
`

var viewTemplFuncs = template.FuncMap{
	"ToLower": strings.ToLower,
}

var viewTempl = template.Must(template.New("view").Funcs(viewTemplFuncs).Parse(viewTemplateStr))

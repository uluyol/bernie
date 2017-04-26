package main

import "html/template"

const rootTemplStr = `<!doctype html>
<html>
<head>
	<meta charset="utf-8" /> 
	<title>Bernie</title>
	<style>
		body {
			margin: 10px 40px;
			font-size: 1.2em;
		}

		a {
			color: blue;
		}
	</style>
</head>
<body>
<pre>
<a href="/">Bernie</a>

<b>Groups</b>
{{- range .Groups}}
  {{- $gname := .Name -}}
  {{- $maxFails := .Pool.AllowableWorkerFailures -}}
  {{- $maxTries := .Pool.AllowableTaskTries}}
  {{$gname}}
    Tasks
    {{- range .Tasks}}
      {{- $pathPre := printf "/tasks/%s/%s" $gname .Name}}
      <a href="{{$pathPre}}/out"><b>{{.Name}}</b></a> <a href="#" onclick="apiPatch('{{$pathPre}}?status-tries=0')">reset tries</a> [{{.Status.HumanFriendly $maxTries}}] <a href="#" onclick="apiDelete('{{$pathPre}}')">rm</a>
    {{- end}}
    Workers
    {{- range .Pool.WorkersCopy}}
      {{- $pathPre := printf "/workers/%s/%s" $gname .Name}}
      <a href="{{$pathPre}}/manifest"><b>{{.Name}}</b></a> <a href="{{$pathPre}}/initout">init out</a> <a href="#" onclick="apiPatch('{{$pathPre}}?status-failedtasks=0')">reset fails</a> [{{.Status.FailedTasks}} fails, {{.Status.HumanFriendly $maxFails}}] <a href="#" onclick="apiDelete('{{$pathPre}}')">rm</a>
    {{- end}}
{{end}}
</pre>
<script>
function apiDelete(url) {
	var xhr = new XMLHttpRequest();
	xhr.open('DELETE', url, true);
	xhr.onload = function() {
		location.reload();
	}
	xhr.send(null);
}
function apiPatch(urlWithQS) {
	var xhr = new XMLHttpRequest();
	xhr.open('PATCH', urlWithQS, true);
	xhr.onload = function() {
		location.reload();
	}
	xhr.send(null);
}
</script>
</body>
</head>
`

var rootTempl = template.Must(template.New("root").Parse(rootTemplStr))

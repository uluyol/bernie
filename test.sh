#!/usr/bin/env bash

set -e

echo '{"name": "tgroup", "init": {"name": "init", "cmd": ["echo", "initilialized"]}}' | curl -d @- -X POST http://localhost:8080/groups/add
echo '{"tasks": [{"name": "sleep-cmd-1", "cmd": ["sleep", "5"]}]}' | curl -d @- -X POST http://localhost:8080/tasks/tgroup/add
echo '{"tasks": [{"name": "sleep-cmd-2", "cmd": ["sleep", "19"]}]}' | curl -d @- -X POST http://localhost:8080/tasks/tgroup/add
echo '{"tasks": [{"name": "sleep-cmd-3", "cmd": ["sleep", "19"]}, {"name": "echo-cmd-1", "cmd": ["echo", "19"]}]}' | curl -d @- -X POST http://localhost:8080/tasks/tgroup/add
echo '{"workers": [{"manifest": "dummy"}, {"manifest": "dummy2"}]}' | curl -d @- -X POST http://localhost:8080/workers/tgroup/add
echo '{"workers": [{"manifest": "dummy"}, {"manifest": "dummy"}]}' | curl -d @- -X POST http://localhost:8080/workers/tgroup/add

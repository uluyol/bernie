## What is bernie?

bernie is a tool to centrally manage your processes.
bernie run tasks in tmux sessions, schedules their execution on workers,
and retries failed tasks.

bernie is **not** a replacement for init, systemd, etc.

## Server

cmd/bernie is the server that manages everything.
bernie has 3 main concepts: groups, workers, and tasks.

### Groups

Groups consist of a worker pool and set of tasks.
During creation, an init task can be optionally passed.
The init task will be run on new workers before they can proccess general tasks.

### Workers

A worker is described by an opaque *manifest*.
Bernie does not interpret the contents of the manifest at all.
When a task is created, it is passed a WORKER_MANIFEST env variable with a path
to a file containing the contents of that manifest.
It is up to the user to interpret this however appropriate (ssh keys, sets of nodes, etc.)

### Tasks

A task is a command (with a working directory and environment), to be run under bernie.
Before the process is created, a tmux session is created where the process will be run.
The session will persist beyond the lifetime of the process.

## Client

cmd/bern is the client used to create groups, tasks, and workers.

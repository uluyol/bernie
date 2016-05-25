## What is bernie?

bernie is a tool to centrally manage your processes. You can
start and stop them as well as view their output from a web
browser and connect directly to a tmux session if you want.

bernie is **not** a replacement for init, systemd, etc.

## How does it work

There are three binaries: bern, berniemux, bernied

bernied (the bernie daemon) manages processes. It creates tmux
sessions that can be connected to and also presents the output
from a web ui.
berniemux registers bernie daemons and forwards requests to them.
bern asks the bernie daemon to run a process and tries to capture
it's environment and replicate that in the created process.

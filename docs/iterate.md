There is a live-fire test repostory at ~/repo/governator-TEST/. It contains a trivial sample GOVERNATOR.md file.

You have full authority to modify and operate in governator-TEST/, but you must never delete or modify the GOVERNATOR.md file.

I have set `alias gov='~/repo/governator/governator'` in my shell.
I have set `alias got='go test ./...'` in my shell.
I have set `alias gob='go build -o governator .'` in my shell.

To reset this testbed, I have been running...
```
cd ~/repo/governator-TEST/ && rm -rf _governator .git && git init . && git add . && git commit -m 'Initial commit' && gov -v init && gov -v plan && gov -v status
```
... to kick off a fresh new governator test from scratch.

I then monitor that project with `gov -v status` and watch the working directories. Please confirm you can do all of
this.

After you execute `gov plan`, get the pid of the worker and watch it - this saves polling loops and tokens.

You may inspect, modify, and execute commands within the testbed as needed (again, just preserve the GOVERNATOR.md). Any Governator code changes as a result, of course need to happen here (in ~/repo/governator), then you can run tests, rebuild, and reset the testbed.

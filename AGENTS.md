# Rules for operating within this project
For full context, read `./README.md`.

Never run `governator init` in this repository. This repo must not contain a
generated `./_governator/` workspace. If initialization is needed for
validation, use a temporary directory.

No tests are required when planning or updating documentation.

## Local manual testing
There is a live-fire test repostory at `~/repo/governator-testbed/`. It contains
a trivial `GOVERNATOR.md` file copied from `docs/GOVERNATOR.md.sample.toyls`.

You have full authority to inspect, modify, and operate within the testbed, but
must NEVER perform a full restart-from-scratch without explicit operator
approval. The way to perform a full reset is:
```
TB="~/repo/governator-testbed" \
  rm -rf $TB && \
  mkdir -p $TB && \
  cp ./docs/GOVERNATOR.md.sample.toyls $TB/GOVERNATOR.md && \
  cd $TB && \
  git init . && \
  git add GOVERNATOR.md && \
  git commit -m 'Initial commit' && \
  gov --verbose init && \
  gov --verbose start
```

_Note: I have `alias gov='~/repo/governator/governator'` in my shell so I'm
always using the latest local build._


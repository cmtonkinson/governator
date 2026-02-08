After making code, template, or config changes, use `./test.sh -a` to invoke the tests.
No tests are required when planning or updating documentation.

Never run `governator init` in this repository. This repo must not contain a generated `_governator/` workspace.
If initialization is needed for validation, use a temporary directory or dedicated testbed repository.

# Single Task Flow Fixture

This fixture contains the smallest Governator repository state: a single open worker task with no dependencies or overlaps.

It is intended for tests that need a known-good starting point, such as status reporting or scheduler sanity checks. The accompanying markdown in `_governator/tasks` documents the behaviour this task represents.

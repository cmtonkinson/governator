# Dependency Chain with Overlap Fixture

This fixture models a small dependency chain plus an overlap conflict. Task T-202 depends on the done task T-201, T-203 depends on T-202, and both T-204 and T-205 race over the `shared-resource` overlap marker.

It is intended for tests that need to exercise dependency gating and scheduler overlap detection together.

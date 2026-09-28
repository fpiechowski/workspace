# Summary

Decoupled tests from the built-in legacy workflow by adding a test-only `extended` workflow fixture and configuring the shared fixture with the complete extended capability set. Updated all internal test references and kept the extended state-machine coverage for change requests, live testing, and release.

Verification: formatting and vet passed; core and all non-core/non-CLI package suites passed. The full `go test ./...` command exceeded the environment timeout, while CLI tests pass when the worker actor identity variables are unset.

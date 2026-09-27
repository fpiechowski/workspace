# Summary

Aligned documentation and help with the shipped `workspace project init` behavior.
README now documents the interactive wizard (activation, questions, cancellation and
final confirmation) and the non-interactive/script mode, and its example config no
longer presents OpenCode `--auto` as a default. The `project init` help text,
PRODUCT.md, and ARCHITECTURE.md describe the current init/configuration contract.
Verification ran gofmt, `go test ./...`, `go vet ./...`, a fresh build, and
`scripts/check-install.py`, all captured as check receipts.

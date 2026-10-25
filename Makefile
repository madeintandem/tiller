.PHONY: build test lint fmt ui

# Build all packages and the tiller binary.
build:
	go build ./...

# Run the full test suite, never from the cache.
#
# -count=1 is the cache defeat, and it is here for correctness rather than
# taste. internal/e2e builds the tiller binary at runtime and drives it as a
# subprocess, so its real input is the whole module; but the Go package imports
# only internal/store, so Go's test cache sees no change when internal/cli or
# internal/core/mutate move and serves a stale `ok`. That is not a stale result
# a reader can spot: it is a green line for a run that never happened, and it
# hid a red spine for a whole task. The blunt flag is chosen over excluding the
# one package by name because it is the harness being incapable of the lie
# rather than someone remembering to add the next such package to a list. The
# cost is the full suite every invocation; `go test ./...` is still there for a
# fast inner loop, on the understanding that its green means less.
# GIT_CONFIG_*: the fixtures make hundreds of small git repos and commits, and a
# newer git answers a commit by spawning `maintenance run --auto` in the
# background; its transient .git/objects/maintenance.lock races the tests that
# snapshot a tree to prove a refusal wrote nothing. Tell every git the tests
# spawn not to.
# GIT_BIN_DIR: on macOS /usr/bin/git is an xcrun shim that costs an extra spawn
# on every git call, and the suite makes thousands. Put the real git (from
# `xcrun --find git`) first on PATH for the test recipe only. Where that lookup
# fails (no Xcode git) or on Linux, the variable is empty and PATH is untouched.
# The prefix is Xcode's whole developer bin, so the test recipe also sees its
# python3, make, and compilers; the tests spawn only git, sh, cp, touch, and go.
# Recursively expanded (=) so only the test recipe runs xcrun, not build or lint.
GIT_BIN_DIR = $(if $(filter Darwin,$(shell uname -s)),$(patsubst %/,%,$(dir $(shell xcrun --find git 2>/dev/null))))
test:
	$(if $(GIT_BIN_DIR),PATH="$(GIT_BIN_DIR):$$PATH" )GIT_CONFIG_COUNT=2 GIT_CONFIG_KEY_0=gc.auto GIT_CONFIG_VALUE_0=0 \
	GIT_CONFIG_KEY_1=maintenance.auto GIT_CONFIG_VALUE_1=false \
	go test -count=1 ./...

# Static checks: vet plus a gofmt cleanliness gate (fails on any unformatted file).
# gofmt comes from the toolchain go.mod selects (go env GOROOT), not from PATH,
# so a machine with an older gofmt on PATH cannot pass lint that CI then fails.
lint:
	go vet ./...
	@unformatted=$$("$$(go env GOROOT)/bin/gofmt" -l .) || exit $$?; \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needs to run on:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

# Rewrite source with gofmt.
fmt:
	"$$(go env GOROOT)/bin/gofmt" -w .

# Build the web console into ui/dist (committed, embedded by the serve command).
ui:
	cd ui && npm install && npm run build

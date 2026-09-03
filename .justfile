default:
  just --list

check: lint test
  just banner "✓ check ✓"

clean:
  go clean -testcache

fmt:
  go mod tidy
  golangci-lint fmt

lint:
  golangci-lint run

#
# bin
#

build:
  mkdir -p tmp/bin
  go build -o tmp/bin/httpdisk-go ./cmd/httpdisk-go

install: build
  cp tmp/bin/httpdisk-go ~/.local/bin/httpdisk-go

#
# test
#

test *ARGS:
  go test ./... {{ARGS}}
  just banner "✓ test ✓"

test-watch *ARGS:
  watchexec -q --clear=reset just test {{ARGS}}

#
# banner and friends
#

set quiet

[private]
banner +ARGS:  (_banner '\e[48;2;064;160;043m' ARGS)
warning +ARGS: (_banner '\e[48;2;251;100;011m' ARGS)
fatal +ARGS:   (_banner '\e[48;2;210;015;057m' ARGS)
  exit 1
_banner BG +ARGS:
  printf '\e[38;5;231m{{BOLD+BG}}[%s] %-72s {{NORMAL}}\n' "$(date +%H:%M:%S)" "{{ARGS}}"

#!/usr/bin/env bash

set -euo pipefail

go build
./multi-test \
	-pkg 'github.com/ericlagergren/decimal' \
	-cmd 'go test -short -v . ./internal/...' \
	-file stdout

# -cmd 'pwd; whoami; ls -l' \

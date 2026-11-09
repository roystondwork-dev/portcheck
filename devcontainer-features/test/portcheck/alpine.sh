#!/bin/bash
# Run by `devcontainer features test` inside a container with the feature installed.
set -e
source dev-container-features-test-lib
check "portcheck is on PATH" bash -c "command -v portcheck"
check "portcheck runs" bash -c "portcheck version | grep -q '^portcheck '"
check "portcheck finds a free port" bash -c "portcheck free 41000-41999 -o plain | tail -1 | grep -Eq '^41[0-9]{3}$'"
reportResults

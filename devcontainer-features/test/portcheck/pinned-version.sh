#!/bin/bash
set -e
source dev-container-features-test-lib
check "the pinned version is installed" bash -c "portcheck version | grep -q '^portcheck 0\.11\.1 '"
reportResults

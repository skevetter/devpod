#!/bin/bash
set -euo pipefail
echo "Installing test feature that depends on python"
# This feature should install after python is available
python3 --version
echo "Test feature installed successfully"

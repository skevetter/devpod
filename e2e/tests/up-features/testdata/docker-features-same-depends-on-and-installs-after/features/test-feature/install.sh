#!/bin/bash
set -e

echo "Installing Test Feature"
echo "Node should be available since it is in dependsOn"

if command -v node >/dev/null 2>&1; then
    echo "SUCCESS: Node is available as expected"
    echo "test-passed" >/tmp/test-result
else
    echo "ERROR: Node is not available"
    exit 1
fi

#!/bin/bash
set -e

echo "Installing dependsOnDemo feature"
echo "This feature depends on the hello feature being installed first"

# Create a test script that uses the hello command
cat >/usr/local/bin/test-depends-on <<'EOF'
#!/bin/bash
if command -v hello >/dev/null 2>&1; then
    echo "SUCCESS: hello command is available"
    hello
else
    echo "FAILURE: hello command not found - dependsOn not working"
    exit 1
fi
EOF

chmod +x /usr/local/bin/test-depends-on
echo "dependsOnDemo feature installed"

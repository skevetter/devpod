#!/bin/bash
set -e

echo "Installing top level feature"

cat >/usr/local/bin/test-nested-chain <<'EOF'
#!/bin/bash
set -e
if command -v middle-cmd >/dev/null 2>&1 && command -v hello >/dev/null 2>&1; then
    echo "All dependencies available"
    middle-cmd
    hello
else
    echo "Missing dependencies"
    exit 1
fi
EOF

chmod +x /usr/local/bin/test-nested-chain
echo "Top level feature installed"

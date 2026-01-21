#!/bin/bash
set -e

echo "Installing mixed dependencies feature"

cat >/usr/local/bin/test-install-order <<'EOF'
#!/bin/bash
if command -v hard-dep >/dev/null 2>&1 && command -v hello >/dev/null 2>&1; then
    echo "Correct order: both dependencies available"
else
    echo "Wrong order: missing dependencies"
    exit 1
fi
EOF

chmod +x /usr/local/bin/test-install-order
echo "Mixed dependencies feature installed"

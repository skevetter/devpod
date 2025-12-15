#!/bin/bash
set -e

echo "Installing hard dependency"

cat >/usr/local/bin/hard-dep <<'EOF'
#!/bin/bash
echo "Hard dependency command"
EOF

chmod +x /usr/local/bin/hard-dep
echo "Hard dependency installed"

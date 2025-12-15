#!/bin/bash
set -e

echo "Installing middle level feature"

cat >/usr/local/bin/middle-cmd <<'EOF'
#!/bin/bash
echo "Middle level command executed"
EOF

chmod +x /usr/local/bin/middle-cmd
echo "Middle level feature installed"

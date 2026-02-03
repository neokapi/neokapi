#!/bin/bash
# Double-click this file to generate CLI demo videos
# (Opens in Terminal with proper TTY for VHS)

cd "$(dirname "$0")"
./generate.sh

echo ""
echo "✅ Done! Press any key to close..."
read -n 1

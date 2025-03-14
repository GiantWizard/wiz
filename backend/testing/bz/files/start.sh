#!/bin/bash
set -e

echo "Installing dependencies..."
apt-get update
apt-get install -y curl unzip ca-certificates

echo "Installing rclone..."
curl -L https://downloads.rclone.org/rclone-current-linux-amd64.zip -o rclone.zip
unzip rclone.zip
cd rclone-*-linux-amd64 || { echo "Failed to enter rclone directory"; exit 1; }
cp rclone /usr/local/bin/
chmod 755 /usr/local/bin/rclone
cd .. || exit 1
rm -rf rclone*

echo "Configuring MEGA..."
if [ -n "$MEGA_USERNAME" ] && [ -n "$MEGA_PASSWORD" ]; then
  echo "Creating rclone config..."
  mkdir -p ~/.config/rclone
  rclone config create mega mega user "$MEGA_USERNAME" pass "$MEGA_PASSWORD" --non-interactive
  echo "Testing MEGA connection..."
  rclone lsd mega: || echo "Warning: Could not list MEGA directory"
else
  echo "Warning: MEGA credentials not found in environment variables"
fi

echo "Starting Bazaar tracker..."
chmod +x ./bazaar-tracker
./bazaar-tracker
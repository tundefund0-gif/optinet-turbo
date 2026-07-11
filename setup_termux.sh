#!/bin/bash
# OptiNet - Termux Setup Script
# Run this in Termux on your Android phone (no root needed!)

echo "================================"
echo "  OptiNet - Termux Setup"
echo "================================"
echo ""

# Update packages
echo "[1/4] Updating packages..."
pkg update -y && pkg upgrade -y

# Install Go
echo "[2/4] Installing Go..."
pkg install -y golang git

# Get the project files
echo "[3/4] Setting up OptiNet..."
cd ~
mkdir -p optinet
# Copy or clone your project here

# Build
echo "[4/4] Building..."
cd ~/optinet
go build -o optinetd ./cmd/optinetd

echo ""
echo "================================"
echo "  Setup Complete!"
echo "================================"
echo ""
echo "To run OptiNet:"
echo "  cd ~/optinet && ./optinetd"
echo ""
echo "Then open in your browser:"
echo "  http://localhost:9090"
echo ""
echo "Set WiFi proxy to:"
echo "  Host: localhost"
echo "  Port: 8080"
echo "================================"

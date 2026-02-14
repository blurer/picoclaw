#!/bin/bash
# PicoClaw Installer for Debian 12
# Installs Go, builds PicoClaw, and sets up systemd service

set -euo pipefail

GO_VERSION="1.23.4"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="$HOME/.picoclaw"
SERVICE_USER="${SUDO_USER:-$USER}"

echo "=== PicoClaw Installer for Debian 12 ==="
echo ""

# Check if running as root for system install
if [[ $EUID -ne 0 ]]; then
    echo "Note: Run with sudo for system-wide install and systemd setup"
    echo "      Running in user-mode (no systemd service)"
    SYSTEM_INSTALL=false
else
    SYSTEM_INSTALL=true
fi

# Install dependencies
echo "[1/6] Installing dependencies..."
apt-get update -qq
apt-get install -y -qq wget git make ca-certificates

# Install Go if not present or wrong version
echo "[2/6] Checking Go installation..."
if command -v go &> /dev/null; then
    CURRENT_GO=$(go version | grep -oP 'go\K[0-9]+\.[0-9]+')
    echo "      Found Go $CURRENT_GO"
else
    echo "      Installing Go $GO_VERSION..."
    wget -q "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -O /tmp/go.tar.gz
    rm -rf /usr/local/go
    tar -C /usr/local -xzf /tmp/go.tar.gz
    rm /tmp/go.tar.gz

    # Add to PATH for current session
    export PATH=$PATH:/usr/local/go/bin

    # Add to profile for future sessions
    if ! grep -q '/usr/local/go/bin' /etc/profile.d/go.sh 2>/dev/null; then
        echo 'export PATH=$PATH:/usr/local/go/bin' > /etc/profile.d/go.sh
    fi
    echo "      Go $GO_VERSION installed"
fi

# Clone or update repo
echo "[3/6] Getting PicoClaw source..."
PICOCLAW_SRC="/tmp/picoclaw-build"
if [[ -d "$PICOCLAW_SRC" ]]; then
    cd "$PICOCLAW_SRC"
    git pull -q
else
    git clone -q https://github.com/blurer/picoclaw.git "$PICOCLAW_SRC"
    cd "$PICOCLAW_SRC"
fi

# Build
echo "[4/6] Building PicoClaw..."
export PATH=$PATH:/usr/local/go/bin
go mod tidy

# Copy workspace for embedding
cp -r workspace ./cmd/picoclaw/workspace

go build -o picoclaw ./cmd/picoclaw

# Clean up embedded workspace
rm -rf ./cmd/picoclaw/workspace

# Install binary
echo "[5/6] Installing binary..."
cp picoclaw "$INSTALL_DIR/picoclaw"
chmod +x "$INSTALL_DIR/picoclaw"

# Initialize config
echo "[6/6] Setting up configuration..."
if [[ ! -d "$CONFIG_DIR" ]]; then
    sudo -u "$SERVICE_USER" "$INSTALL_DIR/picoclaw" onboard
fi

# Copy example configs if they don't exist
if [[ ! -f "$CONFIG_DIR/config.json" ]]; then
    cat > "$CONFIG_DIR/config.json" << 'EOF'
{
  "agents": {
    "defaults": {
      "provider": "ollama",
      "model": "llama3.2",
      "max_tokens": 8192,
      "temperature": 0.7
    }
  },
  "providers": {
    "ollama": {
      "api_base": "http://localhost:11434"
    }
  }
}
EOF
    chown "$SERVICE_USER:$SERVICE_USER" "$CONFIG_DIR/config.json"
fi

# Setup systemd service if system install
if [[ "$SYSTEM_INSTALL" == true ]]; then
    echo ""
    echo "[+] Setting up systemd service..."

    cat > /etc/systemd/system/picoclaw.service << EOF
[Unit]
Description=PicoClaw AI Assistant Gateway
After=network.target

[Service]
Type=simple
User=$SERVICE_USER
WorkingDirectory=$CONFIG_DIR
ExecStart=$INSTALL_DIR/picoclaw gateway
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal

# Environment file for Ollama config
EnvironmentFile=-$CONFIG_DIR/.env

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    echo "      Service installed: picoclaw.service"
    echo ""
    echo "=== Installation Complete ==="
    echo ""
    echo "Commands:"
    echo "  picoclaw agent              # Interactive chat"
    echo "  picoclaw agent -m 'Hello'   # One-shot message"
    echo ""
    echo "Systemd service:"
    echo "  sudo systemctl start picoclaw    # Start gateway"
    echo "  sudo systemctl enable picoclaw   # Start on boot"
    echo "  sudo systemctl status picoclaw   # Check status"
    echo "  journalctl -u picoclaw -f        # View logs"
    echo ""
    echo "Config: $CONFIG_DIR/config.json"
else
    echo ""
    echo "=== Installation Complete ==="
    echo ""
    echo "Commands:"
    echo "  picoclaw agent              # Interactive chat"
    echo "  picoclaw agent -m 'Hello'   # One-shot message"
    echo "  picoclaw gateway            # Start gateway (foreground)"
    echo ""
    echo "Config: $CONFIG_DIR/config.json"
fi

echo ""
echo "Edit config to set your Ollama endpoint:"
echo "  nano $CONFIG_DIR/config.json"

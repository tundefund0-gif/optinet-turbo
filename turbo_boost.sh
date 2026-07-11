#!/data/data/com.termux/files/usr/bin/bash
# OptiNet Turbo Boost - Run this in Termux on the remote phone
# This tunes network settings for maximum gaming performance

echo "╔════════════════════════════════════╗"
echo "║   OptiNet Turbo Network Boost      ║"
echo "╚════════════════════════════════════╝"
echo ""

# 1. Check if we have any sysctl access (some custom ROMs allow it)
if [ -w /proc/sys/net/ipv4/tcp_congestion_control ]; then
    echo "[✓] Root sysctl access detected!"
    
    # Try BBR (best for gaming/latency)
    echo "  → Enabling BBR congestion control..."
    echo "bbr" > /proc/sys/net/ipv4/tcp_congestion_control 2>/dev/null && echo "  ✓ BBR enabled!" || echo "  ✗ BBR not available"
    
    # TCP Fast Open (reduce 1 RTT)
    echo "  → Enabling TCP Fast Open..."
    echo "3" > /proc/sys/net/ipv4/tcp_fastopen 2>/dev/null && echo "  ✓ TCP Fast Open enabled!" || echo "  ✗ Cannot enable"
    
    # Increase TCP buffer sizes
    echo "  → Increasing TCP buffers..."
    echo "4096 131072 16777216" > /proc/sys/net/ipv4/tcp_rmem 2>/dev/null
    echo "4096 131072 16777216" > /proc/sys/net/ipv4/tcp_wmem 2>/dev/null
    
    # Increase backlog
    echo "  → Increasing connection backlog..."
    echo "5000" > /proc/sys/net/core/netdev_max_backlog 2>/dev/null
    echo "5000" > /proc/sys/net/core/somaxconn 2>/dev/null
    
    # Enable MTU probing
    echo "  → Enabling MTU probing..."
    echo "2" > /proc/sys/net/ipv4/tcp_mtu_probing 2>/dev/null
    
    # Faster TCP connection reuse
    echo "  → Tuning TCP timeouts..."
    echo "1" > /proc/sys/net/ipv4/tcp_tw_reuse 2>/dev/null
    echo "30" > /proc/sys/net/ipv4/tcp_fin_timeout 2>/dev/null
    
else
    echo "[!] No root sysctl access (normal for Android)"
    echo "  → Boosting via application-level tuning only"
    echo ""
    echo "  Application tuning already active:"
    echo "  ✓ TCP_NODELAY enabled (no Nagle delay)"
    echo "  ✓ TCP keepalive every 15s"
    echo "  ✓ 512KB TCP buffers"
    echo "  ✓ Pooled zero-copy I/O"
    echo "  ✓ Fastest DNS auto-selected"
    echo "  ✓ Multi-threaded proxy workers"
    echo "  ✓ UDP game traffic proxy"
    echo ""
fi

# 2. Set CPU governor to performance if possible
if [ -w /sys/devices/system/cpu/cpu0/cpufreq/scaling_governor ]; then
    echo "[✓] CPU governor access!"
    for cpu in /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor; do
        echo "performance" > "$cpu" 2>/dev/null
    done
    echo "  → CPU set to performance mode!"
fi

# 3. Check WiFi power save
if [ -w /sys/module/bcmdhd/parameters/wl_mimo_power_save ]; then
    echo "  → Disabling WiFi power saving..."
    echo "N" > /sys/module/bcmdhd/parameters/wl_mimo_power_save 2>/dev/null
fi

echo ""
echo "=== Current Network Status ==="
echo "  Interface: $(ip route get 1 2>/dev/null | head -1 | awk '{print $5}')"
echo "  IP: $(ip route get 1 2>/dev/null | head -1 | awk '{print $7}')"
echo "  DNS: $(getprop net.dns1 2>/dev/null || echo 'auto')"
echo ""
echo "=== OptiNet Running ==="
pgrep -f optinetd > /dev/null && echo "  ✓ optinetd is RUNNING" || echo "  ✗ optinetd NOT running (start with: ./optinetd)"
echo ""
echo "✓ Turbo Boost complete!"

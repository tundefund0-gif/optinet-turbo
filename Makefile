.PHONY: build run clean termux-setup

BINARY=optinetd

build:
	go build -o $(BINARY) ./cmd/optinetd

run: build
	./$(BINARY)

clean:
	rm -f $(BINARY)

termux-setup:
	pkg update && pkg upgrade -y
	pkg install -y golang git
	go mod download
	go build -o $(BINARY) ./cmd/optinetd
	@echo ""
	@echo "=== OptiNet installed! Run with: ./optinetd ==="

phone-setup:
	@echo "============================================"
	@echo "  OptiNet - Phone Setup Guide"
	@echo "============================================"
	@echo ""
	@echo "1. Install Termux from F-Droid:"
	@echo "   https://f-droid.org/packages/com.termux/"
	@echo ""
	@echo "2. Install Go in Termux:"
	@echo "   pkg update && pkg install golang git -y"
	@echo ""
	@echo "3. Copy this project to your phone and build:"
	@echo "   cd optinet && go build -o optinetd ./cmd/optinetd"
	@echo ""
	@echo "4. Run the server:"
	@echo "   ./optinetd"
	@echo ""
	@echo "5. Open browser on your phone to:"
	@echo "   http://localhost:9090"
	@echo ""
	@echo "6. Configure proxy on your WiFi:"
	@echo "   Settings > WiFi > Proxy > Manual"
	@echo "   Host: localhost  Port: 8080"
	@echo ""
	@echo "7. VPN Overlay mode:"
	@echo "   The SOCKS5 proxy runs on port 1080"
	@echo "   Use a VPN app like Drony/Postern to route"
	@echo "   all traffic through the SOCKS5 proxy"
	@echo "============================================"

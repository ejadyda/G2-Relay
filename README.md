# g2ray-lite-forwarder

A lightweight educational reverse proxy for GitHub Codespaces. This project does **not** run Xray/VLESS and does **not** generate VLESS links. It only forwards HTTP and WebSocket traffic from a public Codespaces port to your configured target server.

## Important Notes

- **Educational Purpose Only**: This repository is for educational and testing purposes only.
- **Not for Infrastructure Abuse**: Do not use this to abuse GitHub infrastructure or bypass GitHub policies.
- **Comply with GitHub Terms**: Users should comply with [GitHub Terms of Service](https://docs.github.com/en/site-policy/github-terms/github-terms-of-service) and local laws.
- **Use Secondary Account**: Preferably do not run experimental Codespaces like this on your main GitHub account to avoid account restrictions.

## Quick Start

### 1. Fork This Repository

Click "Fork" to create your own copy of this repository.

### 2. Configure GitHub Codespaces Settings (Recommended)

Before creating a Codespace, optimize your settings:

1. Go to **GitHub Settings** → **Codespaces**
2. Set **Default region** to a region close to your target server (e.g., Europe West, Asia Southeast)
3. Set **Default idle timeout** to **240 minutes** (4 hours)

These settings improve performance and reduce costs.

### 3. Edit config.json

Edit [`config.json`](config.json) with your target server details:

```json
{
  "listen_addr": "0.0.0.0:3000",
  "target_host": "YOUR_TARGET_IP",
  "target_port": "YOUR_TARGET_PORT",
  "target_scheme": "http"
}
```

Replace:
- `YOUR_TARGET_IP`: IP address of your target server
- `YOUR_TARGET_PORT`: Port number of your target server
- `target_scheme`: Use `"http"` or `"https"` depending on your target

### 4. Create a Codespace

Click **Code** → **Codespaces** → **Create codespace on main**

### 5. Verify Port 3000 is Public

1. Wait for the Codespace to start and `postStartCommand` to finish
2. Open the **Ports** tab in the Codespace UI
3. Find port **3000** and ensure its **Visibility** is set to **Public**
4. If not public, click the visibility icon and change it to **Public**

### 6. Test the Proxy

Test that the proxy is working:

```bash
# Test locally
curl -I http://127.0.0.1:3000/health

# Test via public Codespaces URL (replace YOUR-CODESPACE-HOST with your actual host)
curl -I https://YOUR-CODESPACE-HOST/health
```

If both return `HTTP/1.1 200 OK`, the proxy is working.

## Configuration

All settings are in [`config.json`](config.json):

| Field | Type | Purpose |
|-------|------|---------|
| `listen_addr` | string | Address and port to listen on (e.g., `"0.0.0.0:3000"`) |
| `target_host` | string | Target server IP address |
| `target_port` | string | Target server port (as string) |
| `target_scheme` | string | Scheme for target server: `"http"` or `"https"` |

The proxy will **fail to start** if `config.json` is missing or any required field is empty.

## How It Works

The proxy works like this:

```
Client
  ↓
HTTPS://YOUR-CODESPACE-HOST:443
  ↓
[Codespaces automatic HTTPS termination]
  ↓
HTTP://localhost:3000 (this proxy)
  ↓
HTTP/WebSocket → target_host:target_port
  ↓
Your target server
```

The proxy:
- Listens on the configured `listen_addr`
- Forwards HTTP requests via reverse proxy
- Upgrades and tunnels WebSocket connections
- Provides a `/health` endpoint for health checks

## GitHub Codespaces Lifecycle

### Automatic Stopping

GitHub Codespaces automatically stops after **inactivity**. The idle timeout depends on your settings:

- Default: 30 minutes
- If set to 240 minutes: ~4 hours of inactivity

You can also manually stop a Codespace to save resources and reduce usage.

### Restarting

Once stopped, you can restart the Codespace from the **Codespaces** tab on GitHub:

1. Go to your repository
2. Click **Code** → **Codespaces**
3. Click the Codespace to restart it

When restarted, `postStartCommand` runs automatically and the proxy starts.

### Stopping a Codespace

To manually stop a running Codespace and save resources:

1. Open the **terminal** in the Codespace or use GitHub web UI
2. Click **Stop Codespace** in the Codespaces menu
3. Or visit your [Codespaces dashboard](https://github.com/codespaces)

## GitHub Free Usage Limits

GitHub Free tier provides:

- **120 core-hours per month**
- On a **2-core Codespace**: ~60 real hours per month
- On a **4-core Codespace**: ~30 real hours per month

Monitor your usage on the [Codespaces billing page](https://github.com/settings/billing/summary).

## Troubleshooting

### Health Check Works, WebSocket Does Not

If `curl -I http://127.0.0.1:3000/health` works but your application cannot connect:

1. **Check target server configuration**: Ensure the target server is reachable from the Codespace and the WebSocket endpoint is correctly configured.
2. **Inspect proxy logs**: Look at the Codespace terminal output for `[WS]` log entries showing connection attempts.
3. **Verify target accessibility**: From the Codespace terminal, try: `curl -I http://TARGET_HOST:TARGET_PORT/`
4. **Check firewall**: Ensure the target server's firewall allows connections from GitHub Codespaces IP ranges.

### Public URL Does Not Work

If the public Codespaces URL does not work:

1. **Check port visibility**: Open **Ports** tab → verify port 3000 is set to **Public**
2. **Wait for registration**: Sometimes it takes a few seconds for port changes to take effect
3. **Restart the Codespace**: Manually stop and restart the Codespace

### Cannot Connect to Target Server

If the proxy cannot reach the target server:

1. **Verify config.json**: Ensure `target_host`, `target_port`, and `target_scheme` are correct
2. **Check firewall**: Ensure your target server firewall allows connections from GitHub Codespaces
3. **Test connectivity**: From the Codespace terminal, try pinging or curling the target server
4. **Region mismatch**: If set to a distant region, latency may increase. Choose a region closer to your target

### Codespace Stopped Due to Inactivity

This is normal behavior. If your idle timeout is set to 240 minutes:

- The Codespace will stop after ~4 hours of **no activity**
- You can restart it anytime from GitHub Codespaces dashboard
- No data is lost; your `/root/` directory is preserved

### Configuration Not Loading

If you get an error like `configuration error: ...`:

1. **Check config.json exists**: Ensure `config.json` is in the root of the repository
2. **Validate JSON syntax**: Use an online JSON validator to check for syntax errors
3. **Check for required fields**: Ensure all fields are present: `listen_addr`, `target_host`, `target_port`, `target_scheme`
4. **No empty values**: All fields must have non-empty string values

## Testing Commands

```bash
# Health check (local)
curl -I http://127.0.0.1:3000/health

# Health check (via Codespaces public URL)
curl -I https://YOUR-CODESPACE-HOST/health

# Verbose test with headers
curl -v -I https://YOUR-CODESPACE-HOST/health

# Test with custom User-Agent
curl -H "User-Agent: test-client" -I http://127.0.0.1:3000/health
```

## Project Structure

```
.
├── main.go              # Go proxy server
├── config.json          # Configuration file (required)
├── start.sh             # Startup script
├── .devcontainer/       # Codespaces configuration
│   └── devcontainer.json
├── go.mod               # Go module definition
└── README.md            # This file
```

## Development

To develop locally without Codespaces:

1. Edit `config.json` with your target details
2. Run: `go run ./main.go`
3. The proxy listens on the address specified in `config.json`

To build a standalone binary:

```bash
go build -o g2ray-lite-forwarder ./main.go
./g2ray-lite-forwarder
```

## License

MIT

## Disclaimer

This software is provided as-is for educational purposes. Users are responsible for complying with GitHub's Terms of Service, applicable laws, and regulations in their jurisdiction. The authors assume no liability for misuse or unauthorized access.


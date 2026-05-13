# g2ray-lite-forwarder

A lightweight WebSocket reverse proxy for GitHub Codespaces.

**IMPORTANT:** This project does NOT run Xray/VLESS inside Codespaces. It only forwards WebSocket traffic to your own external VLESS WebSocket server.

## Quick Start

### 1. Set Required Environment Variable

In GitHub Codespaces Secrets, add:

```
VLESS_UUID = 360631a7-d3fd-4e82-9de7-342d1d28982e
```

(Replace with your actual VLESS UUID - do NOT commit this to the repository)

### 2. Run the Proxy

```bash
bash ./start.sh
```

The script will:
- Start the proxy listening on port 3000
- Automatically make port 3000 public (via GitHub CLI if available)
- Print your final VLESS connection link

### 3. Connect Using the Printed VLESS Link

The startup output will show:

```
vless://YOUR_UUID@codespace-name-3000.app.github.dev:443?encryption=none&security=tls&sni=codespace-name-3000.app.github.dev&fp=chrome&type=ws&host=codespace-name-3000.app.github.dev&path=%2F#g2ray-lwq4w11y
```

## Configuration

All settings via environment variables:

| Variable | Default | Purpose |
|----------|---------|---------|
| `VLESS_UUID` | (empty) | **Required.** Your VLESS username/UUID. Set via Codespaces Secrets. |
| `LISTEN_ADDR` | `0.0.0.0:3000` | Port on which proxy listens |
| `TARGET_HOST` | `212.95.41.118` | Your VLESS WebSocket server IP |
| `TARGET_PORT` | `48560` | Your VLESS WebSocket server port |
| `TARGET_SCHEME` | `http` | Scheme to use when forwarding (`http` or `https`) |
| `VLESS_PATH` | `/` | WebSocket path on target server |
| `LINK_NAME` | `g2ray-lwq4w11y` | Fragment/name in generated VLESS link |
| `CLIENT_ADDRESS_OVERRIDE` | (empty) | Use this IP in generated link instead of Codespaces hostname |

### Example with Override

If you want to use a custom address in the VLESS link:

```bash
export CLIENT_ADDRESS_OVERRIDE=94.130.50.12
bash ./start.sh
```

This generates a link with:
- Address: `94.130.50.12`
- Host/SNI: `codespace-name-3000.app.github.dev` (Codespaces hostname)

## Default Target

Default target server:

```text
212.95.41.118:48560
```

## Troubleshooting

### Check Health Endpoint

Test if the proxy is working:

```bash
# Test locally
curl -I http://127.0.0.1:3000/health

# Test through Codespaces domain (replace with your actual domain)
curl -I https://codespace-name-3000.app.github.dev/health

# Test with IP override (useful for debugging external IP issues)
curl -I --resolve codespace-name-3000.app.github.dev:443:94.130.50.12 \
  https://codespace-name-3000.app.github.dev/health
```

### Debugging

When `/health` works but VLESS doesn't connect:

1. **If `/health` via Codespaces HTTPS works** → WebSocket forwarding issue
   - Check target server is reachable: `curl -I http://212.95.41.118:48560/`
   - Verify WebSocket upgrade headers are correct
   - Check proxy logs for `[WS]` entries

2. **If `/health` via `--resolve` IP fails** → External IP/SNI/routing issue
   - The IP you're using doesn't reach your Codespaces server
   - Firewall or load balancer configuration issue
   - Not a proxy problem

3. **If both work but VLESS clients can't connect** → Client configuration
   - Verify VLESS link parameters match your setup
   - Check if `security=tls` and certificate validation is needed
   - Test with: `xray -config config.json` (enable log level `debug`)

### Logs

The proxy prints detailed logs including:

- `[WS]` - WebSocket connection details and errors
- `[HTTP]` - HTTP request forwarding  
- `[HEALTH]` - Health check requests
- Connection establishment and errors

### Port Visibility

Port 3000 should be public automatically. To manually set it:

```bash
gh codespace ports visibility 3000:public -c "$CODESPACE_NAME"
```

## Architecture

```
Client
  ↓
HTTPS:// codespace-name-3000.app.github.dev:443
  ↓
[Codespaces HTTPS termination]
  ↓
HTTP://localhost:3000 (this proxy)
  ↓
TCP://212.95.41.118:48560 (your WebSocket server)
  ↓
Your VLESS/Xray backend
```

The proxy forwards the WebSocket upgrade request as-is to preserve the connection protocol.

## Security Notes

- **VLESS_UUID is sensitive** - only set via Codespaces Secrets, never commit to repo
- **Port 3000 is public** - this is intentional for VLESS connections
- **No authentication** between Codespaces and your backend server - add firewall rules if needed
- **HTTPS terminates at Codespaces** - connection to backend depends on `TARGET_SCHEME`

## License

MIT

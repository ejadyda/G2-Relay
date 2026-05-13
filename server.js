const http = require("http");
const httpProxy = require("http-proxy");

const LISTEN_HOST = process.env.LISTEN_HOST || "0.0.0.0";
const LISTEN_PORT = Number(process.env.PORT || process.env.LISTEN_PORT || 3000);

const TARGET_HOST = process.env.TARGET_HOST || "212.95.41.118";
const TARGET_PORT = Number(process.env.TARGET_PORT || 48560);
const TARGET_SCHEME = process.env.TARGET_SCHEME || "http";
const TARGET_URL = `${TARGET_SCHEME}://${TARGET_HOST}:${TARGET_PORT}`;

const VLESS_UUID = process.env.VLESS_UUID || "e6c16592-f8bf-4032-9a8d-1dcf9e8a5e94";
const VLESS_PATH = process.env.VLESS_PATH || "/";
const LINK_NAME = process.env.LINK_NAME || "g2ray-lite";

const proxy = httpProxy.createProxyServer({
  target: TARGET_URL,
  ws: true,
  changeOrigin: false,
  xfwd: true,
  secure: false,
  timeout: 0,
  proxyTimeout: 0
});

function getCodespacePublicHost() {
  const codespaceName = process.env.CODESPACE_NAME;
  const domain = process.env.GITHUB_CODESPACES_PORT_FORWARDING_DOMAIN || "app.github.dev";

  if (!codespaceName) return null;

  return `${codespaceName}-${LISTEN_PORT}.${domain}`;
}

function buildVlessLink(publicHost) {
  const params = new URLSearchParams({
    type: "ws",
    encryption: "none",
    security: "tls",
    path: VLESS_PATH,
    host: publicHost,
    sni: publicHost
  });

  return `vless://${VLESS_UUID}@${publicHost}:443?${params.toString()}#${encodeURIComponent(LINK_NAME)}`;
}

const server = http.createServer((req, res) => {
  if (req.url === "/health" || req.url === "/") {
    res.writeHead(200, {
      "content-type": "text/plain; charset=utf-8",
      "cache-control": "no-store"
    });

    res.end(
      [
        "g2ray-lite-forwarder is running.",
        `Target: ${TARGET_URL}`,
        "",
        "This service only forwards WebSocket traffic to your own VLESS server."
      ].join("\n")
    );

    return;
  }

  proxy.web(req, res, {}, (err) => {
    console.error("[HTTP proxy error]", err.message);

    if (!res.headersSent) {
      res.writeHead(502, { "content-type": "text/plain; charset=utf-8" });
    }

    res.end("Bad Gateway");
  });
});

server.on("upgrade", (req, socket, head) => {
  console.log(`[WS] ${req.socket.remoteAddress} ${req.url} -> ${TARGET_URL}`);
  proxy.ws(req, socket, head);
});

proxy.on("error", (err, req, res) => {
  console.error("[Proxy error]", err.message);

  if (res && !res.headersSent) {
    res.writeHead(502, { "content-type": "text/plain; charset=utf-8" });
    res.end("Proxy error");
  }
});

server.listen(LISTEN_PORT, LISTEN_HOST, () => {
  const publicHost = getCodespacePublicHost();

  console.log("");
  console.log("============================================================");
  console.log("g2ray-lite-forwarder started");
  console.log(`Listening: http://${LISTEN_HOST}:${LISTEN_PORT}`);
  console.log(`Target:    ${TARGET_URL}`);
  console.log("============================================================");

  if (publicHost) {
    console.log("");
    console.log("Public Codespaces URL:");
    console.log(`https://${publicHost}`);
    console.log("");
    console.log("Final VLESS link:");
    console.log(buildVlessLink(publicHost));
    console.log("");
  } else {
    console.log("");
    console.log("Not running inside GitHub Codespaces.");
    console.log("Open this URL locally:");
    console.log(`http://127.0.0.1:${LISTEN_PORT}`);
    console.log("");
  }
});

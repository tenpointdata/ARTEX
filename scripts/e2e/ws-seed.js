// Records a WebSocket exchange through the ARTEX recording proxy, so the traffic UI
// has a stream to show. Starts a loopback echo server, opens ws:// to it through the
// proxy (a CONNECT tunnel, as a browser behind an HTTP proxy does),
// sends `messages`, reads each echo, closes. Only Node's standard library is used.
//
//   const { seedWebSocket } = require("./ws-seed");
//   await seedWebSocket({ proxy: "127.0.0.1:28788", messages: ["hello", "world"] });
//
// Resolves with { url, echoed } once the server echoed every message.
const crypto = require("node:crypto");
const http = require("node:http");
const net = require("node:net");

const GUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11";
const CLOSE_NORMAL = Buffer.from([0x03, 0xe8]); // close status 1000

// frame encodes one final text frame (opcode 1) or close (8); clients must mask.
function frame(opcode, data, mask) {
  const payload = Buffer.isBuffer(data) ? data : Buffer.from(data, "utf8");
  if (payload.length > 125) throw new Error("ws-seed: messages must be at most 125 bytes");
  const head = Buffer.from([0x80 | opcode, (mask ? 0x80 : 0) | payload.length]);
  if (!mask) return Buffer.concat([head, payload]);
  const key = crypto.randomBytes(4);
  const body = Buffer.from(payload.map((b, i) => b ^ key[i % 4]));
  return Buffer.concat([head, key, body]);
}

// readFrames parses complete short frames off buf; returns [frames, rest].
function readFrames(buf) {
  const frames = [];
  for (;;) {
    if (buf.length < 2) break;
    const opcode = buf[0] & 0x0f;
    const masked = (buf[1] & 0x80) !== 0;
    const len = buf[1] & 0x7f;
    if (len > 125) throw new Error("ws-seed: unexpected long frame");
    const need = 2 + (masked ? 4 : 0) + len;
    if (buf.length < need) break;
    let payload = buf.subarray(need - len, need);
    if (masked) {
      const key = buf.subarray(2, 6);
      payload = Buffer.from(payload.map((b, i) => b ^ key[i % 4]));
    }
    frames.push({ opcode, text: payload.toString("utf8") });
    buf = buf.subarray(need);
  }
  return [frames, buf];
}

function startEchoServer() {
  const server = http.createServer((_, res) => res.writeHead(426).end());
  server.on("upgrade", (req, sock) => {
    const accept = crypto.createHash("sha1").update(req.headers["sec-websocket-key"] + GUID).digest("base64");
    sock.write(`HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: ${accept}\r\n\r\n`);
    let buf = Buffer.alloc(0);
    sock.on("data", (d) => {
      let frames;
      [frames, buf] = readFrames(Buffer.concat([buf, d]));
      for (const f of frames) {
        if (f.opcode === 8) { sock.end(frame(8, CLOSE_NORMAL, false)); return; }
        if (f.opcode === 1) sock.write(frame(1, `echo:${f.text}`, false));
      }
    });
    sock.on("error", () => {});
  });
  return new Promise((resolve) => server.listen(0, "127.0.0.1", () => resolve(server)));
}

async function seedWebSocket({ proxy, messages, path = "/ws/probe", timeoutMs = 15000 }) {
  const server = await startEchoServer();
  const url = `ws://127.0.0.1:${server.address().port}${path}`;
  const [phost, pport] = proxy.split(":");
  try {
    const echoed = await new Promise((resolve, reject) => {
      const got = [];
      const fail = (e) => { clearTimeout(timer); sock.destroy(); reject(e); };
      const timer = setTimeout(() => fail(new Error(`ws-seed: timed out (got ${got.length} echoes)`)), timeoutMs);
      const sock = net.connect(Number(pport), phost);
      const key = crypto.randomBytes(16).toString("base64");
      const target = new URL(url).host;
      sock.on("connect", () => sock.write(`CONNECT ${target} HTTP/1.1\r\nHost: ${target}\r\n\r\n`));
      const handshake = `GET ${path} HTTP/1.1\r\nHost: ${target}\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n` +
        `Sec-WebSocket-Key: ${key}\r\nSec-WebSocket-Version: 13\r\n\r\n`;
      let buf = Buffer.alloc(0);
      let tunneled = false;
      let upgraded = false;
      let done = false;
      // Reads one HTTP head off buf; null until it is complete.
      const head = () => {
        const end = buf.indexOf("\r\n\r\n");
        if (end < 0) return null;
        const status = buf.subarray(0, buf.indexOf("\r\n")).toString();
        buf = buf.subarray(end + 4);
        return status;
      };
      sock.on("data", (d) => {
        buf = Buffer.concat([buf, d]);
        if (!tunneled) {
          const status = head();
          if (status === null) return;
          if (!/ 200 /.test(status)) { fail(new Error(`ws-seed: CONNECT answered ${status}`)); return; }
          tunneled = true;
          sock.write(handshake);
        }
        if (!upgraded) {
          const status = head();
          if (status === null) return;
          if (!/ 101 /.test(status)) { fail(new Error(`ws-seed: upgrade answered ${status}`)); return; }
          upgraded = true;
          for (const m of messages) sock.write(frame(1, m, true));
        }
        let frames;
        [frames, buf] = readFrames(buf);
        for (const f of frames) if (f.opcode === 1) got.push(f.text);
        if (got.length >= messages.length && !done) {
          done = true;
          clearTimeout(timer);
          sock.write(frame(8, CLOSE_NORMAL, true));
          setTimeout(() => { sock.destroy(); resolve(got); }, 300);
        }
      });
      sock.on("error", (e) => { if (!done) fail(e); });
    });
    return { url, echoed };
  } finally {
    server.closeAllConnections();
    server.close();
  }
}

module.exports = { seedWebSocket };

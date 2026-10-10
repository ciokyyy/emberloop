// acp-tee.mjs — throwaway timing probe (streaming-burst investigation).
//
// Stdio proxy in front of the real ACP adapter. Forwards bytes unchanged in
// both directions and logs one timestamped line per ACP message so a later
// analysis can tell whether assistant chunks left the adapter spaced out
// (live streaming) or as one burst (consolidated flush):
//
//   node acp-tee.mjs --log <file> -- <adapter> [args...]
//
// Log columns (TSV): monoNs \t teePid \t dir \t kind \t detail \t bytes
//   dir: S2C (adapter stdout) or C2S (our stdin).
import { spawn } from "node:child_process";
import fs from "node:fs";
import { Transform } from "node:stream";

const args = process.argv.slice(2);
const logIdx = args.indexOf("--log");
if (logIdx === -1 || !args[logIdx + 1] || args[logIdx + 2] !== "--") {
  console.error("usage: node acp-tee.mjs --log <file> -- <adapter> [args...]");
  process.exit(2);
}
const pid = process.pid;
const logFile = args[logIdx + 1];
const childArgs = args.slice(logIdx + 3);
const log = fs.createWriteStream(logFile, { flags: "a" });

function describeLine(line) {
  let msg;
  try {
    msg = JSON.parse(line);
  } catch {
    return `raw\t-\t${line.length}`;
  }
  if (msg.method === "session/update") {
    const u = msg.params?.update ?? {};
    const c = u.content;
    const clen = typeof c === "string" ? c.length : JSON.stringify(c ?? "").length;
    return `notify\tsession/update:${u.sessionUpdate ?? "?"}:${clen}\t${line.length}`;
  }
  if (msg.method) {
    let detail = msg.method;
    if (msg.method === "session/prompt") {
      detail = `session/prompt:chars=${JSON.stringify(msg.params?.prompt).length}`;
    }
    return `request\t${detail}\t${line.length}`;
  }
  if (msg.id !== undefined && (msg.result !== undefined || msg.error !== undefined)) {
    return `result\t-\t${line.length}`;
  }
  // Claude CLI stream-json (SDK subprocess protocol): sniff type/event so
  // CLI emission pacing is analyzable with the same log.
  if (typeof msg.type === "string") {
    let detail = msg.type;
    const ev = msg.event?.type || msg.message?.stop_reason || "";
    if (ev) detail += `:${ev}`;
    if (msg.type === "stream_event" && msg.event) {
      const d = msg.event.delta?.type || msg.event.content_block?.type || "";
      if (d) detail += `:${d}`;
    }
    return `cli\t${detail}\t${line.length}`;
  }
  return `other\t-\t${line.length}`;
}

function tap(dir) {
  let buf = "";
  return new Transform({
    transform(chunk, _enc, cb) {
      const s = chunk.toString("utf8");
      buf += s;
      let i;
      while ((i = buf.indexOf("\n")) !== -1) {
        const line = buf.slice(0, i);
        buf = buf.slice(i + 1);
        if (line.trim()) {
          const [kind, detail, bytes] = describeLine(line).split("\t");
          log.write(`${process.hrtime.bigint()}\t${pid}\t${dir}\t${kind}\t${detail}\t${bytes}\n`);
        }
      }
      cb(null, chunk);
    },
  });
}

const child = spawn(childArgs[0], childArgs.slice(1), { stdio: ["pipe", "pipe", "inherit"] });
child.on("exit", (code, sig) => process.exit(code ?? (sig ? 1 : 0)));
child.on("error", (e) => {
  console.error(`acp-tee: spawn failed: ${e}`);
  process.exit(1);
});
// The harness may close the pipes abruptly at shutdown; never die noisily.
process.stdin.on("error", () => {});
process.stdout.on("error", () => {});
child.stdin.on("error", () => {});
child.stdout.on("error", () => {});
process.stdin.pipe(child.stdin);
child.stdout.pipe(tap("S2C")).pipe(process.stdout);
// Client-to-adapter requests (prompt boundaries) get timestamps too. A tap
// stream must be flowing to observe; sink it to /dev/null.
process.stdin.pipe(tap("C2S")).pipe(fs.createWriteStream("/dev/null"));

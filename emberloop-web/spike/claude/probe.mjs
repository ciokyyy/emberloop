// Spike probe: claude Backend through the ACP adapter.
//
// Starts @agentclientprotocol/claude-agent-acp as a subprocess, talks
// ACP over stdio (line-delimited JSON-RPC), creates a stripped session
// (native tools off, only the spike MCP server allowed), sends a
// prompt, prints every update, tries a prompt mid-turn, then restarts
// the adapter and tries session/load. Findings go to
// .scratch/archive/enderbloom-web/spike-findings.md.
import { spawn } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import readline from "node:readline";

const log = (tag, obj) => console.log(`${tag} ${JSON.stringify(obj)}`);
let nextId = 1;
const pending = new Map();

function send(proc, method, params) {
  const id = nextId++;
  const p = new Promise((resolve, reject) => pending.set(id, { resolve, reject }));
  proc.stdin.write(JSON.stringify({ jsonrpc: "2.0", id, method, params }) + "\n");
  return p;
}

function attach(proc, label, onEvent) {
  const rl = readline.createInterface({ input: proc.stdout, crlfDelay: Infinity });
  rl.on("line", (line) => {
    if (!line.trim()) return;
    let msg;
    try { msg = JSON.parse(line); } catch { console.log(`RAW ${line.slice(0, 300)}`); return; }
    if (msg.id !== undefined && (msg.result !== undefined || msg.error !== undefined)) {
      const p = pending.get(msg.id);
      if (!p) { log("UNMATCHED-RESP", msg); return; }
      pending.delete(msg.id);
      if (msg.error) p.reject(new Error(JSON.stringify(msg.error)));
      else p.resolve(msg.result);
      return;
    }
    onEvent(label, msg);
  });
  proc.stderr.on("data", (d) => process.stderr.write(`[${label}:stderr] ${d}`));
}

function handleClientMessage(msg, proc) {
  // Client-side requests from the adapter: permission, fs, terminals.
  if (msg.method === "session/request_permission") {
    log("PERMISSION-REQUEST", msg.params);
    const options = msg.params?.options ?? [];
    const allow = options.find((o) => o.kind === "allow" || /allow/i.test(o.optionId + o.name + (o.title ?? ""))) ?? options[0];
    const resp = { outcome: allow ? "selected" : "cancelled", ...(allow ? { optionId: allow.optionId } : {}) };
    log("PERMISSION-ANSWER", resp);
    proc.stdin.write(JSON.stringify({ jsonrpc: "2.0", id: msg.id, result: resp }) + "\n");
    return true;
  }
  if (msg.id !== undefined && msg.method) {
    log("CLIENT-REQUEST-UNHANDLED", { method: msg.method, params: msg.params });
    proc.stdin.write(JSON.stringify({ jsonrpc: "2.0", id: msg.id, error: { code: -32601, message: "probe: unsupported" } }) + "\n");
    return true;
  }
  return false;
}

async function drive(label, { loadSessionId = null } = {}) {
  const proc = spawn("node", ["./node_modules/@agentclientprotocol/claude-agent-acp/dist/index.js"], { stdio: ["pipe", "pipe", "pipe"] });
  const updates = [];
  attach(proc, label, (l, msg) => {
    if (handleClientMessage(msg, proc)) return;
    if (msg.method === "session/update") {
      updates.push(msg.params.update);
      const u = msg.params.update;
      const kind = u.sessionUpdate;
      let detail = "";
      if (kind === "agent_message_chunk") detail = JSON.stringify(u.content).slice(0, 200);
      else if (kind === "agent_thought_chunk") detail = JSON.stringify(u.content).slice(0, 120);
      else if (kind === "tool_call") detail = `${u.toolCallId} ${u.title ?? ""} kind=${u.kind ?? ""}`;
      else if (kind === "tool_call_update") detail = `${u.toolCallId} status=${u.status ?? ""}`;
      else detail = JSON.stringify(u).slice(0, 300);
      console.log(`UPDATE ${kind} ${detail}`);
      return;
    }
    log("MSG", { method: msg.method ?? null, params: msg.params ?? msg });
  });

  const cwd = process.env.SPIKE_CWD || fs.mkdtempSync(path.join(os.tmpdir(), "claude-spike-"));
  console.log(`CWD ${cwd}`);
  const init = await send(proc, "initialize", { protocolVersion: 1, clientCapabilities: { fs: { readTextFile: true, writeTextFile: true }, terminal: false } });
  log("INIT-RESP", { authMethods: init.authMethods ?? null, agentCapabilities: init.agentCapabilities });

  const spikeServer = path.join(process.cwd(), "mcp-echo-server.mjs");
  const mcpServers = [{ name: "spike", command: "/usr/bin/node", args: [spikeServer], env: [] }];
  let sessionId = loadSessionId;
  if (loadSessionId) {
    const loaded = await send(proc, "session/load", { sessionId: loadSessionId, cwd, mcpServers });
    log("LOAD-RESP", loaded);
  } else {
    const created = await send(proc, "session/new", {
      cwd,
      mcpServers,
      _meta: {
        claudeCode: {
          options: {
            settingSources: [],
            strictMcpConfig: true,
            disallowedTools: ["Task", "Bash", "Read", "Write", "Edit", "Glob", "Grep", "WebFetch", "WebSearch"],
            allowedTools: process.env.SPIKE_ALLOW ? process.env.SPIKE_ALLOW.split(",") : ["mcp__spike__spike_echo"],
          },
        },
      },
    });
    log("NEW-RESP", created);
    sessionId = created.sessionId;
  }

  const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
  let midOutcome = null;
  const midText = "Note the word STEERED in your reply.";
  // Fire the second prompt while the first turn is still running: poll for
  // the first tool call (or 6s), then send. Records whether the first turn
  // was still open when the mid prompt went out.
  const midRunner = (async () => {
    const t0 = Date.now();
    let sawTool = false;
    while (Date.now() - t0 < 6000) {
      await sleep(250);
      if (updates.some((u) => u.sessionUpdate === "tool_call")) { sawTool = true; break; }
    }
    const firstDone = firstSettled;
    console.log(`PROBE-ACTION mid-turn session/prompt sent (sawTool=${sawTool} firstDone=${firstDone})`);
    midOutcome = await send(proc, "session/prompt", { sessionId, prompt: [{ type: "text", text: midText }] }).then(
      (r) => ({ ok: true, stopReason: r.stopReason }),
      (e) => ({ ok: false, error: String(e).slice(0, 300) }),
    );
    log("MIDTURN-RESP", midOutcome);
  })();
  let firstSettled = false;
  const first = send(proc, "session/prompt", {
    sessionId,
    prompt: [{ type: "text", text: "Write a four-line poem about a lighthouse. Then call the spike_echo tool once with text alpha. Then reply with exactly: SPIKE-DONE" }],
  }).then(
    (r) => { firstSettled = true; return { ok: true, r }; },
    (e) => { firstSettled = true; return { ok: false, error: String(e).slice(0, 300) }; },
  );
  const firstResult = await first;
  log("FIRST-RESP", { ok: firstResult.ok, stopReason: firstResult.r?.stopReason ?? firstResult.error });
  await midRunner;

  const kinds = {};
  for (const u of updates) kinds[u.sessionUpdate] = (kinds[u.sessionUpdate] || 0) + 1;
  log("UPDATE-KINDS", kinds);
  const toolNames = updates.filter((u) => u.sessionUpdate === "tool_call").map((u) => ({ id: u.toolCallId, title: u.title, kind: u.kind, raw: JSON.stringify(u).slice(0, 400) }));
  log("TOOL-CALLS", toolNames);

  proc.kill();
  await new Promise((r) => setTimeout(r, 1000));
  return { sessionId, cwd };
}

const mode = process.argv[2] ?? "run";
if (mode === "run") {
  const { sessionId } = await drive("run1");
  console.log(`SESSION ${sessionId}`);
} else if (mode === "load") {
  const sessionId = process.argv[3];
  if (!sessionId) throw new Error("usage: node probe.mjs load <sessionId>");
  await drive("load", { loadSessionId: sessionId });
}

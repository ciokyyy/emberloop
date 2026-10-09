// Minimal stdio MCP server: one tool, spike_echo. Stands in for the
// el MCP server so the probe proves MCP-only stripping.
import readline from "node:readline";

const rl = readline.createInterface({ input: process.stdin, crlfDelay: Infinity });
rl.on("line", (line) => {
  if (!line.trim()) return;
  let msg;
  try { msg = JSON.parse(line); } catch { return; }
  const respond = (result) =>
    process.stdout.write(JSON.stringify({ jsonrpc: "2.0", id: msg.id, result }) + "\n");
  switch (msg.method) {
    case "initialize":
      respond({ protocolVersion: "2024-11-05", capabilities: { tools: {} }, serverInfo: { name: "spike-echo", version: "0.0.0" } });
      break;
    case "notifications/initialized":
      break;
    case "tools/list":
      respond({ tools: [{ name: "spike_echo", description: "Echo back text", inputSchema: { type: "object", properties: { text: { type: "string" } } } }] });
      break;
    case "tools/call":
      respond({ content: [{ type: "text", text: "echo:" + (msg.params?.arguments?.text ?? "") }] });
      break;
    default:
      if (msg.id === undefined) break;
      process.stdout.write(JSON.stringify({ jsonrpc: "2.0", id: msg.id, error: { code: -32601, message: "unknown: " + msg.method } }) + "\n");
  }
});

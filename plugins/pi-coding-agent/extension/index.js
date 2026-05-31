import { randomBytes } from "node:crypto";
import { chmodSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { createServer } from "node:net";
import { homedir } from "node:os";
import { join } from "node:path";

const BRIDGE_DIR = "ero-pi-coding-agent-bridge";
const STATE_FILE = "sessions.json";
const REFRESH_INTERVAL_MS = 15_000;
const MAX_REQUEST_BYTES = 1024 * 1024;
const CONNECTION_TIMEOUT_MS = 30_000;

export default function eroPiCodingAgentBridge(pi) {
  const token = randomBytes(32).toString("hex");
  const bridgeDir = bridgeRuntimeDir();
  const statePath = join(bridgeDir, STATE_FILE);
  let activeSessionId;
  let currentSessionId;
  let socketPath;
  let refreshTimer;
  let lifecycleQueue = Promise.resolve();
  let stopped = false;

  const server = createServer((conn) => {
    let data = "";
    let bytesReceived = 0;
    let closed = false;
    let rejected = false;

    function respond(response) {
      if (closed || conn.destroyed || conn.writableEnded) return;
      closed = true;
      conn.end(JSON.stringify(response) + "\n");
    }

    conn.setEncoding("utf8");
    conn.setTimeout(CONNECTION_TIMEOUT_MS, () => {
      closed = true;
      conn.destroy(new Error("bridge connection timed out"));
    });
    conn.on("error", (error) => {
      closed = true;
      reportBridgeError(error);
    });
    conn.on("data", (chunk) => {
      if (rejected) return;
      bytesReceived += Buffer.byteLength(chunk, "utf8");
      if (bytesReceived > MAX_REQUEST_BYTES) {
        rejected = true;
        respond({ ok: false, error: "request body too large" });
        return;
      }
      data += chunk;
    });
    conn.on("end", () => {
      if (rejected || closed) return;

      let payload;
      try {
        payload = JSON.parse(data);
      } catch (_error) {
        respond({ ok: false, error: "invalid json" });
        return;
      }

      if (!currentSessionId || payload.session_id !== currentSessionId || payload.token !== token) {
        respond({ ok: false, error: "invalid bridge token or session" });
        return;
      }
      if (!payload.message?.trim()) {
        respond({ ok: false, error: "message is required" });
        return;
      }

      try {
        const delivery = Promise.resolve(pi.sendUserMessage(payload.message, { deliverAs: "followUp" }));
        respond({ ok: true });
        delivery.catch((error) => {
          const message = error instanceof Error ? error.message : String(error);
          pi.ui?.notify?.(`Ero bridge failed to deliver review: ${message}`, "error");
        });
      } catch (error) {
        respond({ ok: false, error: error instanceof Error ? error.message : String(error) });
      }
    });
  });

  server.on("error", (error) => {
    reportBridgeError(error);
  });

  async function ensureListening(sessionId) {
    const nextSocketPath = join(bridgeDir, `${sessionId}.sock`);
    if (socketPath === nextSocketPath && server.listening) return;

    await closeServer();
    if (socketPath && socketPath !== nextSocketPath) {
      rmSync(socketPath, { force: true });
    }
    rmSync(nextSocketPath, { force: true });
    socketPath = nextSocketPath;

    await new Promise((resolve, reject) => {
      function cleanup() {
        server.off("error", onError);
        server.off("listening", onListening);
      }
      function onError(error) {
        cleanup();
        reject(error);
      }
      function onListening() {
        cleanup();
        try {
          chmodSync(nextSocketPath, 0o600);
          resolve();
        } catch (error) {
          reject(error);
        }
      }

      server.once("error", onError);
      server.once("listening", onListening);
      try {
        server.listen(nextSocketPath);
      } catch (error) {
        cleanup();
        reject(error);
      }
    });
  }

  async function closeServer() {
    if (!server.listening) return;
    server.closeAllConnections?.();
    await new Promise((resolve) => {
      server.close((error) => {
        if (error) reportBridgeError(error);
        resolve();
      });
    });
  }

  async function register(ctx) {
    const sessionId = ctx.sessionManager.getSessionId();
    if (stopped || activeSessionId !== sessionId) return;
    await ensureListening(sessionId);
    const git = await readGitMetadata(pi, ctx.cwd);
    if (stopped || activeSessionId !== sessionId) return;
    currentSessionId = sessionId;
    upsertSession(statePath, bridgeSessionRecord(ctx, git, socketPath, token));
    ctx.ui.setStatus("ero-pi-coding-agent", `Ero bridge ${sessionId.slice(0, 8)}`);
  }

  async function runLifecycle(task) {
    const run = lifecycleQueue.then(task, task);
    lifecycleQueue = run.catch(() => {});
    return run.catch((error) => {
      reportBridgeError(error);
    });
  }

  async function refresh(ctx) {
    return runLifecycle(async () => {
      if (stopped) return;
      await register(ctx);
    });
  }

  function startPeriodicRefresh(ctx) {
    if (refreshTimer) clearInterval(refreshTimer);
    refreshTimer = setInterval(() => {
      void refresh(ctx);
    }, REFRESH_INTERVAL_MS);
    refreshTimer.unref?.();
  }

  async function cleanupSession(sessionId) {
    if (refreshTimer) clearInterval(refreshTimer);
    refreshTimer = undefined;
    removeSession(statePath, sessionId);
    await closeServer();
    if (socketPath) {
      rmSync(socketPath, { force: true });
      socketPath = undefined;
    }
    if (currentSessionId === sessionId) currentSessionId = undefined;
  }

  async function start(ctx) {
    return runLifecycle(async () => {
      const sessionId = ctx.sessionManager.getSessionId();
      if (activeSessionId && activeSessionId !== sessionId) {
        await cleanupSession(activeSessionId);
      }
      stopped = false;
      activeSessionId = sessionId;
      await register(ctx);
      startPeriodicRefresh(ctx);
    });
  }

  async function stop(ctx) {
    return runLifecycle(async () => {
      const sessionId = ctx.sessionManager.getSessionId();
      if (activeSessionId !== sessionId) {
        removeSession(statePath, sessionId);
        return;
      }
      stopped = true;
      if (refreshTimer) clearInterval(refreshTimer);
      refreshTimer = undefined;
      await cleanupSession(sessionId);
      activeSessionId = undefined;
      ctx.ui.setStatus("ero-pi-coding-agent", undefined);
    });
  }

  pi.on("session_start", async (_event, ctx) => {
    await start(ctx);
  });

  pi.on("before_agent_start", async (_event, ctx) => {
    await refresh(ctx);
  });

  pi.on("turn_start", async (_event, ctx) => {
    await refresh(ctx);
  });

  pi.on("tool_execution_end", async (_event, ctx) => {
    await refresh(ctx);
  });

  pi.on("session_shutdown", async (_event, ctx) => {
    await stop(ctx);
  });

  pi.registerCommand("ero-bridge", {
    description: "Show the active Ero/pi-coding-agent bridge session ID",
    handler: async (_args, ctx) => {
      await start(ctx);
      ctx.ui.notify(
        `Ero pi-coding-agent bridge is active for session ${ctx.sessionManager.getSessionId()} at ${statePath}`,
        "info",
      );
    },
  });
}

async function readGitMetadata(pi, cwd) {
  const [worktreeRoot, currentBranch, headSHA] = await Promise.all([
    gitOutput(pi, cwd, ["rev-parse", "--show-toplevel"]),
    gitOutput(pi, cwd, ["branch", "--show-current"]),
    gitOutput(pi, cwd, ["rev-parse", "HEAD"]),
  ]);
  return { worktreeRoot, currentBranch, headSHA };
}

async function gitOutput(pi, cwd, args) {
  try {
    const result = await pi.exec("git", ["-C", cwd, ...args], { timeout: 5000 });
    if (result.code !== 0) return "";
    return result.stdout.trim();
  } catch (error) {
    reportBridgeError(error);
    return "";
  }
}

function bridgeRuntimeDir() {
  let dir;
  if (process.env.XDG_RUNTIME_DIR) {
    dir = join(process.env.XDG_RUNTIME_DIR, BRIDGE_DIR);
  } else {
    const userCacheDir = process.platform === "darwin" ? join(homedir(), "Library", "Caches") : join(homedir(), ".cache");
    const base = process.env.XDG_CACHE_HOME || userCacheDir;
    dir = join(base, "ero", "runtime", BRIDGE_DIR);
  }
  mkdirSync(dir, { recursive: true, mode: 0o700 });
  chmodSync(dir, 0o700);
  return dir;
}

function readState(path) {
  try {
    return JSON.parse(readFileSync(path, "utf8"));
  } catch (error) {
    if (error?.code !== "ENOENT") {
      reportBridgeError(new Error(`read bridge registry failed: ${formatError(error)}`));
    }
    return { sessions: [] };
  }
}

function writeState(path, state) {
  writeFileSync(path, JSON.stringify(state, null, 2), { mode: 0o600 });
  chmodSync(path, 0o600);
}

export function bridgeSessionRecord(ctx, git, socketPath, token, now = () => new Date()) {
  return {
    session_id: ctx.sessionManager.getSessionId(),
    session_file: ctx.sessionManager.getSessionFile(),
    cwd: ctx.cwd,
    worktree_root: git.worktreeRoot,
    current_branch: git.currentBranch,
    head_sha: git.headSHA,
    socket_path: socketPath,
    token,
    updated_at: now().toISOString(),
  };
}

export function upsertSession(path, session) {
  const state = readState(path);
  const sessions = state.sessions.filter((item) => item.session_id !== session.session_id);
  sessions.push(session);
  writeState(path, { sessions });
}

export function removeSession(path, sessionId) {
  const state = readState(path);
  writeState(path, { sessions: state.sessions.filter((item) => item.session_id !== sessionId) });
}

function reportBridgeError(error) {
  console.warn(`Ero pi-coding-agent bridge: ${formatError(error)}`);
}

function formatError(error) {
  return error instanceof Error ? error.message : String(error);
}

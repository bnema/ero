import assert from "node:assert/strict";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";

import eroPiCodingAgentBridge, { bridgeSessionRecord, removeSession, upsertSession } from "./index.js";

test("refreshes bridge registry after git metadata changes while the session stays open", async () => {
  const runtimeDir = await mkdtemp(join(tmpdir(), "ero-pi-bridge-test-"));
  const previousRuntimeDir = process.env.XDG_RUNTIME_DIR;
  process.env.XDG_RUNTIME_DIR = runtimeDir;
  const handlers = new Map();
  const git = { root: "/repo", branch: "main", head: "aaa" };
  const pi = fakePi({ handlers, git });
  const ctx = fakeContext("session-1", "/repo");

  try {
    eroPiCodingAgentBridge(pi);
    await emit(handlers, "session_start", { reason: "startup" }, ctx);

    let session = await readOnlySession(runtimeDir);
    assert.equal(session.current_branch, "main");
    assert.equal(session.head_sha, "aaa");

    git.branch = "feature";
    git.head = "bbb";
    await emit(handlers, "tool_execution_end", { toolName: "bash" }, ctx);

    session = await readOnlySession(runtimeDir);
    assert.equal(session.current_branch, "feature");
    assert.equal(session.head_sha, "bbb");
  } finally {
    await emitIfRegistered(handlers, "session_shutdown", { reason: "quit" }, ctx);
    if (previousRuntimeDir === undefined) {
      delete process.env.XDG_RUNTIME_DIR;
    } else {
      process.env.XDG_RUNTIME_DIR = previousRuntimeDir;
    }
    await rm(runtimeDir, { recursive: true, force: true });
  }
});

test("upsertSession refreshes branch and head metadata for an existing Pi session", async () => {
  const dir = await mkdtemp(join(tmpdir(), "ero-pi-bridge-state-"));
  try {
    const statePath = join(dir, "sessions.json");
    const ctx = fakeContext("session-1", "/repo");

    upsertSession(
      statePath,
      bridgeSessionRecord(
        ctx,
        { worktreeRoot: "/repo", currentBranch: "main", headSHA: "aaa" },
        "/sock",
        "token",
        () => new Date("2026-01-01T00:00:00Z"),
      ),
    );
    upsertSession(
      statePath,
      bridgeSessionRecord(
        ctx,
        { worktreeRoot: "/repo", currentBranch: "feature", headSHA: "bbb" },
        "/sock",
        "token",
        () => new Date("2026-01-01T00:00:05Z"),
      ),
    );

    const state = await readState(statePath);
    assert.equal(state.sessions.length, 1);
    assert.equal(state.sessions[0].current_branch, "feature");
    assert.equal(state.sessions[0].head_sha, "bbb");
    assert.equal(state.sessions[0].updated_at, "2026-01-01T00:00:05.000Z");
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});

test("removeSession deletes only the requested Pi session", async () => {
  const dir = await mkdtemp(join(tmpdir(), "ero-pi-bridge-state-"));
  try {
    const statePath = join(dir, "sessions.json");

    upsertSession(statePath, { session_id: "keep", current_branch: "main" });
    upsertSession(statePath, { session_id: "drop", current_branch: "feature" });
    removeSession(statePath, "drop");

    const state = await readState(statePath);
    assert.deepEqual(
      state.sessions.map((session) => session.session_id),
      ["keep"],
    );
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});

function fakePi({ handlers, git }) {
  return {
    on(event, handler) {
      const eventHandlers = handlers.get(event) ?? [];
      eventHandlers.push(handler);
      handlers.set(event, eventHandlers);
    },
    async exec(_command, args) {
      const command = args.slice(-2).join(" ");
      if (command === "rev-parse --show-toplevel") return { code: 0, stdout: git.root + "\n" };
      if (command === "branch --show-current") return { code: 0, stdout: git.branch + "\n" };
      if (command === "rev-parse HEAD") return { code: 0, stdout: git.head + "\n" };
      return { code: 1, stdout: "" };
    },
    sendUserMessage() {},
    registerCommand() {},
    ui: { notify() {} },
  };
}

function fakeContext(sessionId, cwd) {
  return {
    cwd,
    sessionManager: {
      getSessionId: () => sessionId,
      getSessionFile: () => `/sessions/${sessionId}.json`,
    },
    ui: { setStatus() {}, notify() {} },
  };
}

async function emitIfRegistered(handlers, event, payload, ctx) {
  const eventHandlers = handlers.get(event) ?? [];
  for (const handler of eventHandlers) {
    await handler(payload, ctx);
  }
}

async function emit(handlers, event, payload, ctx) {
  const eventHandlers = handlers.get(event) ?? [];
  assert.notEqual(eventHandlers.length, 0, `expected ${event} handler`);
  for (const handler of eventHandlers) {
    await handler(payload, ctx);
  }
}

async function readOnlySession(runtimeDir) {
  const data = await readState(join(runtimeDir, "ero-pi-coding-agent-bridge", "sessions.json"));
  assert.equal(data.sessions.length, 1);
  return data.sessions[0];
}

async function readState(path) {
  return JSON.parse(await readFile(path, "utf8"));
}

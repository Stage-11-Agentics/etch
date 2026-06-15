// Etch — OpenCode capture plugin.
//
// This is Etch's first-class live ingestion path for OpenCode (the other is
// Claude Code's .claude/settings.json hooks). OpenCode has no declarative hook
// file, but it has a plugin event system; this plugin subscribes to message,
// tool, and session-lifecycle events and shells out to the `entire-agent-etch`
// binary with the SAME native stdin JSON contract the Claude Code hooks use
// (see docs/HOOK_CONTRACT.md). Etch itself is unchanged — this file is only the
// dispatch.
//
// Install: copy to `.opencode/plugins/etch.ts` in a repo (OpenCode auto-loads
// `.opencode/plugins/*.ts`), or run `entire-agent-etch install-opencode`.
//
// Capture must never break a session: if the binary is absent the plugin is a
// no-op, and every dispatch is best-effort (.nothrow(), wrapped in try/catch).

import type { Plugin } from "@opencode-ai/plugin"

export const EtchPlugin: Plugin = async ({ $ }) => {
  // Guard: no binary on PATH → no-op plugin, exactly like the Claude hook guard.
  const present = Boolean(Bun.which("entire-agent-etch"))
  if (!present) return {}

  // Sessions we've already emitted session_start for, so each maps to one ULID.
  const started = new Set<string>()

  async function dispatch(sub: string, payload: Record<string, unknown>): Promise<void> {
    try {
      // stdin redirect must be a Buffer — Bun treats `< ${string}` as a
      // filename, not as stdin content.
      const json = Buffer.from(JSON.stringify(payload))
      await $`entire-agent-etch ${sub} < ${json}`.quiet().nothrow()
    } catch {
      // Capture is best-effort; a failed dispatch must never surface to the user.
    }
  }

  async function ensureStart(
    sessionID: string,
    model?: { providerID: string; modelID: string },
  ): Promise<void> {
    if (started.has(sessionID)) return
    started.add(sessionID)
    const modelName = model ? `${model.providerID}/${model.modelID}` : undefined
    // raw_data.agent_name sets agent.runtime; raw_data.model sets the model
    // (Entire-dialect fields etch reads from the payload).
    await dispatch("session_start", {
      session_id: sessionID,
      raw_data: { agent_name: "opencode", model: modelName },
    })
  }

  // OpenCode tool args name the edited file `filePath` (or `path`); etch reads
  // `file_path` (Claude's shape) for files_touched. Normalize so the adapter,
  // not etch, owns OpenCode's arg shape.
  function withFilePath(args: unknown): Record<string, unknown> {
    const a = (args ?? {}) as Record<string, unknown>
    const fp = a["file_path"] ?? a["filePath"] ?? a["path"]
    return typeof fp === "string" && fp ? { ...a, file_path: fp } : a
  }

  async function finalize(sessionID: string, reason: string): Promise<void> {
    if (!started.has(sessionID)) return
    started.delete(sessionID)
    await dispatch("session_end", { session_id: sessionID, reason })
  }

  return {
    // A new user message: ensure the session is open, then record the prompt.
    "chat.message": async (input, output) => {
      await ensureStart(input.sessionID, input.model)
      // parts is a discriminated union; narrow to text parts before reading .text.
      const text = (output.parts ?? [])
        .map((p) => (p.type === "text" ? p.text : ""))
        .filter((t) => t)
        .join("\n")
      if (text) {
        await dispatch("user_prompt_submit", { session_id: input.sessionID, prompt: text })
      }
    },

    "tool.execute.before": async (input, output) => {
      await ensureStart(input.sessionID)
      await dispatch("pre_tool_use", {
        session_id: input.sessionID,
        tool_name: input.tool,
        tool_use_id: input.callID,
        tool_input: withFilePath(output.args),
      })
    },

    "tool.execute.after": async (input) => {
      await dispatch("post_tool_use", {
        session_id: input.sessionID,
        tool_name: input.tool,
        tool_use_id: input.callID,
        tool_input: withFilePath(input.args),
      })
    },

    // session.idle fires every turn, so it is NOT a finalizer (that would
    // truncate multi-turn sessions). A session is finalized when it is deleted;
    // otherwise dispose() (below) finalizes still-open sessions on shutdown, and
    // anything missed is picked up by etch's .wip crash recovery.
    event: async ({ event }) => {
      if (event.type === "session.deleted") {
        const id = event.properties?.info?.id
        if (id) await finalize(id, "other")
      }
    },

    // OpenCode is tearing down: finalize every session still open.
    dispose: async () => {
      for (const id of [...started]) {
        await finalize(id, "other")
      }
    },
  }
}

export default EtchPlugin

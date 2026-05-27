# Cairn: The Complete Plan

## What Cairn Is

Cairn captures flat metadata about every AI agent session in a repository and stores it as immutable git refs that travel with push/fetch. Built on top of Entire CLI's hook infrastructure. Designed for 60-80+ concurrent agents across multiple machines.

## Architecture Overview

```mermaid
graph TB
    subgraph "Agent Runtimes"
        CC[Claude Code]
        CX[Codex]
        GC[Gemini CLI]
        OC[OpenCode]
        CU[Cursor]
        CP[Copilot CLI]
    end

    subgraph "Entire CLI (Hook Substrate)"
        EH[Agent Hooks<br/>SessionStart / Stop / ToolUse / etc.]
        EP[External Agent Plugin Protocol<br/>stdin/stdout JSON]
        EC[Entire Checkpoints<br/>entire/checkpoints/v1]
    end

    subgraph "Cairn (Metadata Layer)"
        CA[entire-agent-cairn<br/>Go binary on $PATH]
        BUF[Session Buffer<br/>.cairn/sessions/uuid.wip.jsonl]
        RW[Ref Writer<br/>git hash-object → mktree → commit-tree → update-ref]
    end

    subgraph "Git Storage"
        SR[refs/cairn/sessions/ULID<br/>immutable, one per session]
        OR[refs/cairn/observations/ULID<br/>late-arriving outcome data]
        AR[refs/cairn/archive/YYYY-Q<br/>compacted old sessions]
    end

    subgraph "Git Transport"
        REM[Remote: GitHub / Forgejo<br/>push/fetch via refspec]
    end

    CC & CX & GC & OC & CU & CP --> EH
    EH --> EP
    EH --> EC
    EP --> CA
    CA --> BUF
    BUF --> RW
    RW --> SR
    RW --> OR
    SR & OR & AR --> REM

    style CA fill:#F5C518,color:#000
    style SR fill:#2d5016,color:#fff
    style OR fill:#2d5016,color:#fff
```

## Data Flow: Session Lifecycle

```mermaid
sequenceDiagram
    participant Agent as Agent Runtime
    participant Entire as Entire CLI
    participant Plugin as entire-agent-cairn
    participant Buffer as .wip Buffer
    participant Git as Git Refs

    Agent->>Entire: SessionStart hook
    Entire->>Plugin: session_start (JSON stdin)
    Plugin->>Plugin: Read CAIRN_* env vars
    Plugin->>Plugin: Read git state (branch, HEAD, worktree)
    Plugin->>Plugin: Read machine identity (hostname hash)
    Plugin->>Buffer: Create uuid.wip.jsonl

    loop Each turn
        Agent->>Entire: UserPromptSubmit
        Entire->>Plugin: user_prompt_submit
        Plugin->>Buffer: Append prompt data

        Agent->>Entire: PreToolUse / PostToolUse
        Entire->>Plugin: pre_tool_use / post_tool_use
        Plugin->>Buffer: Append tool events
    end

    alt Normal exit
        Agent->>Entire: Stop / SessionEnd
        Entire->>Plugin: session_end
        Plugin->>Plugin: Read git state (end)
        Plugin->>Plugin: Finalize session.json
        Plugin->>Git: Commit to refs/cairn/sessions/ULID
        Plugin->>Buffer: Delete .wip file
    else Crash / Kill
        Note over Plugin,Buffer: Process dies. .wip file persists.
        Plugin->>Buffer: Next Cairn invocation finds orphaned .wip
        Plugin->>Plugin: Build partial record (status: incomplete)
        Plugin->>Git: Commit partial to refs/cairn/sessions/ULID
        Plugin->>Buffer: Delete .wip file
    end
```

## Git Object Model

```mermaid
graph LR
    REF["refs/cairn/sessions/<br/>01JWB8K3XQ..."]
    COMMIT["Commit (orphan)<br/>author: cairn<br/>msg: cairn session 01JWB8K3XQ...<br/>agent: claude-code / opus-4-7<br/>status: complete"]
    TREE["Tree"]
    BLOB1["session.json<br/>(~2-3KB metadata record)"]
    BLOB2["agent-trace.json<br/>(Agent Trace RFC format)"]

    REF -->|points to| COMMIT
    COMMIT -->|tree| TREE
    TREE -->|blob| BLOB1
    TREE -->|blob| BLOB2

    style REF fill:#F5C518,color:#000
    style COMMIT fill:#444,color:#fff
    style BLOB1 fill:#2d5016,color:#fff
    style BLOB2 fill:#1a3a5c,color:#fff
```

Each session ref is an **orphan commit** — no parent, no DAG, no merge conflicts, no contention. 60 concurrent agents each write to a different ref name. Zero possibility of collision.

## Session Record Shape

```mermaid
graph TD
    S["session.json"]

    S --> ID["Identity<br/>schema_version, session_id,<br/>parent_session_id, status, exit_reason"]
    S --> AG["Agent<br/>runtime, model, version"]
    S --> PR["Prompt<br/>text, source, truncated"]
    S --> OR["Orchestration<br/>type, dispatch_method, ticket_id,<br/>run_id, role, workflow_version, extra{}"]
    S --> TI["Timing<br/>started_at, ended_at, duration_ms"]
    S --> MA["Machine<br/>hostname_hash, os, arch"]
    S --> OP["Operator<br/>git_user, os_user"]
    S --> GS["Git State<br/>start{branch, HEAD, worktree}<br/>end{branch, HEAD, commits[]}"]
    S --> OC["Outcome (observed)<br/>commits[], pr_number, pr_state, ci_status"]
    S --> FT["Files Touched<br/>[{path, action}]"]
    S --> TK["Tokens<br/>input, output, cache_read,<br/>cache_write, api_calls, cost_usd"]
    S --> TU["Tool Use<br/>total_calls, by_tool{}"]
    S --> TR["Transcript Ref<br/>entire_checkpoint_id, local_path"]
    S --> C1["c11 Context<br/>workspace_id, surface_id,<br/>tab_title, pane_lineage"]

    style S fill:#F5C518,color:#000
    style OR fill:#1a3a5c,color:#fff
    style C1 fill:#5c1a3a,color:#fff
```

## Orchestration Declaration

```mermaid
graph LR
    subgraph "Orchestrator sets env vars"
        L[Lattice Orchestrator]
        M[Manual Session]
        C[Custom Orchestrator]
    end

    subgraph "CAIRN_* Environment"
        E1[CAIRN_ORCHESTRATOR_TYPE]
        E2[CAIRN_DISPATCH_METHOD]
        E3[CAIRN_TICKET_ID]
        E4[CAIRN_RUN_ID]
        E5[CAIRN_PARENT_SESSION_ID]
        E6[CAIRN_AGENT_ROLE]
        E7[CAIRN_WORKFLOW_VERSION]
        E8[CAIRN_ORCHESTRATION_EXTRA]
    end

    subgraph "Cairn reads at session start"
        CA[entire-agent-cairn]
        OB[orchestration{} block<br/>in session.json]
    end

    L -->|exports all| E1 & E2 & E3 & E4 & E5 & E6 & E7 & E8
    M -->|exports nothing| CA
    C -->|exports subset| E1 & E3

    E1 & E2 & E3 & E4 & E5 & E6 & E7 & E8 --> CA
    CA -->|populates| OB

    style L fill:#F5C518,color:#000
    style CA fill:#F5C518,color:#000
```

When all env vars are absent → `orchestration.type = "manual"`. No inference, no analysis. Pure capture.

## Cross-Machine Sync

```mermaid
graph LR
    subgraph "Hyperion (MacBook)"
        H1[Agent Session A]
        H2[Agent Session B]
        H3[Agent Session C]
        HR[refs/cairn/sessions/*<br/>local refs]
    end

    subgraph "Remote (GitHub / Forgejo)"
        RR[refs/cairn/sessions/*<br/>shared refs]
    end

    subgraph "Atlas (Mac Studio)"
        A1[Agent Session D]
        A2[Agent Session E]
        AR2[refs/cairn/sessions/*<br/>local refs]
    end

    H1 & H2 & H3 --> HR
    HR -->|git push| RR
    RR -->|git fetch| AR2
    A1 & A2 --> AR2
    AR2 -->|git push| RR
    RR -->|git fetch| HR

    style RR fill:#F5C518,color:#000
```

Sessions captured on Hyperion are visible on Atlas after the next fetch. No external sync, no database, no cloud service.

## Concurrency Model

```mermaid
graph TD
    subgraph "20 Concurrent Agents in One Repo"
        A1[Session α<br/>worktree: feat/login]
        A2[Session β<br/>worktree: feat/cache]
        A3[Session γ<br/>worktree: feat/auth]
        AN[Session ω<br/>worktree: feat/dashboard]
    end

    subgraph "Cairn Refs (zero contention)"
        R1[refs/cairn/sessions/01JW...α]
        R2[refs/cairn/sessions/01JW...β]
        R3[refs/cairn/sessions/01JW...γ]
        RN[refs/cairn/sessions/01JW...ω]
    end

    A1 -->|writes only to| R1
    A2 -->|writes only to| R2
    A3 -->|writes only to| R3
    AN -->|writes only to| RN

    subgraph "Entire Checkpoints (has races)"
        EC[entire/checkpoints/v1<br/>single shared branch<br/>⚠ SetReference with no CAS]
    end

    A1 & A2 & A3 & AN -.->|Entire writes here too<br/>but Cairn doesn't depend on it| EC

    style R1 fill:#2d5016,color:#fff
    style R2 fill:#2d5016,color:#fff
    style R3 fill:#2d5016,color:#fff
    style RN fill:#2d5016,color:#fff
    style EC fill:#8b0000,color:#fff
```

## Ref Lifecycle

```mermaid
graph LR
    ACTIVE["Active refs<br/>refs/cairn/sessions/*<br/>(last 90 days)"]
    INDEX["cairn index<br/>(periodic)"]
    ARCHIVE["Archive refs<br/>refs/cairn/archive/2026-Q2<br/>(compacted)"]
    DELETE["Old session refs<br/>deleted after archival"]

    ACTIVE -->|indexed| INDEX
    ACTIVE -->|after 90 days| ARCHIVE
    ARCHIVE -->|triggers| DELETE

    style ACTIVE fill:#2d5016,color:#fff
    style ARCHIVE fill:#444,color:#fff
```

At 60 sessions/day → ~22,000 refs/year. After indexing, refs older than 90 days are compacted into quarterly archive commits. New clones fetch only recent refs by default.

## Implementation Phases

```mermaid
gantt
    title Cairn Implementation
    dateFormat YYYY-MM-DD
    axisFormat %b %d

    section Phase 0 (done)
    Remote ref validation       :done, p0a, 2026-05-26, 1d
    Entire plugin protocol map  :done, p0b, 2026-05-26, 1d
    Python PoC binary           :done, p0c, 2026-05-26, 1d

    section Phase 1: Capture
    Go binary: entire-agent-cairn  :active, p1a, 2026-05-27, 5d
    Session buffer + crash recovery :p1b, after p1a, 3d
    Ref writer (git plumbing)       :p1c, after p1a, 3d
    Density test (20 agents)        :p1d, after p1b, 2d
    Cross-machine test (Hyperion ↔ Atlas) :p1e, after p1d, 1d

    section Phase 2: Query
    cairn query CLI              :p2a, after p1e, 3d
    cairn index                  :p2b, after p2a, 3d

    section Phase 3: Emit + Integrate
    Agent Trace emission         :p3a, after p2b, 2d
    Lattice skill update (CAIRN_* vars) :p3b, after p1e, 1d
```

## Testing Strategy

```mermaid
graph TD
    subgraph "Phase 1: Test on Cairn's Own Repo"
        T1["Install entire-agent-cairn on Hyperion"]
        T2["Enable on Forge++ repo"]
        T3["Run normal work sessions<br/>(this project generates its own test data)"]
        T4["Verify: session refs created?<br/>Fields populated? Schema valid?"]
        T5["Intentional crash test<br/>(kill agent mid-session)"]
        T6["Verify: .wip recovery works?<br/>Partial record committed?"]
    end

    subgraph "Density Test"
        D1["Pick a real Lattice orchestration run<br/>(10-20 tickets)"]
        D2["Run with Cairn enabled"]
        D3["Verify: all sessions captured?<br/>No ref collisions? No data loss?"]
        D4["Push to Forgejo, fetch on Atlas<br/>All refs arrive intact?"]
    end

    subgraph "Iterate"
        I1["Inspect real session records"]
        I2["Find schema gaps / wrong defaults"]
        I3["Fix, re-test, repeat"]
    end

    T1 --> T2 --> T3 --> T4
    T4 --> T5 --> T6
    T6 --> D1 --> D2 --> D3 --> D4
    D4 --> I1 --> I2 --> I3
    I3 -->|next round| T3

    style T3 fill:#F5C518,color:#000
    style D2 fill:#F5C518,color:#000
    style I1 fill:#2d5016,color:#fff
```

**Key insight: Cairn tests itself.** The work we're doing right now on this project (Forge++/Cairn design sessions) generates real agent sessions. Once `entire-agent-cairn` is installed, every session in this repo becomes test data. The first test is: does the binary we just built capture the session where we built it?

After solo sessions work, the density test is a real Lattice orchestration run — not synthetic data, not a contrived scenario, but actual multi-agent work producing PRs. If Cairn survives a real 10-20 ticket dispatch, it works.

## What We're Not Building

```mermaid
graph TD
    CAIRN["Cairn<br/>(capture + store)"]

    NO1["❌ Analysis engine"]
    NO2["❌ Hierarchical workflow model"]
    NO3["❌ Outcome binding / correlation"]
    NO4["❌ Dashboard"]
    NO5["❌ Hook infrastructure<br/>(Entire does this)"]

    CAIRN -.->|not building| NO1 & NO2 & NO3 & NO4 & NO5

    YES1["✓ Flat metadata per session"]
    YES2["✓ Immutable git refs"]
    YES3["✓ Concurrent-safe at 60+ agents"]
    YES4["✓ Cross-machine via push/fetch"]
    YES5["✓ Agent Trace interop"]

    CAIRN -->|building| YES1 & YES2 & YES3 & YES4 & YES5

    style CAIRN fill:#F5C518,color:#000
    style NO1 fill:#8b0000,color:#fff
    style NO2 fill:#8b0000,color:#fff
    style NO3 fill:#8b0000,color:#fff
    style NO4 fill:#8b0000,color:#fff
    style NO5 fill:#8b0000,color:#fff
    style YES1 fill:#2d5016,color:#fff
    style YES2 fill:#2d5016,color:#fff
    style YES3 fill:#2d5016,color:#fff
    style YES4 fill:#2d5016,color:#fff
    style YES5 fill:#2d5016,color:#fff
```

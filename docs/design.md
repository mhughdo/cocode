# cocode Design System v2

This document defines the visual and interaction system for cocode, a local-first desktop multi-agent code review cockpit. The direction is intentionally closer to the Codex Desktop App than to a typical SaaS dashboard: quiet, compact, precise, pale, and work-focused.

## 1. Product UX decision

cocode should use a centralized chat command center, but it should not become a chat-only app.

The review thread is the user's command surface. The user can describe what to review, ask cocode to build a review plan, monitor progress, and ask broad follow-ups in one place. However, cocode's primary objects are still structured review artifacts: Review Session, Agent Run, Finding, Evidence Bundle, Evidence Map, Follow-up Thread, Copy Packet, and GitHub Preview.

Use this mental model:

```text
Chat starts and explains the workflow.
The review plan gates execution.
Findings are the triage unit.
Evidence Map is the trust surface.
Finding-scoped follow-up answers uncertainty.
Copy and Publish are explicit human actions.
```

Do not let chat obscure deterministic state. A user should always know what agents ran, what context they saw, which findings were accepted, and what will be copied or published.

## 2. Visual atmosphere

The interface should feel like a native macOS productivity tool: calm, editorial, compact, and precise. It should be minimal but not empty; dense review information is presented with tables, dividers, and right-side detail panes instead of large colorful cards.

Target qualities:

- Local-first desktop utility, not marketing SaaS.
- Warm off-white content canvas with pale blue-gray navigation.
- Small controls and restrained typography.
- Dark charcoal primary buttons.
- Thin structural borders instead of heavy shadows.
- Findings and evidence are more important than agent transcript noise.
- All destructive or external side effects remain human-gated.

## 3. Color tokens

Use a warm neutral palette with one serious dark primary action color and semantic status colors used sparingly.

```css
:root {
  --cocode-page: #eef0f3;
  --cocode-app: #fbfbfa;
  --cocode-sidebar: #eaf1fb;
  --cocode-sidebar-active: #dfe8f7;

  --cocode-surface: #ffffff;
  --cocode-surface-muted: #f6f6f4;
  --cocode-surface-subtle: #f1f1ef;

  --cocode-ink: #111214;
  --cocode-ink-secondary: #2e3033;
  --cocode-muted: #74746f;
  --cocode-muted-soft: #9b9b95;

  --cocode-border: #e5e5e1;
  --cocode-border-soft: #eeeeeb;

  --cocode-button: #111214;
  --cocode-button-hover: #2f3033;

  --cocode-green: #2d7d46;
  --cocode-green-bg: #edf7ef;
  --cocode-red: #b64232;
  --cocode-red-bg: #fff0ee;
  --cocode-amber: #a86600;
  --cocode-amber-bg: #fbf3dc;
  --cocode-blue: #2f6c9e;
  --cocode-blue-bg: #e8f3fc;
}
```

Rules:

- Never use bright blue or purple AI gradients.
- Never use pure black for large areas; use charcoal `#111214`.
- Primary buttons are dark charcoal, not blue or green.
- Semantic colors should appear mostly in badges, status dots, and inline severity pills.
- Backgrounds should stay quiet; use depth through layout, not color spectacle.

## 4. Typography

Use a native, compact, technical sans stack. Do not use Inter as the brand-defining face.

```css
font-family: -apple-system, BlinkMacSystemFont, "SF Pro Text", "SF Pro Display", "Geist", "Helvetica Neue", Arial, sans-serif;
font-family-mono: "SF Mono", "Geist Mono", Menlo, Consolas, monospace;
```

Type scale:

| Role | Size | Weight | Notes |
|---|---:|---:|---|
| Screen title | 17-20px | 650 | Tight tracking, single line when possible. |
| Large setup heading | 24px | 700 | Only for centered empty/new review state. |
| Section title | 15px | 650 | Used inside panels. |
| Body text | 13px | 400-520 | Default UI copy. |
| Table text | 12px | 400-600 | Dense review data. |
| Metadata/labels | 11px | 400-550 | Muted, often mono for file paths. |
| Code | 12px | 400 | SF Mono / Geist Mono, 1.65-1.8 line height. |

Rules:

- Avoid oversized hero typography. cocode is a working app.
- Use sentence case, not title case everywhere.
- Use monospace for paths, SHAs, line ranges, commands, and token estimates.
- Avoid long paragraphs in panels. Prefer one claim sentence plus evidence.

## 5. Layout system

### Desktop window

- App mockup frame: 1488 x 900 inside a 1600 x 1000 presentation canvas.
- Window radius: 16px.
- Border: `1px solid rgba(0,0,0,.08)`.
- Shadow: very soft and diffuse only for the full window. Avoid shadows inside the app.

### Sidebar

- Width: 248-260px.
- Background: pale blue-gray `#eaf1fb`.
- Active item background: `rgba(0,0,0,.055)` or `#dfe8f7`.
- Item height: 32-34px.
- Item radius: 8px.
- Icons: 15-16px, single stroke or small square glyph, no emoji.
- Keep user/account status pinned at the bottom.

### Topbar

- Height: 62px.
- Border bottom: `1px solid #eeeeeb`.
- Left: breadcrumbs and page title.
- Right: compact actions.
- Primary action should be the rightmost dark button.

### Main content

Use these screen layout archetypes:

1. **Command center**: centered content group, max width around 900px, optional setup rail.
2. **Plan/configure**: main planning area plus right settings rail.
3. **Running thread**: chat/progress column plus right session detail rail.
4. **Findings board**: table/list plus right preview panel.
5. **Finding detail**: code/evidence workspace plus fixed action rail.
6. **Evidence Map**: left hierarchy, center graph, right evidence/claim panel.
7. **Publish/copy**: accepted findings, preview, copy/export settings.
8. **Settings/adapters**: adapter list plus selected adapter configuration pane.

## 6. Spacing and radii

| Token | Value | Use |
|---|---:|---|
| Page content padding | 24px | Default screen padding. |
| Settings content padding | 28-32px | Spacious settings screens. |
| Panel gap | 14-18px | Between major panes. |
| Card padding | 18px | Most panels. |
| Input horizontal padding | 10-12px | Compact fields. |
| Button height | 28, 32, or 34px | Small, default, new-review. |
| Primary CTA height | 32px | Most app actions. |
| Composer height | 94-118px | Review/follow-up prompt input. |
| Major panel radius | 12px | Cards and panes. |
| Button radius | 7-8px | Compact desktop controls. |
| Input radius | 8px | Form controls. |
| Logo radius | 6px | App mark. |

Avoid pill buttons for primary actions. Pills are acceptable only for small status badges.

## 7. Components

### Primary button

- Background: `#111214`.
- Text: `#ffffff`.
- Height: 32px.
- Radius: 8px.
- Padding: 12-13px horizontal.
- Font: 12px, weight 540-600.
- Hover: `#2f3033`.
- Active: `transform: translateY(1px)` or `scale(.98)`.
- No glow, no bright accent fill.

### Secondary button

- Background: `#ffffff`.
- Border: `1px solid #e5e5e1`.
- Text: `#252629`.
- Same sizing as primary.

### Inputs

- Height: 36px.
- Border: `1px solid #e5e5e1`.
- Radius: 8px.
- Background: white.
- Label above input, 11px muted.
- Helper text below only when needed.

### Composer

The composer is the chat entry point, not a full-screen chatbot.

- Rounded rectangle, 18px radius.
- Height: 94-118px depending on screen.
- Placeholder at top left.
- Bottom row: plus action, permission state, optional shortcut, send button.
- Send button: 32px dark circular button.
- Keep the composer aligned to the review content width, not full window width.

### Status badges

- Height: 22px.
- Radius: 999px.
- Font: 11px, weight 550.
- Use semantic pastel backgrounds only.
- Severity labels: High, Medium, Low, Nit.
- Verification labels: Verified, Plausible, Needs human, Likely false positive.

### Tables and lists

Findings should be dense, scannable, and sortable.

- Use tables or list rows, not giant cards for every finding.
- Row height: 48-74px depending on density.
- Header row: 11px muted text.
- File paths: monospace, 11-12px.
- Decision status should be a small badge, not a big button.
- Selecting a row updates the right preview panel.

### Evidence cards

Evidence items should feel concrete and inspectable.

- Show evidence kind, summary, path, line range.
- Keep exact code snippets in mono blocks.
- Support, counter, missing, test, and search evidence use semantic badges.
- Do not hide counter-evidence; absence of counter-evidence should be explicit.

### Code blocks

- Background: `#fbfbf9`.
- Border: `1px solid #eeeeeb`.
- Radius: 10px.
- Font: 12px mono.
- Highlight risky code with a pale red chip, not a saturated background.
- Highlight safe/replaced code with pale green.

### Evidence Map graph

Evidence Map is finding-specific, not a generic architecture diagram.

- Layout: left code hierarchy, center graph, right claim/action panel, bottom call path strip.
- Center canvas: white with very faint grid.
- Nodes: 150-180px wide, 12px radius, 1px border.
- Supporting nodes: pale green border/background.
- Missing/counter nodes: pale red border/background.
- Edges: solid for observed relationships, dashed red for missing relationships.
- Always show a primary changed-code node when location exists.
- Show a clear unavailable state when graph quality is incomplete.

### Adapter settings rows

Adapter rows must be practical and action-oriented.

Each row contains:

- Adapter icon.
- Adapter name.
- Connection kind or command path.
- Health state.
- Configure, Connect, Enable, or Add action.

Selected adapter detail contains:

- Tabs: General, Command, Permissions, Health.
- Executable path.
- Working directory.
- Output mode.
- Timeout.
- Default model/reasoning.
- Env allowlist.
- Permission toggles.
- Test connection and Save actions.

Adapter health should be visible without opening a modal.

## 8. Screen specifications

### 8.1 New review command center

Purpose: create a review source without feeling like a heavy wizard.

Must include:

- Source cards: Pull request, Local changes, Branch compare, Custom range.
- Repository selector.
- Pull request or branch controls.
- Prompt composer for review focus.
- Suggested setup rail showing connected adapters, redaction, runtime, and read-only mode.
- Primary action: Continue to plan.

Avoid:

- Starting agents immediately from a freeform message.
- Huge welcome illustrations.
- A modal wizard that hides context.

### 8.2 Review plan / configure

Purpose: make orchestration explainable and gated.

Must include:

- Plan checklist created by cocode.
- Changed files summary with additions/deletions.
- Context policy toggles.
- Agent selection/status.
- Runtime/model defaults.
- Primary action: Start review.

Execution starts only after user confirmation.

### 8.3 Running review thread

Purpose: monitor live work without drowning in raw transcripts.

Must include:

- Progress phase and completion bar.
- Files scanned, candidates, canonical findings.
- Agent run statuses.
- Early findings list.
- Thread composer for high-level review questions.
- Pause/cancel controls.

Avoid showing full raw agent logs by default. Raw artifacts belong in debug/event log.

### 8.4 Findings board

Purpose: triage canonical findings.

Must include:

- Summary counters.
- Filters and search.
- Dense table/list of canonical findings.
- Right preview panel with evidence and action buttons.
- Actions: accept, dismiss, copy selected, publish accepted.

Findings are the primary product object.

### 8.5 Finding detail

Purpose: decide whether a single finding is real and actionable.

Must include:

- Title, severity, verification status.
- Changed code and related tests tabs.
- Evidence and counter-evidence.
- Agent consensus.
- Draft GitHub comment.
- Suggested fix.
- Actions: accept, dismiss, defer, copy fix packet, ask follow-up, open Evidence Map.

### 8.6 Evidence Map

Purpose: make verification visual.

Must include:

- Code hierarchy.
- Graph nodes/edges.
- Bottom call path strip.
- Right claim/checklist/action panel.
- Ask verifier action scoped to selected graph path.

Do not turn this into a whole-repo map. It is one finding's evidence graph.

### 8.7 Publish and copy

Purpose: external handoff after human triage.

Must include:

- Accepted findings selector.
- GitHub review preview.
- Anchor warnings.
- Copy packet format and target agent.
- Token estimate.
- Final checklist.
- Human-gated publish action.

Never auto-publish.

### 8.8 Settings / Adapters

Purpose: connect and verify local CLIs and protocol adapters.

Must include:

- Adapter list with health status.
- First-party adapters: Codex CLI, Claude Code, Gemini CLI, OpenCode CLI, Codex App Server, Gemini ACP, Custom CLI.
- Selected adapter detail pane.
- Health check controls.
- Permission toggles with review-mode defaults.
- Save and Test connection actions.

## 9. Interaction model

### Start review flow

```text
User enters source and focus in New review
-> cocode creates review plan
-> user confirms settings
-> agents run in read-only mode
-> cocode normalizes, dedupes, verifies, and builds Evidence Maps
-> user triages findings
-> user copies or publishes accepted findings
```

### Chat behavior

Chat exists at three scopes:

1. **Review thread chat** for broad review questions and progress explanation.
2. **Plan chat** for orchestration clarification before agents start.
3. **Finding follow-up chat** for questions scoped to one evidence bundle.

Finding-specific questions should default to the finding follow-up thread, not the global review thread. This keeps answers grounded and prevents context drift.

### Actions and side effects

- Starting agents requires review-plan confirmation.
- Copying a packet requires explicit click.
- Publishing GitHub comments requires explicit click.
- Writing files is denied in MVP review mode.
- Local-only paths are excluded from external-agent context.

## 10. Empty, loading, and error states

### Empty new review

Show source cards, a repo selector, and a composer. Do not show a blank dashboard.

### Adapter not connected

Show the row in Settings with clear cause, for example "Command not found" or "Auth required", plus a Connect/Test action.

### Agent failure

Preserve completed findings from other agents. Mark the failed agent in the right rail and show a debug artifact link.

### Evidence Map incomplete

Show the primary location node, evidence list, and a clear reason such as "Call path unavailable because no route reference was found." Do not invent graph nodes.

### GitHub anchor warning

Keep the preview visible, mark the comment as unanchored, and offer summary-only publishing.

## 11. Motion and feedback

Motion should be quiet.

- Row hover: background tint only.
- Button active: tiny scale/translate.
- Panel transitions: 160-220ms ease.
- Progress bars: smooth width transition.
- No cinematic page animations inside the desktop app.
- No parallax, blob motion, or neon glow.

## 12. Accessibility

- All buttons need visible focus rings.
- Keyboard navigation should work in sidebars, findings table, tabs, and settings forms.
- Hit targets should be at least 32px on desktop; key primary controls can be 34-36px.
- Color should never be the only signal for severity or status.
- Code snippets need selectable text in implementation.

## 13. Implementation notes for React/Electron

- Use CSS Grid for screen layouts.
- Keep the Electron renderer sandboxed.
- Keep clipboard/file actions behind the preload bridge.
- Prefer shadcn/Radix primitives, but restyle to this system: compact radius, dark primary buttons, pale sidebar, thin borders.
- Use React Query for server state and local state for selection, tabs, and composer drafts.
- Do not import a new graph library until the Evidence Map interaction requirements exceed the simple SVG/canvas version.
- Build adapter settings on the real backend health/config endpoints.

## 14. Anti-patterns

Never use:

- Bright blue, purple, neon, or AI-gradient primary actions.
- Oversized buttons.
- Oversized headings in app screens.
- Three equal SaaS cards for every feature.
- Raw transcript as the main review UI.
- Agent model comparison tables as the default configuration UI.
- A generic architecture diagram for Evidence Map.
- Auto-run agents from a chat message before plan confirmation.
- Auto-publish GitHub comments.
- Emojis in UI text or icons.
- Placeholder names like John Doe or Acme.
- Copywriting like "elevate", "seamless", "next-gen", or "unleash".

## 15. v2 mockup inventory

The v2 mockups are implementation-oriented static screens:

1. `1-thread-setup.png`
2. `2-centralized-chat.png`
3. `3-finding-list.png`
4. `4-finding-details.png`
5. `5-evidence-map.png`
6. `6-adapter-settings.png`

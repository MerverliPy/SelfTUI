package ui

import (
	"context"
	"os"
	"time"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"github.com/charmbracelet/log"

	"github.com/MerverliPy/SelfTUI/internal/agent"
	"github.com/MerverliPy/SelfTUI/internal/config"
	"github.com/MerverliPy/SelfTUI/internal/ollama"
	"github.com/MerverliPy/SelfTUI/internal/session"
)

// AgentView is the Agent tab (PLAN.md §7): a streaming transcript rendered
// with glamour markdown (syntax-highlighted code blocks), a multi-line input,
// model selector (m), graceful errors, and cancellation (esc). M3a probes
// read-only tool support and explicitly falls back to plain chat when a model
// rejects tools or returns no tool call.
type AgentView struct {
	client *ollama.Client

	// ctx is the parent context every operation this view starts (model-list
	// fetches, chat streams) derives from. NewWithContext binds it to the
	// process root; the compatibility constructors leave it at Background.
	ctx context.Context

	runner *agent.Runner
	styles Styles
	dark   bool

	// logger, when non-nil, is the shared debug logger (N5): every runner
	// rebuild (ApplyConfig) re-attaches it so the drawer keeps following
	// the loop across settings saves.
	logger *log.Logger

	// Model selector state (loaded from /api/tags at init; r refreshes).
	models       []ollama.Model
	modelsErr    string
	loading      bool
	defaultModel string // from config; used when no selection exists yet
	model        string // selected model for the next send

	// clientGen counts client replacements (ApplyConfig reload after a
	// host/token change). Model-list results stamp the generation they were
	// issued under; a completion whose generation differs from the current
	// one belongs to an obsolete host and is dropped (M-03).
	clientGen uint64

	// Conversation state: one committed turn per entry (message + the model it
	// belongs to + the assistant footer meta + the width-cached render block).
	turns     []turn
	turnStart time.Time // when the current turn started (elapsed footer)

	// Context budget (M7-C): systemPrompt is kept on the view so the meter and
	// the truncation-marker check budget against the same payload the runner
	// sends (system prompt + conversation). truncated turns on when a send
	// exceeds the input budget; the truncation marker then stays visible in
	// the transcript head until /clear.
	systemPrompt string
	truncated    bool

	// Measured tokens (N3): prompt_eval_count from the last completed turn's
	// final chunk, shown by the ctx meter until the user edits the draft
	// again (input no longer equals measuredDraft) or a new turn starts —
	// ApproxTokens stays authoritative for live drafting. measuredDraft is
	// the draft at measurement time (the sentinel the meter validates
	// against, so every edit path is covered without touching each one).
	measuredPromptTokens int
	measuredDraft        string

	// lastTokPerSec (N4): the measured tok/s of the last completed turn
	// (same turnTokPerSec source the footer uses), surfaced in the shell
	// status row. Zero until a turn's final chunk carried usable metrics; a
	// user stop records no rate (the footer omits it there too); cleared
	// wherever the conversation is replaced (/clear, import, resume).
	lastTokPerSec int

	streaming  bool   // generation in flight
	streamText string // in-flight assistant content (flushed at the repaint tick)

	// N2 — streaming repaint discipline. Token deltas land in pendingStream
	// and merge into streamText only on streamTickMsg, so a burst of deltas
	// costs one glamour render per tick (≤ streamTickInterval) instead of
	// three full renders per token. streamRender caches the rendered active
	// block (header + content, caret excluded) keyed by its exact inputs —
	// content, header, width, theme — so the frame's count pass and window
	// pass share one render and frames with no new flushed text cost zero
	// glamour work. Glamour's margin collapsing is not composable across
	// markdown section boundaries (a paragraph's top margin renders inline
	// after a code block), so sub-block section caching would not stay
	// byte-identical; the frozen-neighbor discipline stays at the committed-
	// turn level (the per-turn render cache), which deltas never touch.
	pendingStream string

	streamRender      string // cached render of the active streaming block
	streamRenderSrc   string // streamText the cache was rendered from
	streamRenderHead  string // assistant header included in the cache
	streamRenderW     int    // terminal width the cache was rendered at
	streamRenderDark  bool   // theme the cache was rendered under
	streamRenderValid bool   // cache holds a rendered block at all

	stopRequest bool          // esc asked to stop; treat stream end as a stop
	stopArmed   bool          // M7: first esc while running arms the interrupt (opencode-style)
	stopCancel  func()        // cancels the in-flight chat context
	chatCh      chan tea.Msg  // activity channel (PLAN §8), one stream owner
	chatDone    chan struct{} // closed by the producer when the turn's goroutine exits (M-06)
	// chatTerminal is the one-slot lossless handoff for the turn's terminal
	// event (agent.AgentDoneMsg). The producer parks the event here only when
	// a saturated activity channel drops its send under cancellation
	// (emitTerminal); waitChatCmd reads the slot strictly after the activity
	// channel has been drained and closed, so the retained completion can
	// never overtake queued deltas and is never delivered twice (cA).
	chatTerminal chan tea.Msg
	toolStatus   string                // latest agent-tool activity for the hint row
	confirmation *agent.ToolConfirmMsg // pending mutation approval; blocks input/tab jumps

	// plainChatReason is the sanitized plain-chat fallback reason of the
	// in-flight turn (F1): set when agent.FallbackMsg lands and cleared when
	// the turn commits. Its commit appends a persistent "no tool ran" note to
	// the assistant turn's content, so the caveat renders inline, survives
	// the committed-turn render cache (rebuildRenderCache re-renders from
	// msg.Content), and reaches the /export transcript and a later /resume
	// reload. Before F1 the only caveat was the transient statusline notice,
	// so a fallback turn could export a narrated tool claim with no marker.
	plainChatReason string

	// Chat parameters (from config until M4).
	temperature float64
	topP        float64
	numCtx      int

	// Workspace tool trust (Phase 4): toolsEnabled arms the jailed tool
	// surface for the next send (false = plain chat, the default); workspace
	// is the canonical root the status row advertises; host is the config
	// Ollama base URL, used to warn when tools could send workspace content
	// to a non-loopback host.
	toolsEnabled bool
	workspace    string
	host         string

	// V2e session undo journal. Owned here (not by the runner) because the
	// runner is rebuilt on every settings save (ApplyConfig) while the
	// journal must survive the whole session: every rebuilt runner re-attaches
	// the same journal via WithJournal. Created memory-only by default; main
	// wires the crash-artifact dir through App.WithUndoDir.
	undo *agent.UndoJournal

	// batchReview is the pending write_files review overlay (V2e). The runner
	// emitted a BatchReviewMsg instead of the generic raw-args confirm; while
	// this is set the Agent tab owns every key and the modal renders the
	// per-file diffs. Approve applies the whole batch, decline applies
	// nothing (y/enter vs n/esc, mirroring the single-file confirm).
	batchReview *agent.BatchReviewMsg

	// batchPage is the file the review overlay shows (V2e residual: review-
	// overlay density at 72×30 for a 16-op batch). The overlay pages one
	// file at a time so an early tall diff can never push later files of a
	// big batch below the fitContent cut; pgup/pgdn moves between pages
	// while the review is open (0-based, clamped in batchKey).
	batchPage int

	// undoConfirm / redoConfirm are the y/esc confirm guards for the /undo
	// and /redo commands (design §4.3: each guarded by the standard confirm;
	// result surfaces as a status-bar notice). Only one is set at a time.
	undoConfirm bool
	redoConfirm bool

	// Input + feedback.
	input   textarea.Model
	chatErr string
	notice  string

	// Chat-session persistence: when sessionDir is set, committed turns are
	// appended to a per-process transcript file under it by one ordered
	// background recorder (M-04) — never on the update loop. Errors disable
	// the log once and surface one notice; chat never blocks on the disk.
	sessionDir  string
	sessionHost string // recorded in the transcript header (best effort)
	recorder    *session.Recorder
	recorded    bool // a turn was accepted by the recorder (/export pre-empt)
	sessionErr  bool
	// sessionErrMsg retains the one surfaced recorder failure so later
	// surfaces (e.g. /export) can echo the real cause instead of giving
	// dead-end advice — recording is permanently off for the run once the
	// first failure lands (P1-5).
	sessionErrMsg string

	// Composer (M7-A): slash-command drafting. The menu is derived from the
	// live input value (typing "/cl" filters to clear), so there is no
	// separate buffer; slashIdx is the highlighted row and slashQuery the
	// last-seen filter (a change resets the highlight to the top).
	// helpOpen is the /help overlay; clearConfirm asks before wiping the
	// conversation (both are modals for the App's tab-jump guard).
	slashIdx     int
	slashQuery   string
	helpOpen     bool
	clearConfirm bool

	// N6 display toggles (session-scoped, default off — no leader key: the
	// palette and slash menu stay the discoverable paths). showThinking
	// renders per-turn reasoning blocks; showDetails renders tool-activity
	// blocks. Both gate display only: neither changes what the agent loop
	// sends to the model. Reasoning and tool lines are stored regardless of
	// the toggles so a mid-session flip reveals past turns too, while a
	// toggle-off frame stays byte-identical to the pre-N6 renderer.
	showThinking bool
	showDetails  bool

	// Per-turn extras of the in-flight turn (N6). thinkingText/pendingThinking
	// mirror the N2 token batching (deltas land in pendingThinking, merge on
	// the repaint tick, never touch streamText); streamTools collects one
	// sanitized row per tool start/result (a multi-line summary is stored as
	// one row per line so the count and the render agree). Both commit with
	// the turn so the transcript keeps them behind the toggles after the
	// turn ends.
	thinkingText    string
	pendingThinking string
	streamTools     []string

	// Attachment expansion (N6): when a send carries @-references, the file
	// reads run in a command off the update loop (slow/network filesystems
	// and FIFOs must never block a repaint or Esc); the landing message
	// starts the turn. expandPending disarms the composer while expansion is
	// in flight so a second send cannot race the first.
	expandPending bool

	// @-file picker (N6): a fresh "@" in the composer lists the jailed
	// workspace's files (fetched off the update loop, M-04 discipline);
	// picking inserts "@path " into the draft and the send path expands the
	// references through the jailed read_file. The filter is derived from
	// the draft tail after the last '@' (word-bounded), so the picker can
	// never desync from what the user sees; fileOpen is the armed flag (esc
	// or a word end closes it until the next "@").
	fileOpen    bool
	fileLoading bool
	fileList    []string
	fileIdx     int
	fileFilter  string // last-seen filter text (a change resets the highlight)

	// Transcript scroll: follow auto-tails the newest content while
	// streaming or after a new turn; u/d scroll away from the tail.
	scroll int
	follow bool

	// Model selector overlay. M7-C: filter-as-you-type with the config default
	// starred; the filter is a plain string (the overlay owns its keys), not a
	// textinput, so typing letters filters instead of moving a cursor.
	selectorOpen bool
	selIdx       int
	selFilter    string

	// Resume picker (V2a): overlays the saved per-process transcripts under
	// sessionDir. The list is loaded by a command (no transcript filesystem
	// work on the update loop, same M-04 discipline as recording) and the
	// chosen file is parsed by another command; importing into the live
	// conversation with a non-empty history asks first via resumeConfirm
	// (resumePick holds the chosen path while the dialog is up).
	// resumePending spans the parse window: sends are refused while a load
	// is in flight, so a message typed between pick and import can never be
	// silently destroyed by the import replacing the conversation.
	resumeOpen    bool
	resumeLoading bool
	resumeIdx     int
	resumeList    []session.Saved
	resumeConfirm bool
	resumePick    string
	resumePending bool

	// glamour renderer, rebuilt when width changes (wrap is width-bound).
	tr      *glamour.TermRenderer
	renderW int

	chatChOnce bool
	w, h       int
}

// turn is one committed conversation entry: the message sent to/from the
// model, the model the turn belongs to (assistant header chip), the turn's
// footer meta ("3.4s · stop"; empty for user turns), and the glamour-rendered
// block (header + content) cached per width so geometry-change re-renders
// never lose it. One struct keeps the four views of a turn in lockstep —
// there is no parallel slice to desync.
type turn struct {
	msg    ollama.ChatMessage
	model  string
	meta   string
	render string
	// N6 per-turn extras, committed from the in-flight turn. thinking is the
	// turn's raw reasoning text (as relayed by agent.ThinkingMsg); tools are
	// the sanitized tool-activity lines. Both are stored always and rendered
	// only behind the showThinking/showDetails toggles (default off →
	// byte-identical frames). wire, when set on a user turn, is the expanded
	// @-reference content the runner will send — the transcript still renders
	// the displayed draft (msg.Content).
	thinking string
	tools    []string
	wire     string
}

// NewAgentView builds the Agent tab using the current directory as its
// workspace, with a background parent context and workspace tools DISABLED
// (the default: no tool definitions reach the model until the user opts in
// via cfg.ToolsEnabled — production wiring through NewWithContext). It
// remains a compatibility constructor for tests and legacy callers.
func NewAgentView(client *ollama.Client, styles Styles, theme, defaultModel string, agentCfg config.AgentConfig) AgentView {
	return newAgentView(nil, client, styles, theme, defaultModel, "", "", agentCfg, false, "")
}

// NewAgentViewWithWorkspace builds the Agent tab with the configured project
// root and system prompt (background parent context; tools disabled — see
// NewAgentView).
func NewAgentViewWithWorkspace(client *ollama.Client, styles Styles, theme, defaultModel, workspaceRoot string, agentCfg config.AgentConfig) AgentView {
	return newAgentView(nil, client, styles, theme, defaultModel, workspaceRoot, agentCfg.SystemPrompt, agentCfg, false, "")
}

// newAgentView is the private constructor: it stores ctx as the parent every
// operation this view starts derives from, and arms the workspace tools
// exactly when the caller says so (the App passes cfg.ToolsEnabled). host is
// the config Ollama base URL, used for the remote-host warning; a compat
// caller that cannot prove the host leaves it empty, which never warns.
func newAgentView(ctx context.Context, client *ollama.Client, styles Styles, theme, defaultModel, workspaceRoot, systemPrompt string, agentCfg config.AgentConfig, toolsEnabled bool, host string) AgentView {
	ctx = normalizeCtx(ctx)
	root := workspaceRoot
	if root == "" {
		root, _ = os.Getwd()
	}
	ta := textarea.New()
	ta.Prompt = "❯ "
	ta.Placeholder = "/ for commands, or chat with the selected model…"
	ta.ShowLineNumbers = false // line numbers waste width on a phone
	ta.Focus()                 // the input is the Agent tab's primary surface
	undo, _ := agent.NewUndoJournal("")
	return AgentView{
		client:       client,
		ctx:          ctx,
		runner:       runnerFor(client, root, systemPrompt, agentCfg.MaxToolIterations, toolsEnabled).WithJournal(undo),
		undo:         undo,
		styles:       styles,
		dark:         theme != "light",
		loading:      true,
		defaultModel: defaultModel,
		temperature:  agentCfg.Temperature,
		topP:         agentCfg.TopP,
		numCtx:       agentCfg.NumCtx,
		systemPrompt: systemPrompt,
		toolsEnabled: toolsEnabled,
		workspace:    canonicalWorkspaceLabel(root),
		host:         host,
		follow:       true,
		input:        ta,
		selectorOpen: false,
		selIdx:       0,
		renderW:      -1,
	}
}

// runnerFor builds the agent runner for one tools state: plain chat (no
// policy) when disabled, the armed policy runner when enabled.
func runnerFor(client *ollama.Client, root, systemPrompt string, maxIterations int, toolsEnabled bool) *agent.Runner {
	if !toolsEnabled {
		return agent.NewRunner(client, root, systemPrompt, maxIterations)
	}
	return agent.NewRunnerWithPolicy(client, root, systemPrompt, maxIterations, &agent.ToolPolicy{})
}

// WithLogger attaches the shared debug logger (N5): the current runner is
// re-armed immediately, and ApplyConfig keeps it across runner rebuilds so
// the drawer follows the loop across settings saves. Nil clears the trace.
func (v AgentView) WithLogger(l *log.Logger) AgentView {
	v.logger = l
	if v.runner != nil {
		v.runner = v.runner.WithLogger(l)
	}
	return v
}

// --- messages -------------------------------------------------------------

type agentModelsLoadedMsg struct {
	gen    uint64 // client generation at issue; mismatched completions are dropped
	models []ollama.Model
}
type agentModelsErrMsg struct {
	gen uint64
	err string
}

// sessionAppendMsg reports one committed turn's recorder outcome. err is nil
// on success (which never round-trips — the ack command returns nil instead);
// the first non-nil error disables recording once (M-04).
type sessionAppendMsg struct{ err error }

// sessionExportMsg reports the /export flush outcome: the exact transcript
// path once every earlier enqueued turn is durable, or the failure that
// disabled recording.
type sessionExportMsg struct {
	path string
	err  error
}

// sessionListMsg reports the /resume picker listing: the saved transcripts
// under sessionDir (newest first), or the listing failure.
type sessionListMsg struct {
	list []session.Saved
	err  error
}

// sessionLoadedMsg reports the parsed turns of one chosen transcript, or the
// parse/load failure. Import happens on the update loop only after this
// arrives — the file read itself never blocks a frame.
type sessionLoadedMsg struct {
	path  string
	turns []session.Turn
	err   error
}

// agentEventMsg is the single envelope the root App accepts for every
// asynchronous Agent command result: the model-list fetch results
// (agentModelsLoadedMsg/agentModelsErrMsg), the recorder outcomes
// (sessionAppendMsg/sessionExportMsg), and every event the chat activity
// channel delivers (agent.TokenMsg, agent.ToolStartMsg, agent.ToolResultMsg,
// agent.ToolConfirmMsg, agent.ToolOutputMsg, agent.FallbackMsg,
// agent.AgentDoneMsg). App.Update has exactly one routing case
// per child and unwraps before delegating, so any payload that is produced is
// routed by construction — a newly added async result can no longer be
// dropped at the shell (the 2026-09-06 ToolConfirmMsg routing bug).
type agentEventMsg struct{ msg tea.Msg }

// agentThemeMsg asks the root App to switch the whole shell theme. The Agent
// view does not own the palette (settings do), so the slash command /theme
// emits this and App applies it (M7-A).
type agentThemeMsg struct{ theme string }

// streamTickMsg (N2) merges the token deltas accumulated since the last tick
// into streamText and re-primes the active-block render cache — batching
// per-token frames into a bounded repaint cadence. It travels wrapped in
// agentEventMsg: App.Update forwards only key presses and child envelopes to
// the Agent tab, so a bare tea.Msg from a command would be swallowed by the
// shell's default case and never reach this view.
type streamTickMsg struct{}

// fileListMsg reports the @-file picker's workspace listing (N6): the
// workspace-relative file paths, walked off the update loop (M-04 discipline;
// the composer frame never blocks on the filesystem). An unreadable
// workspace lists as empty — the picker degrades to "no files", never an
// error surface.
type fileListMsg struct{ files []string }

// attachExpandedMsg lands the deferred @-reference expansion (N6): the draft
// text as sent and its expanded wire content, resolved off the update loop
// by the send command. It triggers beginTurn, which commits the user turn
// and starts the chat.
type attachExpandedMsg struct {
	draft string
	wire  string
}

// streamTickInterval is the streaming repaint cadence. Ollama streams deltas
// every ~10–50 ms; 60 ms coalesces 1–6 deltas per repaint (≈17 fps), keeps
// the caret visually smooth (well under the ~100 ms perception threshold for
// live typing), and keeps the final chunk's immediate flush (onChatDone)
// imperceptibly late for the N3 footer metrics.
const streamTickInterval = 60 * time.Millisecond

// Init starts the model list fetch for the selector.
func (v AgentView) Init() tea.Cmd {
	return v.loadModelsCmd()
}

func (v AgentView) loadModelsCmd() tea.Cmd {
	gen := v.clientGen
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(v.ctx, 60*time.Second)
		defer cancel()
		models, err := v.client.List(ctx)
		if err != nil {
			return agentEventMsg{msg: agentModelsErrMsg{gen: gen, err: err.Error()}}
		}
		return agentEventMsg{msg: agentModelsLoadedMsg{gen: gen, models: models}}
	}
}

// emitEvent is the one context-aware producer delivery used by every
// background stream producer (the chat and pull goroutines), replacing
// unconditional channel sends (M-06). Contract:
//
//   - Delivery succeeds in order, or cancellation wins: while the run context
//     is alive a blocked send (a full 64-slot activity channel nobody is
//     draining) yields to cancellation instead of stranding the producer
//     forever.
//   - Once canceled, a send never blocks again: the event is delivered only
//     when a consumer is draining at that instant, and dropped otherwise — so
//     a producer whose consumer has gone away still terminates. Activity
//     deltas may be dropped here; the chat turn's single terminal event
//     (agent.AgentDoneMsg) never is — it goes through emitTerminal instead,
//     which retains a dropped completion for post-drain pickup (cA).
//   - The channel is owned by the producing goroutine, which closes it only
//     after its last send — nothing here can ever send on a closed channel.
func emitEvent(ctx context.Context, ch chan tea.Msg, msg tea.Msg) {
	select {
	case ch <- msg:
		return
	case <-ctx.Done():
	}
	// Cancellation won a blocked send (the channel was full at that instant).
	// The consumer may have drained concurrently — that is the one case where
	// delivery is still possible — so give the event a single nonblocking
	// chance; if nobody is draining, drop it and never block again.
	select {
	case ch <- msg:
	default:
	}
}

// emitTerminal delivers the chat turn's single terminal event
// (agent.AgentDoneMsg) under emitEvent's cancellation contract — a producer
// must never block after cancellation — but without its drop: when the
// saturated activity channel still refuses the event after cancellation, the
// event is parked in the one-slot terminal channel for the consumer to pick
// up once the activity channel has been drained and closed (waitChatCmd),
// so the completion that ends the turn can never be lost (cA). A terminal
// that reached the activity channel is never parked, so exactly one
// completion reaches the view per turn; the parking write precedes the
// producer's channel close, so a parked event is always observable by the
// time the drain sees the channel closed.
func emitTerminal(ctx context.Context, ch chan tea.Msg, terminal chan tea.Msg, msg tea.Msg) {
	select {
	case ch <- msg:
		return
	case <-ctx.Done():
	}
	select {
	case ch <- msg: // a consumer drained concurrently: normal delivery
		return
	default:
		// Full and nobody draining: retain the completion instead of losing
		// it. The slot is written only when the channel send failed, so it
		// can never double a delivered done; the producer emits one terminal
		// per turn, so the one-slot buffer cannot overflow.
		select {
		case terminal <- msg:
		default: // unreachable: one terminal event per turn
		}
	}
}

// startChat begins a streaming agent turn in a background goroutine. The
// activity channel carries agentEventMsg-wrapped tool events, token deltas,
// and one final agent.AgentDoneMsg (the shell unwraps before routing, so a
// chat event can never be dropped at the App again). turnStart anchors the
// per-turn elapsed footer (M7-B). chatDone is closed when the producer
// goroutine exits — the M-06 termination oracle for saturation tests (the
// channel close is not enough: reading it would drain the backlog and
// unblock a stuck producer).
func (v AgentView) startChat() (AgentView, tea.Cmd) {
	ch := make(chan tea.Msg, 64)
	done := make(chan struct{})
	// terminal is the one-slot handoff for the terminal event dropped on a
	// full activity channel under cancellation (cA). A fresh slot per turn
	// keeps the lifecycle self-contained: onChatDone clears it, and the next
	// startChat allocates a new one.
	terminal := make(chan tea.Msg, 1)
	ctx, cancel := context.WithCancel(v.ctx)

	v.chatCh = ch
	v.chatDone = done
	v.chatTerminal = terminal
	v.stopCancel = cancel
	v.streaming = true
	v.streamText = ""
	v.pendingStream = "" // N2: a fresh turn starts with an empty delta batch
	v.thinkingText = ""  // N6: fresh per-turn extras
	pendingThinking := ""
	_ = pendingThinking
	v.pendingThinking = ""
	v.streamTools = nil
	v.plainChatReason = "" // F1: a stale fallback must never mark a later turn
	v.resetStreamRender()
	v.stopRequest = false
	v.stopArmed = false
	v.chatErr = ""
	v.notice = ""
	v.follow = true
	v.turnStart = time.Now()
	v.measuredPromptTokens = 0 // new turn: the meter is approximate again

	model := v.model
	msgs := make([]ollama.ChatMessage, 0, len(v.turns))
	for _, t := range v.turns {
		// N6: a turn with @-references carries its expanded wire content —
		// exactly what the runner will send — instead of the displayed draft.
		content := t.msg.Content
		if t.msg.Role == ollama.RoleUser && t.wire != "" {
			content = t.wire
		}
		msgs = append(msgs, ollama.ChatMessage{Role: t.msg.Role, Content: content})
	}

	go func() {
		defer close(done)
		defer close(ch)
		defer cancel()
		v.runner.Run(ctx, agent.Request{
			Model: model, Messages: msgs,
			Temperature: v.temperature, TopP: v.topP, NumCtx: v.numCtx,
		}, func(msg agent.Msg) {
			// cA: activity deltas may drop on a saturated channel under
			// cancellation, but the turn's terminal event (AgentDoneMsg) must
			// never be lost — route it through the retaining path so onChatDone
			// always runs once the drain reaches it.
			if _, isDone := msg.(agent.AgentDoneMsg); isDone {
				emitTerminal(ctx, ch, terminal, agentEventMsg{msg: msg})
				return
			}
			emitEvent(ctx, ch, agentEventMsg{msg: msg})
		})
	}()

	// N2: batch the activity subscription with the first repaint tick — the
	// tick flushes deltas at streamTickInterval and re-arms itself while the
	// turn streams.
	return v, tea.Batch(v.waitChatCmd(), v.streamTickCmd())
}

// streamTickCmd re-arms the repaint tick (N2). The tick arrives wrapped in
// agentEventMsg so the App shell forwards it (see streamTickMsg).
func (v AgentView) streamTickCmd() tea.Cmd {
	return tea.Tick(streamTickInterval, func(time.Time) tea.Msg {
		return agentEventMsg{msg: streamTickMsg{}}
	})
}

// mergeStreamDeltas folds pendingStream into streamText (append-only; the
// merge is the only writer of streamText during a turn). Pure state merge:
// rendering is the caller's concern (the tick re-primes the cache; the done
// path commits). Also re-arms follow, matching the per-token behavior this
// batching replaces (M7-B: streaming re-tails the pane).
func (v *AgentView) mergeStreamDeltas() {
	if v.pendingStream == "" {
		return
	}
	v.streamText += v.pendingStream
	v.pendingStream = ""
	v.follow = true
}

// liveStreamText is the assistant text including deltas still awaiting the
// repaint tick (N2). Logic that must observe the complete stream — payload
// budgeting, transcript guards, the commit path — reads this; the render
// path reads flushed streamText so frames stay aligned with the cache.
func (v AgentView) liveStreamText() string {
	if v.pendingStream == "" {
		return v.streamText
	}
	return v.streamText + v.pendingStream
}

// resetStreamRender drops the active-block render cache (N2).
func (v *AgentView) resetStreamRender() {
	v.streamRender = ""
	v.streamRenderSrc = ""
	v.streamRenderHead = ""
	v.streamRenderW = 0
	v.streamRenderDark = false
	v.streamRenderValid = false
}

// primeStreamRender renders the active streaming block once per content,
// header, width, or theme change (N2). Called from the repaint tick and the
// geometry/theme paths so View reads a warm cache; a cold View still falls
// back to a fresh render (streamBlockRender), never to stale output.
func (v *AgentView) primeStreamRender() {
	if v.streamText == "" {
		if v.streamRenderValid {
			v.resetStreamRender()
		}
		return
	}
	header := v.assistantHeader(v.model)
	w := chatRenderWidth(v.w - 2)
	if v.streamRenderValid && v.streamRenderSrc == v.streamText &&
		v.streamRenderHead == header && v.streamRenderW == w && v.streamRenderDark == v.dark {
		return // unchanged inputs: the cache is still exact
	}
	v.streamRender = v.renderBlock(header, v.streamText)
	v.streamRenderSrc = v.streamText
	v.streamRenderHead = header
	v.streamRenderW = w
	v.streamRenderDark = v.dark
	v.streamRenderValid = true
}

// streamBlockRender returns the rendered streaming block (header + content,
// caret excluded) from the N2 cache when its key still matches, or a fresh
// renderBlock otherwise. Both paths are byte-identical by construction, so
// the N1 equivalence pins and golden discipline hold regardless of which one
// a frame hits.
func (v AgentView) streamBlockRender() string {
	header := v.assistantHeader(v.model)
	w := chatRenderWidth(v.w - 2)
	if v.streamRenderValid && v.streamRenderSrc == v.streamText &&
		v.streamRenderHead == header && v.streamRenderW == w && v.streamRenderDark == v.dark {
		return v.streamRender
	}
	return v.renderBlock(header, v.streamText)
}

// waitChatCmd is the resubscribed activity command (PLAN §8; same pattern as
// the Models pull): it blocks until the chat goroutine posts its next message.
// It is also the cA completion pickup: a closed activity channel yields its
// queued events first and only then reports ok=false (the producer closes it
// only after its last send), so when the turn's terminal event was dropped on
// the full channel and parked in the one-slot chatTerminal (emitTerminal) it
// surfaces here — strictly after every queued delta, exactly once. A terminal
// that reached the channel is never parked and onChatDone already ran, so an
// empty slot ends the subscription (nil message) without re-arming.
func (v AgentView) waitChatCmd() tea.Cmd {
	ch := v.chatCh
	if ch == nil {
		return nil
	}
	terminal := v.chatTerminal
	return func() tea.Msg {
		msg, ok := <-ch
		if ok {
			return msg
		}
		select {
		case done := <-terminal:
			return done
		default:
			return nil
		}
	}
}

// ModalOpen reports whether the Agent tab is showing a modal. The root App
// uses it so the 1/2/3 tab-jump keys cannot steal from an approval dialog, a
// confirmation, or the help overlay.
func (v AgentView) ModalOpen() bool {
	return v.selectorOpen || v.confirmation != nil || v.batchReview != nil ||
		v.helpOpen || v.clearConfirm || v.undoConfirm || v.redoConfirm ||
		v.resumeOpen || v.resumeConfirm
}

// --- update ---------------------------------------------------------------

func (v AgentView) Update(msg tea.Msg) (AgentView, tea.Cmd) {
	cmds := make([]tea.Cmd, 0, 3)
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case agentEventMsg:
		// The App shell normally unwraps the envelope before delegating; when
		// the view runs standalone (or a test drives its commands directly)
		// the wrapped result comes back as-is, so unwrap and re-dispatch to
		// the same switch.
		return v.Update(msg.msg)

	case attachExpandedMsg:
		// The deferred @-reference expansion has landed: commit the turn and
		// start the chat. The reads already happened on the command goroutine,
		// so this stays repaint-cheap.
		return v.beginTurn(msg.draft, msg.wire)

	case sessionAppendMsg:
		// One recorder error disables the transcript once and surfaces one
		// notice; later identical outcomes (jobs accepted before the view
		// learned of the failure) are ignored (M-04).
		if msg.err != nil && !v.sessionErr {
			v.sessionErr = true
			v.sessionErrMsg = msg.err.Error()
			v.notice = "session log: " + msg.err.Error()
		}
		return v, nil

	case sessionExportMsg:
		return v.applySessionExport(msg), nil

	case sessionListMsg:
		return v.applySessionList(msg), nil

	case sessionLoadedMsg:
		return v.applySessionLoaded(msg), nil

	case fileListMsg:
		// N6: the @-picker's workspace listing landed. The list is taken as
		// offered (the walk is jail-bounded by construction); a picked path is
		// re-verified through the jailed read at expansion time, so even a
		// hostile listing can only produce a visible "unavailable" note.
		v.fileLoading = false
		v.fileList = msg.files
		v.fileIdx = 0
		return v, nil

	case tea.WindowSizeMsg:
		v.w, v.h = msg.Width, msg.Height
		// Wrap width changed: recompose the textarea fit and rebuild the
		// renderer + caches at the new width. N2: re-prime the active-block
		// cache too, so the next frame cannot fall back to per-frame renders
		// until the next tick. N1 micro-item: the render width is bucketed
		// to 5 columns, so jitter inside one bucket leaves the renderer and
		// both caches warm; only a bucket change rebuilds (a nil renderer
		// always retries the build).
		v = v.fitComposer()
		if w := chatRenderWidth(v.w - 2); v.renderW != w || v.tr == nil {
			v.renderW = -1
			v.rebuildRenderer()
			v.rebuildRenderCache()
			v.primeStreamRender()
		}
		return v, nil

	case agentModelsLoadedMsg:
		// A model list from an obsolete client generation (the host/token
		// changed while the fetch was in flight) must not replace the current
		// host's models or the chat model chosen from them (M-03).
		if msg.gen != v.clientGen {
			return v, nil
		}
		v, cmd = v.onModelsLoaded(msg.models)
		cmds = append(cmds, cmd)

	case agentModelsErrMsg:
		if msg.gen != v.clientGen {
			return v, nil
		}
		v.loading = false
		v.modelsErr = sanitizeTerminalText(msg.err)
		return v, nil

	case agent.TokenMsg:
		// N2: deltas queue for the repaint tick; the frame itself re-renders
		// only when the tick flushes them (or the turn commits). follow stays
		// per-token exactly as before (M7-B: streaming re-tails the pane).
		if v.streaming {
			v.pendingStream += msg.Text
			v.follow = true
		}
		return v, v.waitChatCmd()

	case streamTickMsg:
		// N2 repaint tick: flush accumulated deltas, re-prime the active-block
		// cache once per content change, and re-arm while the turn streams.
		// N6: the same tick merges thinking deltas so a reasoning burst costs
		// one composed block per repaint, never one per delta.
		v.mergeStreamDeltas()
		v.mergeThinkingDeltas()
		v.primeStreamRender()
		if v.streaming {
			return v, v.streamTickCmd()
		}
		return v, nil

	case agent.ThinkingMsg:
		// N6: reasoning deltas queue for the repaint tick (same batching the
		// content stream uses). They are stored always — the /thinking toggle
		// gates rendering, not storage — and never enter streamText, so the
		// assistant's answer and the M7-B caret behavior are untouched.
		if v.streaming {
			v.pendingThinking += msg.Text
		}
		return v, v.waitChatCmd()

	case agent.ToolStartMsg:
		if v.streaming {
			// The tool name and its argument JSON come from the remote model's
			// tool call; sanitize before the statusline shows them (H-05).
			v.toolStatus = sanitizeTerminalText("⚙ " + msg.Name + " " + msg.Input)
			// N6: the /details block keeps one bounded row per tool event.
			v.streamTools = append(v.streamTools, toolDetailRows("⚙ "+msg.Name+" "+msg.Input)...)
		}
		return v, v.waitChatCmd()

	case agent.ToolResultMsg:
		if v.streaming {
			prefix := "✓ "
			if !msg.OK {
				prefix = "⚠ "
			}
			// Summary can carry bytes read from the workspace at a hostile
			// model's request; sanitize the composed status row as one value.
			v.toolStatus = sanitizeTerminalText(prefix + msg.Name + ": " + firstLine(msg.Summary))
			v.streamTools = append(v.streamTools, toolDetailRows(prefix+msg.Name+": "+msg.Summary)...)
		}
		return v, v.waitChatCmd()

	case agent.ToolOutputMsg:
		if v.streaming {
			// Command output is remote/model-requested process output; only a
			// sanitized first line reaches the statusline. The complete bounded
			// result is sent back to the model through ToolResultMsg.
			v.toolStatus = sanitizeTerminalText("… " + msg.Name + " " + msg.Stream + ": " + firstLine(msg.Text))
		}
		return v, v.waitChatCmd()

	case agent.ToolConfirmMsg:
		// The confirmation overlay echoes the remote tool name and input;
		// sanitize this display copy (the runner keeps its own raw copy).
		msg.Name = sanitizeTerminalText(msg.Name)
		msg.Input = sanitizeTerminalText(msg.Input)
		v.confirmation = &msg
		return v, v.waitChatCmd()

	case agent.BatchReviewMsg:
		// V2e: the write_files review stage. The diffs and summaries echo
		// workspace content and model text; sanitize the display copy like the
		// raw-args confirm above (the runner keeps the authoritative copy).
		msg.Name = sanitizeTerminalText(msg.Name)
		msg.Workspace = sanitizeTerminalText(msg.Workspace)
		msg.Note = sanitizeTerminalText(msg.Note)
		for i := range msg.Files {
			f := &msg.Files[i]
			f.Path = sanitizeTerminalText(f.Path)
			f.Kind = sanitizeTerminalText(f.Kind)
			f.Summary = sanitizeTerminalText(f.Summary)
			for j := range f.Rows {
				f.Rows[j] = sanitizeTerminalText(f.Rows[j])
			}
		}
		v.batchReview = &msg
		v.batchPage = 0 // V2e residual: a fresh review opens on file 1 of N
		return v, v.waitChatCmd()

	case undoDoneMsg:
		// A /undo or /redo finished (host-side journal op, ran in a command
		// off the update loop). Surface the result as a status-bar notice.
		v.applyUndoDone(msg)
		return v, nil

	case agent.FallbackMsg:
		v.notice = sanitizeTerminalText(msg.Reason)
		// F1: remember the fallback for the in-flight turn so its commit
		// carries the persistent caveat (render + transcript + resume), not
		// just this transient statusline notice.
		v.plainChatReason = sanitizeTerminalText(msg.Reason)
		return v, v.waitChatCmd()

	case agent.AgentDoneMsg:
		return v.onChatDone(msg)

	case tea.KeyMsg:
		v, cmd = v.handleKey(msg)
		cmds = append(cmds, cmd)
	}
	if len(cmds) == 0 {
		return v, nil
	}
	return v, tea.Batch(cmds...)
}

// onModelsLoaded picks the model to chat with: the config default when it is
// installed, otherwise the first entry (PLAN §5: "first /api/tags entry at
// runtime when empty"). An existing selection survives a refresh.
func (v AgentView) onModelsLoaded(models []ollama.Model) (AgentView, tea.Cmd) {
	// Model names come from the remote /api/tags host; sanitize them as they
	// are stored so headers, the picker, notices, and the chat request all
	// carry one clean representation (H-05).
	models = sanitizeModelNames(models)
	v.models = models
	v.loading = false
	v.modelsErr = ""
	if len(models) == 0 {
		v.model = ""
		v.selIdx = 0
		return v, nil
	}

	keep := v.model
	v.model = ""
	for _, m := range models {
		if m.Name == keep {
			v.model = keep
			break
		}
	}
	if v.model == "" && v.defaultModel != "" {
		for _, m := range models {
			if m.Name == v.defaultModel {
				v.model = v.defaultModel
				break
			}
		}
	}
	if v.model == "" {
		v.model = models[0].Name
	}
	for i, m := range models {
		if m.Name == v.model {
			v.selIdx = i
			break
		}
	}
	return v, nil
}

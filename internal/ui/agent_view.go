package ui

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/log"

	"selftui/internal/agent"
	"selftui/internal/config"
	"selftui/internal/ollama"
	"selftui/internal/session"
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
	return AgentView{
		client:       client,
		ctx:          ctx,
		runner:       runnerFor(client, root, systemPrompt, agentCfg.MaxToolIterations, toolsEnabled),
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
	return v.selectorOpen || v.confirmation != nil || v.helpOpen || v.clearConfirm ||
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

// mergeThinkingDeltas folds pendingThinking into thinkingText (N6). Pure
// state merge on the repaint tick, mirroring mergeStreamDeltas; thinking
// never touches streamText so the assistant content stream stays aligned
// with the N2 render cache.
func (v *AgentView) mergeThinkingDeltas() {
	if v.pendingThinking == "" {
		return
	}
	// Reasoning comes off the remote response and can carry control bytes
	// like any model text; sanitize at the merge so every stored copy — the
	// live block and the committed turn — is clean (same discipline as
	// boundedToolLine).
	v.thinkingText += sanitizeTerminalText(v.pendingThinking)
	v.pendingThinking = ""
}

// boundedToolLine caps one tool-activity line's stored length so a 64 KiB
// tool result cannot flood the /details block (the full result already went
// back to the model through ToolResultMsg).
func boundedToolLine(s string) string {
	s = sanitizeTerminalText(s)
	const max = 2048
	if len(s) <= max {
		return s
	}
	return s[:max] + "…[truncated]"
}

// toolDetailRows bounds one tool-event line and splits it into one stored
// element per display row: streamLineCount accounts one row per element, so
// a multi-line tool summary stored as a single element would render taller
// than its count and push the composer/status area off-screen during the
// live tool phase.
func toolDetailRows(s string) []string {
	return strings.Split(boundedToolLine(s), "\n")
}

// reasoningBlock composes a turn's reasoning for display (N6): a muted
// marker row above the raw reasoning text. Plain text, no glamour —
// reasoning is verbose and the block must stay cheap to compose per frame.
func (v AgentView) reasoningBlock(thinking string) string {
	return v.styles.mutedText().Render("· reasoning\n" + thinking)
}

// plainChatFallbackNote is the persistent F1 caveat appended to a
// plain-chat-fallback assistant turn. The runner downgraded the turn before
// any tool executed, so a narrated tool claim ("I wrote file X") is not
// real; the note says so inline and in the exported/resumed transcript. It
// is a markdown blockquote, so the raw transcript file and the rendered
// terminal agree. reason is runner-supplied display text, sanitized when it
// entered state and again here (idempotent) before it joins the markdown
// body.
func plainChatFallbackNote(reason string) string {
	return "\n\n> ⚠ **plain chat** — no tool ran this turn: " + sanitizeTerminalText(reason)
}

// onChatDone finalizes a turn: commits the streamed text as an assistant
// message whose header carries elapsed + terminal reason right-aligned
// (M7-B/opencode-style), then surfaces an error unless the user stopped the
// stream with esc (a stop is not an error).
func (v AgentView) onChatDone(m agent.AgentDoneMsg) (AgentView, tea.Cmd) {
	// N2: land any deltas still awaiting the repaint tick before the commit —
	// the final chunk must flush immediately (no tick-interval tail latency
	// for the committed turn or the N3 footer metrics riding its meta row).
	v.mergeStreamDeltas()
	v.mergeThinkingDeltas() // a ThinkingMsg racing AgentDoneMsg flushes here too
	v.streaming = false
	v.stopCancel = nil
	v.stopArmed = false
	v.chatCh = nil
	v.chatDone = nil
	v.chatTerminal = nil // cA: the turn's retained terminal slot is consumed; next turn allocates a fresh one
	v.toolStatus = ""
	v.confirmation = nil

	// N3: the done event's metrics describe the payload that was just sent,
	// independent of whether any text streamed back. When the final chunk
	// carried a measured prompt token count, the ctx meter shows it until
	// the draft or turn changes; an older host (no metrics) clears any
	// stale measurement instead.
	if m.Metrics.PromptTokens > 0 {
		v.measuredPromptTokens = int(m.Metrics.PromptTokens)
		v.measuredDraft = v.input.Value()
	} else {
		v.measuredPromptTokens = 0
	}
	// N4: remember the measured rate for the shell status row; a user stop
	// keeps no rate, exactly like the turn footer.
	if t := turnTokPerSec(m.Metrics); t > 0 && !v.stopRequest {
		v.lastTokPerSec = t
	}

	var recCmd tea.Cmd // ack waiter for the committed assistant turn, if any

	if v.streamText != "" {
		// The Ollama done_reason on the done event is remote text rendered on
		// the assistant header; sanitize it before it becomes turn meta.
		// N3: when the final chunk carried generation metrics, the measured
		// tok/s rides the same meta row. The N6 extras (reasoning, tool
		// lines) commit with the content; a turn that produced no content
		// (error mid-loop) commits nothing, exactly as before N6.
		meta := turnFooter(v.turnStart, sanitizeTerminalText(m.Reason), v.stopRequest, turnTokPerSec(m.Metrics))
		// F1: a plain-chat fallback (agent.FallbackMsg) means the runner
		// downgraded this turn before any tool executed — a tool claim the
		// reply narrates is not real. Append the persistent caveat to the
		// committed content once, so the in-memory turn, its render cache,
		// the /export transcript, and a later /resume reload all carry it
		// (the transcript mirrors what the terminal shows).
		content := v.streamText
		if v.plainChatReason != "" {
			content += plainChatFallbackNote(v.plainChatReason)
			v.plainChatReason = ""
		}
		v.turns = append(v.turns, turn{
			msg:      ollama.ChatMessage{Role: ollama.RoleAssistant, Content: content},
			model:    v.model,
			meta:     meta,
			render:   v.renderBlock(v.assistantHeaderRow(v.model, meta), content),
			thinking: v.thinkingText,
			tools:    v.streamTools,
		})
		v, recCmd = v.enqueueSessionTurn("assistant", v.model, content, meta, time.Now())
		v.streamText = ""
		v.thinkingText = "" // N6: the turn's extras committed with it
		v.streamTools = nil
		v.resetStreamRender() // the active block committed; nothing cached to show
	}

	switch {
	case v.stopRequest:
		v.notice = "stopped" // esc asked to stop, even if the stream just finished
	case m.Err != "":
		// The error body can come from the remote host; sanitize before the
		// statusline renders it (H-05).
		v.chatErr = sanitizeTerminalText(m.Err)
	default:
		v.notice = ""
	}
	v.stopRequest = false
	v.follow = true
	return v, recCmd
}

// --- keys -----------------------------------------------------------------

func (v AgentView) handleKey(msg tea.KeyMsg) (AgentView, tea.Cmd) {
	if _, isPress := msg.(tea.KeyPressMsg); !isPress {
		return v, nil
	}
	k := msg.Key()

	// Hard modals own every key: a pending mutation approval, the model
	// selector, the /clear confirmation, and the /help overlay.
	if v.confirmation != nil {
		return v.confirmKey(k)
	}
	if v.selectorOpen {
		return v.selectorKey(k)
	}
	if v.clearConfirm {
		return v.clearConfirmKey(k)
	}
	if v.resumeConfirm {
		return v.resumeConfirmKey(k)
	}
	if v.resumeOpen {
		return v.resumeKey(k)
	}
	if v.helpOpen {
		if k.Code == tea.KeyEsc || k.Code == tea.KeyEnter || k.Text == "x" {
			v.helpOpen = false
		}
		return v, nil
	}

	// While a slash draft is showing, the draft's keys steer the menu
	// (slashDraftKey); every other key keeps editing the draft below.
	// The @-file picker (N6) sits one priority above it: while armed, its
	// nav keys steer the file list and every other key keeps editing the
	// draft (which is also the live filter).
	if v.fileMenuActive() {
		av, handled, cmd := v.filePickerKey(k)
		if handled {
			return av, cmd
		}
		v = av
	}
	if v.slashMenu() {
		av, handled, cmd := v.slashDraftKey(k)
		if handled {
			return av, cmd
		}
		v = av
	}

	switch {
	case k.Code == tea.KeyEnter && k.Mod.Contains(tea.ModShift):
		// shift+enter inserts a newline: forward a plain enter to the textarea
		// (bubbles key matching is modifier-sensitive, so the real event would
		// fall through every binding).
		ta, cmd := v.input.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		v.input = ta
		return v, cmd
	case k.Code == tea.KeyEnter && !k.Mod.Contains(tea.ModShift):
		// Plain enter sends; the textarea never sees it.
		if v.streaming {
			return v, nil // one turn at a time; swallow silently
		}
		return v.sendInput()
	case k.Code == tea.KeyEsc:
		switch {
		case v.streaming && v.stopCancel != nil:
			// Armed interrupt (opencode-style): the first esc while running
			// only warns — the statusline flips to "esc again to interrupt" —
			// and only the second cancels, so a stray esc can't kill a long
			// generation.
			if v.stopArmed {
				v.stopRequest = true
				v.stopCancel()
			} else {
				v.stopArmed = true
			}
		case v.input.Value() != "":
			// Idle with a drafted prompt: esc clears it (M7-A). A second esc
			// on an already-empty input is a no-op.
			ti := v.input
			ti.Reset()
			v.input = ti
			v.slashQuery = ""
			v.slashIdx = 0
			v.fileOpen = false // N6: a cleared draft disarms the @-picker
			return v.fitComposer(), nil
		}
		return v, nil
	case (k.Code == tea.KeyPgUp || k.Code == tea.KeyPgDown) && v.input.Value() == "" && !v.streaming:
		return v.pageScroll(k.Code == tea.KeyPgUp)
	case k.Text == "f" && v.input.Value() == "" && !v.streaming:
		// Auto-follow toggle (M7-B): off lets pgup/u scroll away; on snaps
		// back to the live tail.
		if v.follow {
			v.follow = false
		} else {
			v.follow = true
			v.scroll = 0
		}
		return v, nil
	case k.Text == "m" && v.input.Value() == "" && !v.streaming:
		// The letter commands (m/r/u/d/f) only fire while the input is empty,
		// so typing ordinary prose never triggers them (confirmed live: the
		// 'm' in "stop me" opened the selector — M2 lesson).
		return v.openSelector()
	case k.Text == "r" && v.input.Value() == "" && !v.streaming:
		v.loading = true
		v.modelsErr = ""
		return v, v.loadModelsCmd()
	case k.Text == "u" && v.input.Value() == "":
		v.scroll++
		v.follow = false
		v.clampScroll()
		return v, nil
	case k.Text == "d" && v.input.Value() == "":
		v.scroll--
		v.clampScroll()
		if v.scroll <= 0 {
			v.follow = true // reaching the tail re-engages auto-follow
		}
		return v, nil
	}

	ta, cmd := v.input.Update(msg)
	v.input = ta
	// N6: a fresh "@" arms the file picker (typing, not streaming — the
	// slash menu follows the same rule). Every other edit re-checks the
	// trigger word so the picker closes itself when the '@' or the query is
	// edited away.
	if k.Text == "@" && !v.streaming && !v.fileOpen {
		v.fileOpen = true
		v.fileLoading = true
		v.fileList = nil
		v.fileIdx = 0
		v.fileFilter = ""
		return v.fitComposer(), v.listFilesCmd()
	}
	if v.fileOpen {
		if _, _, ok := v.atQuery(); !ok {
			v.fileOpen = false
		}
	}
	return v.fitComposer(), cmd
}

// confirmKey owns the mutation-approval dialog's keys: y or enter approves,
// n or esc declines, and any other key is swallowed (the dialog has no
// default affirmative key). The runner is unblocked either way.
func (v AgentView) confirmKey(k tea.Key) (AgentView, tea.Cmd) {
	switch {
	case k.Text == "y" || k.Code == tea.KeyEnter:
		v.confirmation.Respond(true)
		v.notice = "approved " + v.confirmation.Name
		v.confirmation = nil
	case k.Text == "n" || k.Code == tea.KeyEsc:
		v.confirmation.Respond(false)
		v.notice = "declined " + v.confirmation.Name
		v.confirmation = nil
	}
	return v, nil
}

// slashDraftKey owns the composer keys while the "/" command menu is
// showing: arrows steer the highlighted row, enter runs it, and esc drops
// the whole draft. Every other key keeps editing the draft (handled=false),
// so the menu filters live as characters land (backspace too).
func (v AgentView) slashDraftKey(k tea.Key) (AgentView, bool, tea.Cmd) {
	switch {
	case k.Code == tea.KeyEsc:
		ti := v.input
		ti.Reset()
		v.input = ti
		v.slashQuery = ""
		v.slashIdx = 0
		return v.fitComposer(), true, nil
	case k.Code == tea.KeyEnter && !k.Mod.Contains(tea.ModShift):
		av, cmd := v.runSlashCommand()
		return av, true, cmd
	case k.Code == tea.KeyUp:
		if v.slashIdx > 0 {
			v.slashIdx--
		}
		return v, true, nil
	case k.Code == tea.KeyDown:
		if n := len(v.slashMatches()); v.slashIdx < n-1 {
			v.slashIdx++
		}
		return v, true, nil
	}
	// Draft text changed: reset the highlight to the top row unless the
	// filter still selects the same command (keep it simple: top row).
	if q := v.slashQueryOf(); q != v.slashQuery {
		v.slashQuery = q
		v.slashIdx = 0
	}
	return v, false, nil
}

// fitComposer re-sizes the prompt textarea to the current width and content
// height. The composer grows from one to composerMaxRows rows as a
// multi-line prompt is typed (opencode-style auto-grow); sizing happens on
// every text change, geometry change, and reset so wrapping and the pane
// height stay correct.
func (v AgentView) fitComposer() AgentView {
	ta := v.input
	ta.SetWidth(maxInt(v.w-2, 10))
	ta.SetHeight(composerRowsFor(ta.Value(), maxInt(v.w-2, 10)))
	v.input = ta
	return v
}

// composerMaxRows caps the auto-growing composer prompt.
const composerMaxRows = 4

// composerRowsFor counts how many terminal rows a prompt occupies in a
// textarea taW columns wide: one per physical line plus extra rows for
// wrapping (the prompt glyph shortens the first line).
func composerRowsFor(value string, taW int) int {
	if taW < 1 {
		taW = 1
	}
	lines := strings.Split(value, "\n")
	n := 0
	for j, l := range lines {
		w := lipgloss.Width(l)
		if w == 0 {
			n++
			continue
		}
		usable := taW
		if j == 0 {
			usable = maxInt(taW-2, 1) // the "❯ " prompt shares the first row
		}
		n += 1 + (w-1)/usable
	}
	return clampInt(n, 1, composerMaxRows)
}

// pageScroll moves the transcript window one visible page (pgup up, pgdn
// down). Reaching the tail on the way down re-engages auto-follow.
func (v AgentView) pageScroll(up bool) (AgentView, tea.Cmd) {
	page := maxInt(1, v.pageHeight())
	if up {
		v.scroll += page
		v.follow = false
	} else {
		v.scroll -= page
		if v.scroll <= 0 {
			v.scroll = 0
			v.follow = true
			return v, nil
		}
		v.follow = false
	}
	v.clampScroll()
	return v, nil
}

// pageHeight is the number of visible transcript rows used as the pgup/pgdn
// page size — the same accounting renderChatPane uses for its window
// (composer height is dynamic: it grows with the prompt).
func (v AgentView) pageHeight() int {
	bodyH := maxInt(v.h-2, 1)
	chatH := maxInt(bodyH-v.composerRows()-3-1, 1)
	return maxInt(chatH-2, 1)
}

// sendInput appends the typed text as a user message and starts a stream.
func (v AgentView) sendInput() (AgentView, tea.Cmd) {
	text := strings.TrimSpace(v.input.Value())
	if text == "" {
		return v, nil
	}
	if v.resumePending {
		// A transcript load is in flight and will replace the conversation:
		// refuse the send (the draft stays in the input) instead of letting
		// the import destroy it on landing.
		v.notice = "resume in progress — the transcript is still loading"
		return v, nil
	}
	if v.model == "" {
		v.notice = "no model selected — press m or pull one in the Models tab"
		return v, nil
	}
	if v.expandPending {
		// The previous send's @-references are still expanding off the update
		// loop; a second send would race the first turn's start.
		v.notice = "attachments are still expanding — try again in a moment"
		return v, nil
	}
	i := v.input
	i.Reset()
	v.input = i
	v.fileOpen = false
	v = v.fitComposer()
	v.measuredPromptTokens = 0 // a new draft/send: budget check is approximate again

	// N6: expand @-references through the jailed read_file. The transcript
	// renders the draft; the wire content carries the inline file blocks.
	// The reads are bounded by the read_file ceiling (maxReadBytes per file).
	// When the draft carries references the expansion runs in a command —
	// one filesystem read per token must never block the update loop (a
	// slow/network filesystem or a referenced FIFO would freeze repaint and
	// even Esc) — and beginTurn starts the turn when it lands.
	if refs := agent.FileRefTokens(text); len(refs) > 0 {
		root := v.runner.Root()
		ctx := v.ctx
		v.expandPending = true
		return v, func() tea.Msg {
			return attachExpandedMsg{draft: text, wire: agent.ExpandFileRefs(ctx, root, text)}
		}
	}
	return v.beginTurn(text, "")
}

// beginTurn commits the user turn and starts the chat. It is the landing
// path for the deferred attachment expansion (attachExpandedMsg) and the
// direct path when the draft carries no @-references. The remote-attachment
// warning rides after startChat (which resets the notice for the new turn)
// so it survives to the post-turn statusline.
func (v AgentView) beginTurn(text, wire string) (AgentView, tea.Cmd) {
	v.expandPending = false
	if v.resumePending {
		// A transcript load raced the (now async) expansion: refuse the send
		// and put the draft back — same rule as sendInput — instead of
		// letting the import destroy it on landing.
		ta := v.input
		ta.SetValue(text)
		v.input = ta
		v.notice = "resume in progress — the transcript is still loading"
		return v, nil
	}
	warn := ""
	if wire != "" && wire != text && v.host != "" && !config.LoopbackHost(v.host) {
		warn = "⚠ attached workspace content sent to " + v.host
	}
	v.turns = append(v.turns, turn{
		msg:    ollama.ChatMessage{Role: ollama.RoleUser, Content: text},
		model:  v.model,
		render: v.renderBlock(v.userHeader(), text),
		wire:   wire,
	})
	v.checkContextBudget()
	v, recCmd := v.enqueueSessionTurn("user", v.model, text, "", time.Now())
	av, chatCmd := v.startChat()
	// The remote-attachment warning rides after startChat (which resets the
	// notice for the new turn) so it survives to the post-turn statusline.
	if warn != "" {
		av.notice = warn
	}
	return av, tea.Batch(recCmd, chatCmd)
}

// enqueueSessionTurn mirrors one committed turn onto the ordered background
// recorder (constructed once at composition-root wiring in WithSessionDir).
// It takes a pointer receiver so the recorder outcome (accepted turn, first
// failure) lands on the calling view, not a discarded copy. The update loop
// only enqueues immutable work and arms the ack command; Open/Append/Flush/
// Close all happen on the recorder worker, so a slow or stalled transcript
// directory can never block Update (M-04). On the first failure the ack
// disables the log once and one notice tells the user where it failed — a
// transcript is never worth breaking the chat for.
func (v *AgentView) enqueueSessionTurn(role, model, content, meta string, at time.Time) (AgentView, tea.Cmd) {
	if v.sessionDir == "" || v.sessionErr || v.recorder == nil {
		return *v, nil
	}
	// The transcript mirrors what the terminal shows, so committed content
	// is sanitized the same way (the file can otherwise be re-opened in a
	// terminal-paging editor where control bytes would execute).
	done, err := v.recorder.Append(role, model, sanitizeTerminalText(content), meta, at)
	if err != nil {
		// Backlog full: the sink is wedged; recording is over for this run.
		v.sessionErr = true
		v.sessionErrMsg = err.Error()
		v.notice = "session log: " + err.Error()
		return *v, nil
	}
	v.recorded = true
	return *v, func() tea.Msg {
		res := <-done
		if res.Err == nil {
			return nil // a successful append needs no UI round-trip
		}
		return agentEventMsg{msg: sessionAppendMsg{err: res.Err}}
	}
}

// WithSessionDir enables transcript persistence under dir with host recorded
// in the file header (the composition root — main via App — wires it once at
// startup; empty dir disables). The concrete recorder is constructed here,
// not lazily on the update loop: a committed turn only ever enqueues work
// onto the existing dependency. Re-wiring closes the previous recorder first
// (Close is idempotent), so repeated calls never strand a worker.
func (v AgentView) WithSessionDir(dir, host string) AgentView {
	if v.recorder != nil {
		v.recorder.Close()
	}
	v.sessionDir = dir
	v.sessionHost = host
	v.recorder = nil
	v.recorded = false
	v.sessionErr = false
	v.sessionErrMsg = ""
	if dir != "" {
		v.recorder = session.NewRecorder(dir, host)
	}
	return v
}

// CloseRecorder flushes every committed turn and stops the transcript
// recorder's worker. It is the normal-shutdown lifecycle boundary (main calls
// it after the tea program exits); a nil recorder (recording disabled or no
// turn yet) is a no-op, and Close is idempotent.
func (v AgentView) CloseRecorder() error {
	if v.recorder == nil {
		return nil
	}
	return v.recorder.Close()
}

// exportSession flushes the Markdown transcript and reports its path. The
// flush runs on the recorder worker strictly after every earlier enqueued
// turn (ordered jobs), so the reported path is exact and the export is
// append-only and cannot be resumed (chat stays in-memory) — the notice
// reports the file and never claims the conversation can be reloaded. Update
// only enqueues and processes the completion message (M-04).
func (v AgentView) exportSession() (AgentView, tea.Cmd) {
	switch {
	case v.sessionDir == "":
		v.notice = "session recording is off — no transcript is written"
		return v, nil
	case v.sessionErr:
		// Recording failed earlier and is permanently off for this run, so
		// "send a message first" would be dead-end advice: echo the real
		// failure instead (P1-5).
		v.notice = "session recording failed: " + v.sessionErrMsg
		return v, nil
	case v.recorder == nil || !v.recorded:
		// The composition root wires the recorder eagerly; "no recorder" can
		// only mean recording never accepted a turn — same advice either way.
		v.notice = "nothing recorded yet — send a message first"
		return v, nil
	}
	done, err := v.recorder.Flush()
	if err != nil {
		v.sessionErr = true
		v.sessionErrMsg = err.Error()
		v.notice = "session log: " + err.Error()
		return v, nil
	}
	return v, func() tea.Msg {
		res := <-done
		return agentEventMsg{msg: sessionExportMsg{path: res.Path, err: res.Err}}
	}
}

// applySessionExport lands one /export completion on the view. A failure
// disables recording once (same one-error surface as an append failure); an
// empty path means nothing was ever recorded.
func (v AgentView) applySessionExport(m sessionExportMsg) AgentView {
	if m.err != nil {
		v.sessionErr = true
		v.sessionErrMsg = m.err.Error()
		v.notice = "session log: " + m.err.Error()
		return v
	}
	if m.path == "" {
		v.notice = "nothing recorded yet — send a message first"
		return v
	}
	v.notice = "transcript: " + m.path
	return v
}

// --- resume (V2a: reload a saved transcript into the live conversation) ---

// openResume starts the /resume flow: the picker lists the saved transcripts
// under sessionDir. Listing runs in a command (M-04 discipline: no
// transcript filesystem work on the update loop); the notice cases below
// mirror /export's (recording off, nothing saved yet).
func (v AgentView) openResume() (AgentView, tea.Cmd) {
	switch {
	case v.sessionDir == "":
		v.notice = "session recording is off — nothing to resume"
		return v, nil
	case v.streaming:
		return v, nil // one turn at a time; /resume while running is a no-op
	}
	v.resumeOpen = true
	v.resumeLoading = true
	v.resumeList = nil
	v.resumeIdx = 0
	return v, v.listSessionsCmd()
}

// listSessionsCmd reads the saved-transcript listing off the update loop.
func (v AgentView) listSessionsCmd() tea.Cmd {
	dir := v.sessionDir
	return func() tea.Msg {
		list, err := session.ListSessions(dir)
		return agentEventMsg{msg: sessionListMsg{list: list, err: err}}
	}
}

// applySessionList lands the picker listing. A listing failure closes the
// picker and surfaces one notice — the same once-and-continue posture as
// the recorder (the chat never blocks on the disk).
func (v AgentView) applySessionList(m sessionListMsg) AgentView {
	v.resumeLoading = false
	if m.err != nil {
		v.resumeOpen = false
		v.notice = "resume: " + m.err.Error()
		return v
	}
	v.resumeList = m.list
	v.resumeIdx = 0
	if len(m.list) == 0 {
		v.resumeOpen = false
		v.notice = "no saved sessions"
	}
	return v
}

// resumeKey owns the picker's keys (same shape as the model selector: esc
// close, enter pick, j/k or arrows move). Picking with a live conversation
// asks first — resuming replaces the in-memory history.
func (v AgentView) resumeKey(k tea.Key) (AgentView, tea.Cmd) {
	switch {
	case k.Code == tea.KeyEsc:
		v.resumeOpen = false
		v.resumeList = nil
		v.resumeIdx = 0
	case k.Code == tea.KeyEnter:
		if len(v.resumeList) > 0 && v.resumeIdx >= 0 && v.resumeIdx < len(v.resumeList) {
			path := v.resumeList[v.resumeIdx].Path
			if len(v.turns) > 0 || v.streamText != "" {
				v.resumePick = path
				v.resumeConfirm = true
				return v, nil
			}
			return v.importSession(path)
		}
	case k.Text == "j" || k.Code == tea.KeyDown:
		if v.resumeIdx < len(v.resumeList)-1 {
			v.resumeIdx++
		}
	case k.Text == "k" || k.Code == tea.KeyUp:
		if v.resumeIdx > 0 {
			v.resumeIdx--
		}
	}
	return v, nil
}

// resumeConfirmKey handles y/enter (proceed) vs n/esc (cancel) in the
// overwrite dialog, mirroring the /clear confirmation.
func (v AgentView) resumeConfirmKey(k tea.Key) (AgentView, tea.Cmd) {
	switch {
	case k.Text == "y" || k.Code == tea.KeyEnter:
		v.resumeConfirm = false
		path := v.resumePick
		v.resumePick = ""
		return v.importSession(path)
	case k.Text == "n" || k.Code == tea.KeyEsc:
		// Cancel backs out of the whole flow — dialog and picker — so the
		// next keypress reaches the composer instead of a hidden list.
		v.resumeConfirm = false
		v.resumeOpen = false
		v.resumeList = nil
		v.resumeIdx = 0
		v.resumePick = ""
		v.notice = "resume cancelled"
	}
	return v, nil
}

// importSession starts the parse of one chosen transcript. Parsing runs in
// a command; applySessionLoaded performs the actual import once the turns
// arrive. resumePending blocks sends for the whole window, so the import
// can never race a turn the user sends mid-load into oblivion.
func (v AgentView) importSession(path string) (AgentView, tea.Cmd) {
	v.resumeOpen = false
	v.resumeList = nil
	v.resumeIdx = 0
	v.resumePending = true
	return v, func() tea.Msg {
		turns, err := session.Load(path)
		return agentEventMsg{msg: sessionLoadedMsg{path: path, turns: turns, err: err}}
	}
}

// applySessionLoaded imports parsed turns into the live conversation.
//
// Safe-import semantics (V2a): transcripts only ever record committed
// user/assistant turns — tool-call activity lives on the statusline and is
// never written — so an import can never fabricate tool state. The imported
// turns enter as plain history for the next runner request. The truncation
// flag recomputes here over system prompt + imported history (mirroring a
// normal commit), so an over-budget transcript shows the truncation marker
// immediately instead of only after the next send. Imported model/meta
// strings and the notice's file name are file-derived display text and are
// sanitized like any other remote-derived string before they enter state.
// The next send keeps the currently configured/selected model — the
// picker's rows show historical names only and never switch the active
// model. The import replaces the in-memory conversation only; the new
// run's transcript file stays append-only and records just the turns sent
// after the resume.
func (v AgentView) applySessionLoaded(m sessionLoadedMsg) AgentView {
	// The load window is over in every outcome: unblock sends whether the
	// import succeeded, failed, or was superseded.
	v.resumePending = false
	if m.err != nil {
		v.notice = "resume failed: " + m.err.Error()
		return v
	}
	v.turns = v.turns[:0]
	for _, t := range m.turns {
		role := ollama.RoleUser
		if t.Role == "assistant" {
			role = ollama.RoleAssistant
		}
		// Imported metadata is file-derived text: sanitize it before it
		// enters state or the render cache (same boundary as streamed
		// tokens and model names).
		model := sanitizeTerminalText(t.Model)
		meta := sanitizeTerminalText(t.Meta)
		header := v.userHeader()
		if role == ollama.RoleAssistant {
			header = v.assistantHeaderRow(model, meta)
		}
		v.turns = append(v.turns, turn{
			msg:    ollama.ChatMessage{Role: role, Content: t.Content},
			model:  model,
			meta:   meta,
			render: v.renderBlock(header, t.Content),
		})
	}
	// Budget recompute on import (mirrors a normal commit's
	// checkContextBudget): an over-budget transcript must surface the
	// truncation marker now, not only after the next send.
	v.truncated = false
	v.measuredPromptTokens = 0 // the conversation was replaced wholesale
	v.lastTokPerSec = 0        // its last turn's rate no longer describes this one
	v.checkContextBudget()
	v.scroll = 0
	v.follow = true
	name := m.path
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	name = sanitizeTerminalText(name)
	v.notice = fmt.Sprintf("resumed %d turns from %s", len(m.turns), name)
	return v
}

// --- slash commands (M7-A) ------------------------------------------------

// slashCommand is one entry of the "/" command menu.
type slashCommand struct {
	name string // matched after the leading "/"
	desc string
}

func slashCommandList() []slashCommand {
	return []slashCommand{
		{"clear", "clear the conversation (asks first)"},
		{"model", "pick a model (m)"},
		{"resume", "resume a saved chat transcript"},
		{"theme", "toggle dark/light for this session"},
		{"details", "toggle tool-output blocks"},
		{"thinking", "toggle reasoning blocks"},
		{"export", "flush + reveal the transcript file path"},
		{"help", "list slash commands and keys"},
		{"refresh", "reload the model list (r)"},
	}
}

// slashQueryOf is the lowercased filter text after the leading "/".
func (v AgentView) slashQueryOf() string {
	return strings.ToLower(strings.TrimPrefix(v.input.Value(), "/"))
}

// slashMenu reports whether the command menu should show: the input holds a
// "/" draft that still matches at least one command. A draft matching
// nothing ("how do I write a /"? chat about a file named /x) is treated as
// ordinary prose and typed/sent normally.
func (v AgentView) slashMenu() bool {
	if v.streaming || v.input.Value() == "" || !strings.HasPrefix(v.input.Value(), "/") {
		return false
	}
	return len(v.slashMatches()) > 0
}

// slashMatches returns the commands whose name starts with the draft filter.
func (v AgentView) slashMatches() []slashCommand {
	q := v.slashQueryOf()
	var out []slashCommand
	for _, c := range slashCommandList() {
		if strings.HasPrefix(c.name, q) {
			out = append(out, c)
		}
	}
	return out
}

// runSlashCommand executes the highlighted command, consuming the draft. Only
// reachable while the menu is open (the highlight is in range).
func (v AgentView) runSlashCommand() (AgentView, tea.Cmd) {
	matches := v.slashMatches()
	if len(matches) == 0 {
		return v, nil
	}
	if v.slashIdx < 0 || v.slashIdx >= len(matches) {
		v.slashIdx = 0
	}
	name := matches[v.slashIdx].name

	ti := v.input
	ti.Reset()
	v.input = ti
	v.slashQuery = ""
	v.slashIdx = 0
	v.fileOpen = false
	v = v.fitComposer() // the draft was consumed; shrink the composer back

	switch name {
	case "clear":
		if len(v.turns) == 0 && v.streamText == "" {
			v.notice = "nothing to clear"
			return v, nil
		}
		v.clearConfirm = true
		return v, nil
	case "resume":
		return v.openResume()
	case "model":
		return v.openSelector()
	case "theme":
		next := "dark"
		if v.dark {
			next = "light"
		}
		// The theme lives on the root App (shared with Settings and Models);
		// emit a message and let App apply it shell-wide.
		return v, func() tea.Msg { return agentThemeMsg{theme: next} }
	case "help":
		v.helpOpen = true
		return v, nil
	case "details":
		// N6: gate the tool-activity blocks (display only — the agent loop's
		// wire payload is untouched). Session-scoped, default off.
		v.showDetails = !v.showDetails
		v.notice = "tool-output blocks " + toggleWord(v.showDetails)
		return v, nil
	case "thinking":
		// N6: surface per-turn reasoning blocks (qwen3 thinking; stored
		// always, shown only behind this toggle). Session-scoped, default off.
		v.showThinking = !v.showThinking
		v.notice = "reasoning blocks " + toggleWord(v.showThinking)
		return v, nil
	case "export":
		return v.exportSession()
	case "refresh":
		v.loading = true
		v.modelsErr = ""
		return v, v.loadModelsCmd()
	}
	return v, nil
}

// toggleWord renders a toggle state for notices.
func toggleWord(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

// --- @-file picker (N6) ---------------------------------------------------

// fileMenuMaxRows caps the @-picker's file rows so the menu never needs its
// own scroll and the chat pane keeps room at the phone geometry.
const fileMenuMaxRows = 7

// atQuery reports the composer's live @-trigger: the byte index of the last
// '@' in the draft and the word after it. The trigger is word-bounded — any
// whitespace after the '@' ends it — so the filter is always exactly the
// text the user can see and the picker can never desync from the draft.
func (v AgentView) atQuery() (at int, query string, ok bool) {
	val := v.input.Value()
	i := strings.LastIndexByte(val, '@')
	if i < 0 {
		return 0, "", false
	}
	tail := val[i+1:]
	if strings.ContainsAny(tail, " \t\n\r") {
		return 0, "", false
	}
	return i, tail, true
}

// fileMenuActive reports whether the @-picker owns navigation keys: armed
// and the trigger word still intact.
func (v AgentView) fileMenuActive() bool {
	if !v.fileOpen || v.confirmation != nil {
		return false
	}
	_, _, ok := v.atQuery()
	return ok
}

// fileMatches applies the live filter (the trigger word) to the workspace
// listing. Ranking is fuzzy but predictable: substring matches first (by
// position, then path length), then subsequence matches (fuzzy, by path
// length, then lexicographic). The list is already jail-bounded — the walk
// never leaves the workspace — and every picked path is re-verified through
// the jailed read at expansion time.
func (v AgentView) fileMatches() []string {
	_, q, ok := v.atQuery()
	if !ok || (v.fileLoading && len(v.fileList) == 0) {
		return nil
	}
	q = strings.ToLower(q)
	type scored struct {
		path  string
		class int // 0 = substring, 1 = subsequence
		index int // substring position (lower ranks earlier)
	}
	out := make([]scored, 0, len(v.fileList))
	for _, p := range v.fileList {
		low := strings.ToLower(p)
		if q == "" {
			out = append(out, scored{p, 0, 0})
			continue
		}
		if i := strings.Index(low, q); i >= 0 {
			out = append(out, scored{p, 0, i})
			continue
		}
		if isFuzzySubsequence(low, q) {
			out = append(out, scored{p, 1, 0})
		}
	}
	sort.Slice(out, func(a, b int) bool {
		x, y := out[a], out[b]
		if x.class != y.class {
			return x.class < y.class
		}
		if x.class == 0 && x.index != y.index {
			return x.index < y.index
		}
		if len(x.path) != len(y.path) {
			return len(x.path) < len(y.path)
		}
		return x.path < y.path
	})
	paths := make([]string, len(out))
	for i, s := range out {
		paths[i] = s.path
	}
	return paths
}

// isFuzzySubsequence reports whether q appears in low as an in-order
// subsequence (byte-wise; the filter is lowercased on both sides).
func isFuzzySubsequence(low, q string) bool {
	i := 0
	for j := 0; j < len(low) && i < len(q); j++ {
		if low[j] == q[i] {
			i++
		}
	}
	return i == len(q)
}

// filePickerKey owns the composer keys while the @-picker is active:
// arrows/j-k steer the highlighted row, enter attaches the selection, esc
// disarms (the draft keeps the typed query). Every other key keeps editing
// the draft — which is the live filter (handled=false falls through to the
// textarea; the trigger re-check closes the picker when the word ends).
// Navigation and enter claim a key only while the filtered list has rows:
// with nothing to act on (listing still loading, no match), the keys fall
// through so enter still sends the draft instead of dying in an empty menu.
func (v AgentView) filePickerKey(k tea.Key) (AgentView, bool, tea.Cmd) {
	matches := v.fileMatches()
	switch {
	case k.Code == tea.KeyEsc:
		v.fileOpen = false
		v.fileIdx = 0
		v.fileFilter = ""
		return v, true, nil
	case k.Code == tea.KeyEnter && !k.Mod.Contains(tea.ModShift) && len(matches) > 0:
		av, cmd := v.pickFile()
		return av, true, cmd
	case (k.Text == "j" || k.Code == tea.KeyDown) && len(matches) > 0:
		if v.fileIdx < len(matches)-1 {
			v.fileIdx++
		}
		return v, true, nil
	case (k.Text == "k" || k.Code == tea.KeyUp) && len(matches) > 0:
		if v.fileIdx > 0 {
			v.fileIdx--
		}
		return v, true, nil
	}
	// Draft text changed: reset the highlight to the top row when the
	// filter changed (same convention as the slash menu).
	if _, q, ok := v.atQuery(); ok && q != v.fileFilter {
		v.fileFilter = q
		v.fileIdx = 0
	}
	return v, false, nil
}

// pickFile inserts the highlighted file as "@path " in place of the typed
// query and disarms the picker. The send path — not the picker — does the
// attachment: the reference is expanded through the jailed read_file when
// the draft is sent, so what the user picked and what the model receives
// always go through the same gate.
func (v AgentView) pickFile() (AgentView, tea.Cmd) {
	matches := v.fileMatches()
	if len(matches) == 0 {
		v.fileOpen = false
		v.fileIdx = 0
		v.fileFilter = ""
		return v, nil
	}
	// Read the highlighted entry before resetting the picker state: the
	// reset must not clobber which row the user picked.
	path := matches[v.fileIdx]
	v.fileOpen = false
	v.fileIdx = 0
	v.fileFilter = ""
	at, _, ok := v.atQuery()
	if !ok {
		return v, nil
	}
	ta := v.input
	// Spaces are escaped ("\ ") so the send-path tokenizer keeps the whole
	// path as one reference (paths with spaces are offered by the picker).
	ta.SetValue(v.input.Value()[:at] + "@" + agent.EscapeFileRef(path) + " ")
	v.input = ta
	return v.fitComposer(), nil
}

// listFilesCmd walks the jailed workspace off the update loop (M-04
// discipline: no filesystem work in Update). The listing is advisory; a
// failure lists as empty ("no files"), never an error surface.
func (v AgentView) listFilesCmd() tea.Cmd {
	root := v.runner.Root()
	ctx := v.ctx
	return func() tea.Msg {
		return agentEventMsg{msg: fileListMsg{files: agent.WorkspaceFiles(ctx, root)}}
	}
}

// fileMenuHeight is the bordered menu's row budget while the @-picker is
// active (rows + hint row + box borders).
func (v AgentView) fileMenuHeight() int {
	rows := len(v.fileMatches())
	if rows > fileMenuMaxRows {
		rows = fileMenuMaxRows
	}
	if rows == 0 {
		rows = 1 // the "listing…" / "no files" row
	}
	return rows + 3
}

// renderFileMenu draws the bordered @-file menu between the transcript and
// the input (same anatomy as the slash menu): the windowed, filtered file
// rows plus one hint row.
func (v AgentView) renderFileMenu() string {
	matches := v.fileMatches()
	innerW := maxInt(v.w-2, 16)

	var rows []string
	if len(matches) == 0 {
		label := "no files match"
		if v.fileLoading {
			label = "listing workspace…"
		}
		rows = append(rows, v.styles.Placeholder.Render(truncateToWidth(label, innerW)))
	} else {
		start := clampInt(v.fileIdx-fileMenuMaxRows/2, 0, maxInt(0, len(matches)-fileMenuMaxRows))
		end := start + fileMenuMaxRows
		if end > len(matches) {
			end = len(matches)
		}
		for i := start; i < end; i++ {
			marker := "  "
			row := marker + matches[i]
			if i == v.fileIdx {
				marker = "❯ "
				row = marker + lipgloss.NewStyle().Bold(true).Foreground(v.styles.accent).Render(matches[i])
			}
			rows = append(rows, truncateToWidth(row, innerW))
		}
	}
	rows = append(rows, v.styles.Placeholder.Render(truncateToWidth("type to filter · ↑/↓ or j/k move · enter attach · esc close", innerW)))
	return v.styles.Pane.Width(v.w).Height(v.fileMenuHeight()).Render(strings.Join(rows, "\n"))
}

// --- model selector (M7-C: filter as you type) ----------------------------

func (v AgentView) openSelector() (AgentView, tea.Cmd) {
	if len(v.models) == 0 {
		v.notice = "no models — pull one from the Models tab (p)"
		return v, nil
	}
	v.selectorOpen = true
	v.selFilter = ""
	v.reanchorSelector()
	return v, nil
}

// filteredModels applies the live selector filter, a case-insensitive
// substring across name, family, parameter size, and quantization.
func (v AgentView) filteredModels() []ollama.Model {
	f := strings.ToLower(strings.TrimSpace(v.selFilter))
	if f == "" {
		return v.models
	}
	var out []ollama.Model
	for _, m := range v.models {
		hay := strings.ToLower(strings.Join([]string{m.Name, m.Family, m.ParameterSize, m.Quantization}, " "))
		if strings.Contains(hay, f) {
			out = append(out, m)
		}
	}
	return out
}

// reanchorSelector points the highlight at the current chat model when a
// filter edit still contains it, else at the top of the filtered list.
func (v *AgentView) reanchorSelector() {
	if v.model != "" {
		for i, m := range v.filteredModels() {
			if m.Name == v.model {
				v.selIdx = i
				return
			}
		}
	}
	v.selIdx = 0
}

func (v AgentView) selectorKey(k tea.Key) (AgentView, tea.Cmd) {
	list := v.filteredModels()
	switch {
	case k.Code == tea.KeyEsc:
		v.selectorOpen = false
		v.selFilter = ""
		v.selIdx = 0
	case k.Code == tea.KeyEnter:
		if len(list) > 0 && v.selIdx >= 0 && v.selIdx < len(list) {
			v.model = list[v.selIdx].Name
			v.notice = "model " + v.model
		}
		v.selectorOpen = false
		v.selFilter = ""
	case k.Text == "j" || k.Code == tea.KeyDown:
		if v.selIdx < len(list)-1 {
			v.selIdx++
		}
	case k.Text == "k" || k.Code == tea.KeyUp:
		if v.selIdx > 0 {
			v.selIdx--
		}
	case k.Code == tea.KeyBackspace:
		if v.selFilter != "" {
			v.selFilter = trimLastRune(v.selFilter)
			v.reanchorSelector()
		}
	default:
		// Every other printable rune extends the live filter. j/k stay
		// reserved for navigation; spaces are legal filter characters.
		if t := k.Text; t != "" {
			v.selFilter += t
			v.reanchorSelector()
		}
	}
	return v, nil
}

// trimLastRune removes the final rune (used by the selector filter backspace).
func trimLastRune(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	return string(r[:len(r)-1])
}

// clearConfirmKey handles y/enter (wipe) vs n/esc (cancel) in the /clear dialog.
func (v AgentView) clearConfirmKey(k tea.Key) (AgentView, tea.Cmd) {
	switch {
	case k.Text == "y" || k.Code == tea.KeyEnter:
		v.clearConfirm = false
		v.notice = "conversation cleared"
		v.turns = nil
		v.scroll = 0
		v.follow = true
		v.truncated = false
		v.measuredPromptTokens = 0
		v.lastTokPerSec = 0 // the conversation was wiped; the rate described its last turn
	case k.Text == "n" || k.Code == tea.KeyEsc:
		v.clearConfirm = false
		v.notice = "clear cancelled"
	}
	return v, nil
}

// --- context budget (M7-C) ------------------------------------------------

// payloadMessages mirrors exactly what the runner will send on the next turn
// (system prompt + committed conversation + in-flight/drafted text), so the
// meter and truncation marker budget against the same payload BudgetMessages
// sees.
func (v AgentView) payloadMessages() []ollama.ChatMessage {
	var msgs []ollama.ChatMessage
	if v.systemPrompt != "" {
		msgs = append(msgs, ollama.ChatMessage{Role: ollama.RoleSystem, Content: v.systemPrompt})
	}
	for i := range v.turns {
		// N6: a user turn with @-references sends its expanded wire content,
		// so the meter and truncation marker budget against the true payload.
		content := v.turns[i].msg.Content
		if v.turns[i].msg.Role == ollama.RoleUser && v.turns[i].wire != "" {
			content = v.turns[i].wire
		}
		msgs = append(msgs, ollama.ChatMessage{Role: v.turns[i].msg.Role, Content: content})
	}
	// N2: budget against the complete stream (deltas awaiting the tick
	// included) — the payload must match what the runner would send.
	if v.streaming {
		if live := v.liveStreamText(); live != "" {
			msgs = append(msgs, ollama.ChatMessage{Role: ollama.RoleAssistant, Content: live})
		}
	}
	if v.input.Value() != "" {
		msgs = append(msgs, ollama.ChatMessage{Role: ollama.RoleUser, Content: v.input.Value()})
	}
	return msgs
}

// ctxTokens counts the approximate tokens of the next payload using the same
// estimator as agent.BudgetMessages (4 chars per token).
func (v AgentView) ctxTokens() int {
	// N3: while the draft is untouched since the measured turn completed,
	// the meter shows the host-reported prompt token count (exact) instead
	// of the approximate estimator; any edit or new turn falls back to
	// ApproxTokens (see the measuredPromptTokens field comment).
	if v.measuredPromptTokens > 0 && v.input.Value() == v.measuredDraft {
		return v.measuredPromptTokens
	}
	return agent.ApproxTokens(v.payloadMessages())
}

// checkContextBudget flags the conversation as truncated when a send exceeds
// the input budget the runner enforces (three quarters of numCtx). The flag
// surfaces the truncation marker in the transcript head until /clear (M7-C:
// make the marker visible, not a silent wire-only drop).
func (v *AgentView) checkContextBudget() {
	limit := v.ctxLimit()
	if limit > 0 && v.ctxTokens() > limit {
		v.truncated = true
	}
}

// ctxLimit is the approximate input-token budget: three quarters of numCtx,
// the same bound BudgetMessages enforces (the model keeps the rest to reply).
func (v AgentView) ctxLimit() int {
	return v.numCtx * 3 / 4
}

// ctxPct is the budget percentage used, 0..100, computed from the same
// approximate tokens BudgetMessages counts.
func (v AgentView) ctxPct() int {
	limit := v.ctxLimit()
	if limit <= 0 {
		return 0
	}
	pct := v.ctxTokens() * 100 / limit
	if pct > 100 {
		pct = 100
	}
	return pct
}

// amberCtxPct is the amber context tier (N4): at or above 80% of the meter's
// existing displayed scale — the ¾-num_ctx input budget the runner enforces —
// usage warns amber before the red-100% tier. OWNER DECISION (PLAN §12 N4):
// the plan's “~80% of num_ctx” is read as 80% on the meter's displayed scale
// so the amber tier sits before (not beyond) today's red-100% tier.
const amberCtxPct = 80

// ctxTier classifies context usage for the meter's tier colors. Shared by the
// composer header and the shell status row so the tiers cannot drift.
type ctxTier int

const (
	ctxOK ctxTier = iota
	ctxAmber
	ctxRed
)

// ctxTierFor maps a meter percentage onto its tier: red at 100% (the budget
// is full — sending past it truncates the model's reply), amber from
// amberCtxPct, OK below.
func ctxTierFor(pct int) ctxTier {
	switch {
	case pct >= 100:
		return ctxRed
	case pct >= amberCtxPct:
		return ctxAmber
	default:
		return ctxOK
	}
}

// ctxMeterPlain renders the plain (unstyled) live context meter, e.g.
// "ctx ▓▓░░░ 38%", shown on the composer header's right side.
func (v AgentView) ctxMeterPlain() string {
	limit := v.ctxLimit()
	if limit <= 0 {
		return ""
	}
	pct := v.ctxPct()
	filled := (pct + 9) / 20
	if filled > 5 {
		filled = 5
	}
	bar := strings.Repeat("▓", filled) + strings.Repeat("░", 5-filled)
	return "ctx " + bar + " " + fmt.Sprintf("%d%%", pct)
}

// ctxMeterSegment renders the status row's compact context meter (N4), with
// its tier color: amber at the amber tier, red at 100%, the shell's plain
// muted style below. Empty when there is nothing to meter (no num_ctx, or a
// zero-token conversation) so a no-data frame stays byte-identical to the
// pre-N4 status row.
func (v AgentView) ctxMeterSegment() string {
	plain := v.ctxMeterPlain()
	if plain == "" || v.ctxTokens() == 0 {
		return ""
	}
	switch ctxTierFor(v.ctxPct()) {
	case ctxAmber:
		return v.styles.warnText().Render(plain)
	case ctxRed:
		return v.styles.Error.Render(plain)
	default:
		return plain
	}
}

// --- rendering ------------------------------------------------------------

// assistantHeader is the "who replied" chip shown at the left of each
// assistant block header line (streaming and committed).
func (v AgentView) assistantHeader(model string) string {
	if model == "" {
		model = "assistant"
	}
	return v.styles.agentAccent().Render("◈ " + model)
}

// assistantHeaderRow is a full-width header line: the model chip on the left
// and, when the turn finished, elapsed + reason meta right-aligned on the
// same row (opencode session headers carry time/status on the right). The
// pad uses the chat pane's inner width so meta always lands at the margin.
func (v AgentView) assistantHeaderRow(model, meta string) string {
	left := v.assistantHeader(model)
	if meta == "" {
		return left
	}
	inner := maxInt(v.w-2, 10)
	pad := inner - lipgloss.Width(left) - lipgloss.Width(meta)
	if pad < 1 {
		return left + "  " + v.styles.mutedText().Render(meta)
	}
	return left + strings.Repeat(" ", pad) + v.styles.mutedText().Render(meta)
}

// userHeader is the marker above each user block.
func (v AgentView) userHeader() string {
	return v.styles.agentAccent().Render("❯ you")
}

// turnFooter renders the per-turn meta (M7-B): elapsed wall time plus the
// terminal reason — the model's ollama done_reason ("stop"/"length") or
// "stopped" when the user cut the stream with esc — and, when the turn's
// final chunk carried generation metrics, the measured tok/s (N3). Returns
// "" when no start time was recorded (an assistant block committed without
// startChat), so hand-constructed transcripts in tests render no meta.
// Displayed on the assistant header's right side. tokPerSec <= 0 (metrics
// absent, zero duration, or a user stop) keeps the footer byte-identical to
// the pre-N3 shape.
func turnFooter(start time.Time, reason string, stopped bool, tokPerSec int) string {
	elapsed := ""
	if !start.IsZero() {
		elapsed = fmt.Sprintf("%.1fs", time.Since(start).Seconds())
	}
	if elapsed == "" {
		return ""
	}
	var meta string
	switch {
	case stopped:
		return elapsed + " · stopped"
	case reason != "":
		meta = elapsed + " · " + reason
	default:
		meta = elapsed
	}
	if tokPerSec > 0 {
		meta += fmt.Sprintf(" · %d tok/s", tokPerSec)
	}
	return meta
}

// turnTokPerSec converts a done event's final-chunk metrics into the footer's
// integer tok/s (eval_count / eval_duration, the duration in nanoseconds).
// 0 means no displayable rate: metrics absent or a zero duration.
func turnTokPerSec(m ollama.ChatMetrics) int {
	if m.Tokens <= 0 || m.Nanos <= 0 {
		return 0
	}
	return int(math.Round(float64(m.Tokens) * 1e9 / m.Nanos))
}

// chatRenderWidth buckets the chat pane's render width down to a multiple of
// five (N1 micro-item, PLAN §12 N7): resize jitter inside one bucket reuses
// the bucketed glamour renderer and the per-width caches instead of
// rebuilding and re-rendering on every wiggle. Rounding is down so a bucketed
// render never exceeds the real pane; panes narrower than one bucket keep
// their exact width.
func chatRenderWidth(pane int) int {
	pane = maxInt(pane, 1)
	if pane < 5 {
		return pane
	}
	return pane - pane%5
}

// ensureRenderer builds the glamour renderer when it is missing or the
// (bucketed) width changed. renderBlock calls it lazily so streaming renders
// never fail.
func (v *AgentView) ensureRenderer(width int) {
	width = maxInt(width, 1)
	if v.tr != nil && v.renderW == width {
		return
	}
	style := "dark"
	if !v.dark {
		style = "light"
	}
	tr, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		v.tr = nil
		return
	}
	v.tr = tr
	v.renderW = width
}

// rebuildRenderer (re)builds the renderer for the current (bucketed) width
// after a geometry change. A failed build keeps the previous renderer; View
// falls back to raw text so output is never silent.
func (v *AgentView) rebuildRenderer() {
	v.ensureRenderer(chatRenderWidth(v.w - 2))
}

// rebuildRenderCache re-renders every committed block at the current width.
// Only called on geometry changes (rare); renderBlock uses the shared cache
// every frame otherwise.
func (v *AgentView) rebuildRenderCache() {
	if v.renderW != chatRenderWidth(v.w-2) {
		return // renderer is stale; ensureRenderer on next renderBlock fixes it
	}
	for i := range v.turns {
		v.turns[i].render = v.renderBlock(v.headerFor(i), v.turns[i].msg.Content)
	}
}

// headerFor returns the role header line for a committed turn index.
// Assistant rows carry their model chip (and right-aligned meta when the
// turn recorded one); user rows keep the plain "❯ you" marker.
func (v AgentView) headerFor(i int) string {
	t := v.turns[i]
	if t.msg.Role == ollama.RoleAssistant {
		return v.assistantHeaderRow(t.model, t.meta)
	}
	return v.userHeader()
}

// renderBlock renders header + markdown content as one block. It never
// returns empty text for non-empty input: glamour failures fall back to the
// raw markdown so chat output is never silent (PLAN §6).
func (v AgentView) renderBlock(header, md string) string {
	if md == "" {
		if header == "" {
			return ""
		}
		return header
	}
	// H-05 boundary: chat content (streamed tokens and committed turns, from
	// either role) is the audit-cited leak site — glamour passes ESC payload
	// bytes through its styled output, and the raw-markdown fallback below
	// returns md verbatim. Sanitize before both so neither branch can carry
	// a hostile sequence into the terminal; user text is local but harmless
	// to strip here (display-only, idempotent).
	md = sanitizeTerminalText(md)
	v.ensureRenderer(chatRenderWidth(v.w - 2))
	if v.tr != nil {
		if out, err := v.tr.RenderBytes([]byte(md)); err == nil {
			return header + "\n" + strings.TrimSuffix(string(out), "\n")
		}
	}
	return header + "\n" + md
}

// chatLines assembles the full rendered transcript as individual display
// lines: an optional truncation marker (M7-C), the cached history blocks
// (header + markdown content + per-turn footer), plus the live streaming
// block with the streaming caret (M7-B). It is the O(total) convenience
// view over the same windowing machinery renderChatPane uses — production
// frames go through chatWindow so only the visible rows are materialized
// (N1); a test-side naive copy of this assembly pins the two paths equal.
func (v AgentView) chatLines() []string {
	return v.chatWindow(0, math.MaxInt)
}

// truncationMarkerLines returns the transcript-head marker shown when the
// runner silently omitted older turns to fit the budget (M7-C); empty when
// nothing was truncated.
func (v AgentView) truncationMarkerLines() []string {
	if !v.truncated {
		return nil
	}
	return []string{
		v.styles.mutedText().Render("… " + agent.TruncationNotice),
		"",
	}
}

// effectiveBlock returns the turn's display block: the cached per-width
// render, or the sanitized raw content when the cache entry is empty. The
// fallback covers a render gap (e.g. a hand-built transcript in tests); raw
// history must never reach the transcript unsanitized (H-05).
func (t turn) effectiveBlock() string {
	if t.render != "" {
		return t.render
	}
	return sanitizeTerminalText(t.msg.Content)
}

// turnBlock composes one committed turn's full display block (N6): its
// gated extras — the reasoning block behind /thinking, the tool-activity
// lines behind /details, in that order (reasoning precedes the tool calls,
// the tool calls precede the final answer) — prepended to the effective
// content block. With both toggles off or empty extras this is exactly
// effectiveBlock, so default frames are byte-identical to the pre-N6
// renderer by construction.
func (v AgentView) turnBlock(i int) string {
	t := v.turns[i]
	block := t.effectiveBlock()
	var extras []string
	if v.showThinking && t.thinking != "" {
		extras = append(extras, v.reasoningBlock(t.thinking))
	}
	if v.showDetails {
		for _, l := range t.tools {
			extras = append(extras, v.styles.mutedText().Render(l))
		}
	}
	if len(extras) == 0 {
		return block
	}
	if block == "" {
		return strings.Join(extras, "\n")
	}
	return strings.Join(extras, "\n") + "\n" + block
}

// turnDisplayLines returns the display lines one committed turn contributes
// to the transcript: its composed block split on newlines plus the trailing
// separator row. An empty block contributes nothing at all — not even the
// separator — so blank turns never leave a gap in the transcript.
func (v AgentView) turnDisplayLines(i int) []string {
	block := v.turnBlock(i)
	if block == "" {
		return nil
	}
	lines := strings.Split(block, "\n")
	return append(lines, "") // separator after each message
}

// turnLineCount is the row count turnDisplayLines would produce, computed by
// newline arithmetic on the composed block: a non-empty block splits into
// Count("\n")+1 lines plus the separator row. With the toggles off this
// composes nothing new (turnBlock returns the cached block directly), so
// chatLineCount walks a 2,000-turn transcript with this alone.
func (v AgentView) turnLineCount(i int) int {
	block := v.turnBlock(i)
	if block == "" {
		return 0
	}
	return strings.Count(block, "\n") + 2
}

// streamExtrasVisible reports whether the in-flight turn currently has
// gated extras to render (N6): live reasoning behind /thinking or tool
// activity behind /details. The content stream is not part of this.
func (v AgentView) streamExtrasVisible() bool {
	return (v.showThinking && v.thinkingText != "") || (v.showDetails && len(v.streamTools) > 0)
}

// streamDisplayLines renders the live streaming block: the gated extras
// (reasoning behind /thinking, tool lines behind /details), then header +
// content split on newlines, the streaming caret riding the last line while
// a turn streams (M7-B), and the trailing separator. Rendered only once the
// model is actually producing something — no phantom empty header/caret
// while a tool runs (that state lives on the statusline; N6 moves thinking
// behind the toggle instead of leaving it invisible).
func (v AgentView) streamDisplayLines() []string {
	if v.streamText == "" && !v.streamExtrasVisible() {
		return nil
	}
	var sb []string
	if v.showThinking && v.thinkingText != "" {
		sb = append(sb, strings.Split(v.reasoningBlock(v.thinkingText), "\n")...)
	}
	if v.showDetails {
		for _, l := range v.streamTools {
			sb = append(sb, v.styles.mutedText().Render(l))
		}
	}
	if v.streamText != "" {
		block := strings.Split(v.streamBlockRender(), "\n") // N2: cache-backed
		if v.streaming {
			block = withStreamingCaret(block)
		}
		sb = append(sb, block...)
	}
	return append(sb, "")
}

// streamLineCount is the row count streamDisplayLines would produce, by
// newline arithmetic on the same sources (no composition on the count path):
// the reasoning block is one marker row plus its body lines, tool lines
// count one each, and the content block is cache-backed. Zero when the turn
// contributes nothing.
func (v AgentView) streamLineCount() int {
	n := 0
	if v.showThinking && v.thinkingText != "" {
		n += 2 + strings.Count(v.thinkingText, "\n")
	}
	if v.showDetails {
		n += len(v.streamTools)
	}
	if v.streamText != "" {
		block := v.streamBlockRender()      // N2: cache-backed
		n += strings.Count(block, "\n") + 1 // split lines
		if v.streaming && strings.HasSuffix(block, "\n") {
			n++ // caret becomes its own row after the blank last line
		}
	}
	if n == 0 {
		return 0
	}
	return n + 1 // separator
}

// trailingBlankCount counts the blank lines chatLines drops from the tail of
// the full transcript. Blanks can only trail inside the last contributing
// source: every earlier source ends with its separator row immediately
// followed by the next source's header, and empty sources contribute
// nothing. Only the tail source is examined.
func (v AgentView) trailingBlankCount() int {
	if tail := v.streamDisplayLines(); len(tail) > 0 {
		// The stream source contributes: count its trailing blanks (the
		// separator row included — it is what the tail trim drops).
		n := 0
		for i := len(tail) - 1; i >= 0 && tail[i] == ""; i-- {
			n++
		}
		return n
	}
	for i := len(v.turns) - 1; i >= 0; i-- {
		block := v.turnBlock(i)
		if block == "" {
			continue
		}
		return strings.Count(block[len(strings.TrimRight(block, "\n")):], "\n") + 1
	}
	if v.truncated {
		return 1 // marker row + its separator; the marker itself is non-empty
	}
	return 0
}

// chatLineCount returns the total number of transcript display lines —
// exactly len(chatLines()) — without splitting any block: committed turns
// are counted by newline arithmetic (O(turns), zero allocs) and only the
// tail source is walked for the trailing-blank drop.
func (v AgentView) chatLineCount() int {
	total := len(v.truncationMarkerLines())
	for i := range v.turns {
		total += v.turnLineCount(i)
	}
	total += v.streamLineCount()
	if blanks := v.trailingBlankCount(); blanks > 0 {
		if blanks > total {
			blanks = total
		}
		total -= blanks
	}
	return total
}

// chatWindow materializes only the transcript display lines in [start, end)
// — the N1 render window. Committed turns are advanced by their line
// counts; only blocks overlapping the window are split and copied, so a
// tail window (the follow-mode common case) touches O(visible) rows no
// matter how long the session. Output is byte-identical to the O(total)
// assembly; a test-side naive copy pins that equivalence.
func (v AgentView) chatWindow(start, end int) []string {
	return v.chatWindowTotal(start, end, v.chatLineCount())
}

// chatWindowTotal is chatWindow with the transcript total supplied by the
// caller (renderChatPane already computed it for the scroll clamp; this
// avoids a second counting walk per frame).
func (v AgentView) chatWindowTotal(start, end, total int) []string {
	if end > total {
		end = total
	}
	if start < 0 {
		start = 0
	}
	if end <= start {
		return nil
	}
	var out []string
	pos := 0
	take := func(lines []string) {
		lo := start - pos
		if lo < 0 {
			lo = 0
		}
		hi := end - pos
		if hi > len(lines) {
			hi = len(lines)
		}
		if lo < hi {
			out = append(out, lines[lo:hi]...)
		}
		pos += len(lines)
	}
	if marker := v.truncationMarkerLines(); len(marker) > 0 {
		take(marker)
	}
	for i := range v.turns {
		if pos >= end {
			break
		}
		n := v.turnLineCount(i)
		if n == 0 {
			continue
		}
		if pos+n <= start {
			pos += n // entirely before the window: skip the split
			continue
		}
		take(v.turnDisplayLines(i))
	}
	if pos < end {
		take(v.streamDisplayLines())
	}
	// A window ending at the transcript tail must end where chatLines ends:
	// the trailing-blank drop.
	if end == total {
		for len(out) > 0 && out[len(out)-1] == "" {
			out = out[:len(out)-1]
		}
	}
	return out
}

// withStreamingCaret appends the streaming caret ("▍") to the live block. It
// rides the last visible line while a turn streams text and disappears the
// moment the turn commits and streaming goes false (M7-B).
func withStreamingCaret(lines []string) []string {
	if len(lines) == 0 {
		return []string{"▍"}
	}
	last := len(lines) - 1
	if strings.TrimSpace(lines[last]) != "" {
		lines[last] += "▍"
	} else {
		lines = append(lines, "▍")
	}
	return lines
}

// View renders transcript + hint + input per the current geometry. A modal
// (model selector, mutation approval, /help, /clear confirm) replaces the
// body with a centered overlay; the slash-command menu (M7-A) floats between
// the transcript and the input while a "/" draft is being composed.
func (v AgentView) View() string {
	bodyH := maxInt(v.h-2, 1)

	if v.confirmation != nil {
		return v.renderConfirmationOverlay(bodyH)
	}
	if v.selectorOpen {
		return v.renderSelectorOverlay(bodyH)
	}
	if v.resumeOpen {
		return v.renderResumeOverlay(bodyH)
	}
	if v.resumeConfirm {
		return v.renderResumeConfirmOverlay(bodyH)
	}
	if v.helpOpen {
		return v.renderHelpOverlay(bodyH)
	}
	if v.clearConfirm {
		return v.renderClearConfirmOverlay(bodyH)
	}

	if len(v.models) == 0 {
		return v.fullSizePane(bodyH)
	}

	// Bottom region (opencode footer anatomy): the composer pane (a header
	// row with model chip + context usage, then the auto-growing prompt),
	// below it a one-row statusline (spinner/status · interrupt, legend).
	// The slash menu or the @-file picker (N6) floats between the transcript
	// and the composer while its draft is being composed.
	menuH := 0
	switch {
	case v.fileMenuActive():
		menuH = v.fileMenuHeight()
	case v.slashMenu():
		menuH = v.slashMenuHeight()
	}
	composerH := v.composerRows() + 3
	chatH := maxInt(bodyH-composerH-1-menuH, 1)

	chatPane := v.renderChatPane(chatH)
	composer := v.renderComposer()
	status := v.statusLine()

	out := chatPane
	if menuH > 0 {
		menu := v.renderSlashMenu()
		if v.fileMenuActive() {
			menu = v.renderFileMenu()
		}
		out += "\n" + menu
	}
	return out + "\n" + composer + "\n" + status
}

// renderChatPane windows the transcript into the scroll region.
func (v AgentView) renderChatPane(h int) string {
	if h < 2 {
		return ""
	}
	// N1 windowing: count the transcript by newline arithmetic (no block
	// splits), then materialize only the visible rows. Rendering never
	// mutates state: the effective offset is computed locally. Follow
	// anchors to the tail (offset 0); otherwise the stored offset clamps to
	// the current content, so a shrink never shows past the head.
	total := v.chatLineCount()
	contentH := h - 2
	scroll := 0
	if !v.follow {
		scroll = clampInt(v.scroll, 0, maxInt(0, total-contentH))
	}

	// Window honors the scroll offset: scroll 0 shows the tail; scrolling
	// up shifts the window toward the head.
	end := total - scroll
	start := maxInt(0, end-contentH)
	window := v.chatWindowTotal(start, end, total)
	pane := v.styles.Pane.Width(v.w).Height(h)
	return pane.Render(lipgloss.JoinVertical(lipgloss.Left, window...))
}

// composerRows is the current prompt height in terminal rows (1..4).
func (v AgentView) composerRows() int {
	return composerRowsFor(v.input.Value(), maxInt(v.w-2, 10))
}

// renderComposer draws the composer block: a header row (model chip left,
// context meter + token usage right) above the growing prompt textarea, all
// inside one bordered pane (opencode footer anatomy).
func (v AgentView) renderComposer() string {
	innerW := maxInt(v.w-2, 10)
	ta := v.input
	rows := composerRowsFor(ta.Value(), innerW)
	ta.SetWidth(innerW)
	ta.SetHeight(rows)
	content := v.composerHeader() + "\n" + ta.View()
	return v.styles.Pane.Width(v.w).Height(rows + 3).Render(content)
}

// composerHeader is the composer's top row: model chip on the left, context
// usage (bar + percent + k-tokens) on the right — identity left, activity
// right, like opencode's footer.
func (v AgentView) composerHeader() string {
	innerW := maxInt(v.w-2, 10)
	left := v.assistantHeader(v.model)
	right := ""
	plain := v.ctxMeterPlain()
	if usage := v.ctxUsage(); usage != "" {
		plain += " · " + usage
	}
	// N4: tier colors via the shared ctxTierFor so the composer header and
	// the shell status row cannot drift apart.
	switch ctxTierFor(v.ctxPct()) {
	case ctxRed:
		right = v.styles.Error.Render("ctx full — /clear")
	case ctxAmber:
		right = v.styles.warnText().Render(plain)
	default:
		right = v.styles.mutedText().Render(plain)
	}
	pad := innerW - lipgloss.Width(left) - lipgloss.Width(right)
	if pad >= 1 {
		return left + strings.Repeat(" ", pad) + right
	}
	// Very narrow: keep the identity, drop the activity rather than wrap.
	room := innerW - lipgloss.Width(right) - 1
	if room > 0 {
		return truncateToWidth(left, room) + " " + right
	}
	return truncateToWidth(left, innerW)
}

// ctxUsage renders the approximate payload tokens as "1.2k/3.1k" against the
// input budget, mirroring opencode's token usage meta.
func (v AgentView) ctxUsage() string {
	limit := v.ctxLimit()
	if limit <= 0 {
		return ""
	}
	return fmt.Sprintf("%.1fk/%.1fk", float64(v.ctxTokens())/1000, float64(limit)/1000)
}

// composing reports whether the chat input holds text. The shell's digit-key
// tab jumps are disabled while composing so bare digits type into the prompt
// (M6 fix found by the reconnect smoke); with an empty input digits still
// switch tabs.
func (v AgentView) composing() bool {
	return v.input.Value() != ""
}

// statusLine is the one-row strip under the composer (opencode footer
// anatomy): an error, the running state with its interrupt hint, a transient
// notice, or the width-fitted key legend. Context usage lives in the
// composer header above, so this row stays a status/legend only.
func (v AgentView) statusLine() string {
	maxW := maxInt(v.w-2, 24)

	switch {
	case v.chatErr != "":
		return v.styles.Error.Render(truncateToWidth("⚠ "+v.chatErr+" — enter to retry", maxW))
	case v.streaming:
		left := "running…"
		if v.toolStatus != "" {
			left = firstLine(v.toolStatus)
		}
		right := "esc interrupt"
		if v.stopArmed {
			right = "esc again to interrupt"
		}
		return v.statusRow(maxW, []string{left}, right, v.stopArmed)
	case v.notice != "":
		return v.styles.Placeholder.Render(truncateToWidth(v.notice, maxW))
	case v.remoteHost():
		// Persistent remote-host warning (Phase 4): tools against a
		// non-loopback host can carry workspace content off the machine, so
		// the statusline says so until the user turns tools off or points at
		// a local host. It outranks the key legend on purpose.
		warn := "⚠ tools on — workspace content may be sent to " + v.host
		return v.styles.Error.Render(truncateToWidth(warn, maxW))
	}

	// Legend. While composing, the letter commands (m/r/u/d/f) are off; with
	// an empty input they are advertised (the interrupt/stop hint only shows
	// while running). The leading identity segment — tools state + canonical
	// workspace — is the one thing that survives width pressure (the legend
	// drops from the tail first), so the Agent view always answers "what can
	// this model touch?" even on a phone.
	if v.input.Value() != "" {
		return v.statusRow(maxW, []string{"enter send", "shift+enter newline", "esc clear draft"}, "", false)
	}
	identity := toolsChip(v.toolsEnabled)
	if v.workspace != "" {
		identity += " · " + v.workspace
	}
	segs := []string{identity, "enter send", "/ commands", "m model", "r refresh", "shift+enter newline"}
	return v.statusRow(maxW, segs, "", false)
}

// toolsChip is the stable tools-state label shared by the status bar and the
// Agent statusline.
func toolsChip(on bool) string {
	if on {
		return "tools on"
	}
	return "tools off"
}

// remoteHost reports whether tools are armed against a host that is not
// loopback — the only combination that can send workspace content off this
// machine. An unknown (empty) host never warns.
func (v AgentView) remoteHost() bool {
	return v.toolsEnabled && v.host != "" && !config.LoopbackHost(v.host)
}

// statusRow styles one statusline: Placeholder legend segments joined with
// " · " plus an optional right chip (warn=true renders it red). Segments
// drop from the tail until the row fits maxW so it never wraps on a phone.
func (v AgentView) statusRow(maxW int, segments []string, right string, warn bool) string {
	plain := strings.Join(segments, " · ")
	if right != "" {
		plain += " · " + right
	}
	for lipgloss.Width(plain) > maxW && len(segments) > 1 {
		segments = segments[:len(segments)-1]
		plain = strings.Join(segments, " · ")
		if right != "" {
			plain += " · " + right
		}
	}
	if lipgloss.Width(plain) > maxW {
		room := maxW
		if right != "" {
			room -= lipgloss.Width(right) + 3 // " · "
		}
		if room > 0 {
			segments[0] = truncateToWidth(segments[0], room)
		}
	}

	styled := v.styles.Placeholder.Render(strings.Join(segments, " · "))
	if right == "" {
		return styled
	}
	if warn {
		return styled + " · " + v.styles.Error.Render(right)
	}
	return styled + " · " + v.styles.mutedText().Render(right)
}

// truncateToWidth trims s to at most maxW visible columns and, when it had to
// cut, appends "…" so truncated text is visibly truncated (lipgloss MaxWidth
// alone cuts silently). ANSI sequences count as zero width and are never
// split mid-sequence, so styled text keeps its styling.
func truncateToWidth(s string, maxW int) string {
	if maxW < 1 {
		return ""
	}
	if lipgloss.Width(s) <= maxW {
		return s
	}
	runes := []rune(s)
	var b strings.Builder
	cols := 0
	limit := maxW - 1 // reserve the ellipsis column
	for i := 0; i < len(runes); {
		r := runes[i]
		if r == '\x1b' {
			// Copy the whole escape sequence without counting width.
			j := i + 1
			for j < len(runes) && !isAnsiFinal(runes[j]) {
				j++
			}
			if j < len(runes) {
				j++
			}
			b.WriteString(string(runes[i:j]))
			i = j
			continue
		}
		w := lipgloss.Width(string(r))
		if cols+w > limit {
			break
		}
		b.WriteRune(r)
		cols += w
		i++
	}
	return b.String() + "…"
}

// isAnsiFinal reports whether r can end an ANSI escape sequence (final bytes
// are letters; lipgloss only emits SGR sequences ending in 'm').
func isAnsiFinal(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// fullSizePane renders the loading / error / empty model-list states.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func (v AgentView) fullSizePane(bodyH int) string {
	pane := v.styles.Pane.Width(v.w).Height(maxInt(bodyH-2, 1))
	var content string
	switch {
	case v.loading:
		content = v.styles.Placeholder.Render("⏳ loading chat models…")
	case v.modelsErr != "":
		content = v.styles.Error.Render("⚠ "+v.modelsErr) + "\n" +
			v.styles.Placeholder.Render("press r to retry")
	default:
		content = v.styles.Placeholder.Render("no models installed") + "\n" +
			v.styles.Placeholder.Render("pull one from the Models tab (p), then press r")
	}
	return pane.Render(content)
}

// renderConfirmationOverlay asks for the one explicit approval required for
// every write, edit, and constrained command. It intentionally has no default
// affirmative key; only y or enter approves, and esc declines.
func (v AgentView) renderConfirmationOverlay(bodyH int) string {
	c := v.confirmation
	if c == nil {
		return ""
	}
	title := "Confirm mutation"
	if c.Name == "run_command" {
		title = "Confirm sandboxed command"
	}
	lines := []string{
		"Allow " + c.Name + "?",
		"workspace: " + c.Workspace,
		"timeout: " + c.Timeout.String(),
		"input: " + c.Input,
		"",
		"y / enter approve · n / esc decline",
	}
	return v.renderOverlayTitle(bodyH, title, lines)
}

// slashMenuMaxRows fits the whole command set (nine commands as of the
// N6 /details + /thinking additions) so the menu never needs its own
// scroll. Raised from 7: every command must stay reachable through the
// menu, and a ninth row still leaves the chat pane usable at the measured
// phone geometry (the pane shrinks by exactly one more row).
const slashMenuMaxRows = 9

// renderSelectorOverlay centers the model picker over the body. The picker
// filters as you type (any printable key extends the filter across name,
// family, size, quant), stars the config default model, and windows the list
// around the selection so long model lists stay readable on a phone (M7-C).
func (v AgentView) renderSelectorOverlay(bodyH int) string {
	list := v.filteredModels()
	maxRows := maxInt(bodyH-12, 3)

	lines := []string{"filter: " + v.selFilter}
	if len(list) == 0 {
		lines = append(lines, v.styles.Placeholder.Render("no model matches “"+v.selFilter+"”"))
	} else {
		start := clampInt(v.selIdx-maxRows/2, 0, maxInt(0, len(list)-maxRows))
		end := start + maxRows
		if end > len(list) {
			end = len(list)
		}
		if start > 0 {
			lines = append(lines, fmt.Sprintf("… %d earlier", start))
		}
		for i := start; i < end; i++ {
			m := list[i]
			marker := "  "
			name := m.Name
			if i == v.selIdx {
				marker = "❯ "
			}
			base := marker + name
			if m.Name == v.defaultModel {
				base += " ★"
			}
			// Row width is measured on plain text; styling never changes it.
			row := marker + name
			if m.Name == v.defaultModel {
				row += " " + v.styles.agentAccent().Render("★")
			}
			if i == v.selIdx {
				row = marker + lipgloss.NewStyle().Bold(true).Foreground(v.styles.accent).Render(name) + strings.TrimPrefix(row, marker+name)
			}
			if summary := modelSummary(m); summary != "" && lipgloss.Width(base)+2+lipgloss.Width(summary) <= maxInt(v.w-8, 20) {
				row += "  " + v.styles.Placeholder.Render(summary)
			}
			lines = append(lines, row)
		}
		if end < len(list) {
			lines = append(lines, fmt.Sprintf("… %d more", len(list)-end))
		}
	}
	lines = append(lines, "type to filter · ↑/↓ or j/k move · enter pick · esc close")

	return v.renderOverlayTitle(bodyH, "Model", lines)
}

// renderHelpOverlay is the /help reference: slash commands, keys, and the
// command palette. A compact list, not the onboarding flow (package D stays
// out of v0.1 scope).
func (v AgentView) renderHelpOverlay(bodyH int) string {
	lines := []string{
		"slash commands",
		" /clear    clear the conversation (asks first)",
		" /model    pick a model",
		" /resume   resume a saved chat transcript",
		" /theme    toggle dark/light for this session",
		" /details  toggle tool-output blocks",
		" /thinking toggle reasoning blocks",
		" /export   flush + reveal the transcript file path",
		" /help     show this reference",
		" /refresh  reload the model list",
		"",
		"composer",
		" enter send · shift+enter newline · esc clear draft",
		" the header row: model chip + live ctx usage",
		"",
		"running",
		" esc arms the interrupt · esc again cancels",
		"",
		"transcript (empty input)",
		" m model · r refresh · u/d scroll · f auto-follow",
		" pgup/pgdn page · digits switch tab",
		"",
		"any tab",
		" ctrl+p command palette · ctrl+c quit",
		"",
		"y / enter approve · n / esc decline",
	}
	return v.renderOverlayTitle(bodyH, "Help", lines)
}

// renderClearConfirmOverlay asks before /clear wipes the conversation (same
// guard rails as every destructive action: y/enter to confirm, esc to back
// out; nothing is cleared on a stray key).
func (v AgentView) renderClearConfirmOverlay(bodyH int) string {
	lines := []string{
		"Clear the conversation?",
		"",
		"y / enter clear · n / esc cancel",
	}
	return v.renderOverlayTitle(bodyH, "Clear conversation", lines)
}

// renderResumeOverlay is the /resume picker: one windowed row per saved
// transcript (newest first) with a humanized mtime + size summary, styled
// after the model picker and height-capped by the shared overlay helper.
func (v AgentView) renderResumeOverlay(bodyH int) string {
	list := v.resumeList
	maxRows := maxInt(bodyH-12, 3)

	lines := []string{"saved chats — enter to resume"}
	switch {
	case v.resumeLoading && len(list) == 0:
		lines = append(lines, v.styles.Placeholder.Render("loading…"))
	case len(list) == 0:
		lines = append(lines, v.styles.Placeholder.Render("no saved sessions"))
	default:
		start := clampInt(v.resumeIdx-maxRows/2, 0, maxInt(0, len(list)-maxRows))
		end := start + maxRows
		if end > len(list) {
			end = len(list)
		}
		if start > 0 {
			lines = append(lines, fmt.Sprintf("… %d earlier", start))
		}
		for i := start; i < end; i++ {
			s := list[i]
			marker := "  "
			if i == v.resumeIdx {
				marker = "❯ "
			}
			// UTC on purpose: the picker row must render identically on any
			// machine (golden fixtures pin it byte-for-byte).
			summary := s.ModTime.UTC().Format("2006-01-02 15:04") + " · " + humanSize(s.Size)
			row := marker + summary
			if i == v.resumeIdx {
				row = marker + lipgloss.NewStyle().Bold(true).Foreground(v.styles.accent).Render(summary)
			}
			lines = append(lines, row)
		}
		if end < len(list) {
			lines = append(lines, fmt.Sprintf("… %d more", len(list)-end))
		}
	}
	lines = append(lines, "↑/↓ or j/k move · enter resume · esc close")

	return v.renderOverlayTitle(bodyH, "Resume", lines)
}

// humanSize renders a file size the way picker rows show it ("312 B",
// "4.2 kB", "1.3 MB") — one decimal only above the byte range.
func humanSize(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f kB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}

// renderResumeConfirmOverlay asks before a resume replaces the live
// conversation (same guard rails as /clear: y/enter confirms, esc backs
// out; nothing is replaced on a stray key).
func (v AgentView) renderResumeConfirmOverlay(bodyH int) string {
	name := v.resumePick
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	lines := []string{
		"Resume over the current conversation?",
		name,
		"",
		"y / enter resume · n / esc cancel",
	}
	return v.renderOverlayTitle(bodyH, "Resume conversation", lines)
}

// slashMenuHeight is the bordered menu's row budget while a slash draft is
// showing (rows + box borders), capped so the chat pane keeps room on a phone.
func (v AgentView) slashMenuHeight() int {
	rows := len(v.slashMatches())
	if rows > slashMenuMaxRows {
		rows = slashMenuMaxRows
	}
	return rows + 2
}

// renderSlashMenu draws the bordered "/" command menu between the transcript
// and the input. The highlighted row is bold/accent; the filter is the draft
// itself (typing narrows the menu live — M7-A).
func (v AgentView) renderSlashMenu() string {
	matches := v.slashMatches()
	if len(matches) == 0 {
		return ""
	}
	rows := len(matches)
	if rows > slashMenuMaxRows {
		rows = slashMenuMaxRows
	}
	innerW := maxInt(v.w-2, 16)

	out := make([]string, 0, rows)
	for i, c := range matches[:rows] {
		marker := "  "
		name := "/" + c.name
		if i == v.slashIdx {
			marker = "❯ "
			name = lipgloss.NewStyle().Bold(true).Foreground(v.styles.accent).Render("/" + c.name)
		}
		prefix := marker + name
		desc := c.desc
		if room := innerW - lipgloss.Width(prefix) - 2; room > 0 {
			desc = truncateToWidth(c.desc, room)
			prefix += "  " + v.styles.Placeholder.Render(desc)
		} else {
			prefix = truncateToWidth(prefix, innerW)
		}
		out = append(out, prefix)
	}
	return v.styles.Pane.Width(v.w).Height(rows + 2).Render(strings.Join(out, "\n"))
}

// renderOverlayTitle centers a bordered dialog over the whole Agent body via
// the shared overlay helper (fitContent keeps every decision row on screen at
// the measured phone geometry).
func (v AgentView) renderOverlayTitle(bodyH int, title string, lines []string) string {
	return renderCenteredOverlay(v.w, bodyH, v.styles, title, lines)
}

// clampScroll bounds the stored scroll offset by the currently visible window
// (composer height is dynamic — same accounting as renderChatPane). It runs
// on the key paths that change scroll; rendering computes its effective
// offset locally and never writes state.
func (v *AgentView) clampScroll() {
	bodyH := maxInt(v.h-2, 1)
	chatH := maxInt(bodyH-v.composerRows()-3-1, 1)
	lines := v.chatLineCount()
	maxScroll := maxInt(0, lines-maxInt(chatH-2, 1))
	if v.scroll > maxScroll {
		v.scroll = maxScroll
	}
	if v.scroll < 0 {
		v.scroll = 0
	}
}

// warnText returns the amber foreground style for the context meter's amber
// tier (N4) — a “getting close” warning between the muted idle bar and the
// red budget-full state.
func (s Styles) warnText() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(s.warn)
}

// agentAccent returns the accent style applied to role headers.
func (s Styles) agentAccent() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(s.accent)
}

// mutedText returns the muted foreground style for secondary rows (transcript
// footers, the context meter's idle bar) — plain, not the italic Placeholder.
func (s Styles) mutedText() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(s.muted)
}

// --- M4 live apply: re-theme + config apply -------------------------------

// applyTheme re-tints the Agent tab and rebuilds the glamour renderer at the
// new dark/light state so committed blocks re-render in the active theme.
func (v AgentView) applyTheme(dark bool, styles Styles) AgentView {
	v.styles = styles
	v.dark = dark
	v.rebuildRenderer()
	v.rebuildRenderCache()
	v.primeStreamRender() // N2: active block re-tints with the shell theme
	return v
}

// ApplyConfig applies a successful settings save in-session: scalar chat
// parameters, the default model, the workspace tool trust switch, and the
// host take effect for the next send, and the runner is rebuilt so a new
// workspace root, system prompt, iteration cap, tools state, and client
// apply. An in-flight turn keeps the runner it started with (startChat copies
// the pointer before the goroutine runs), so swapping here is safe
// mid-stream. reload=true (host/token change) clears the selector models and
// refetches from the new host.
func (v AgentView) ApplyConfig(cfg config.Config, c *ollama.Client, reload bool) (AgentView, tea.Cmd) {
	v.client = c
	v.defaultModel = cfg.DefaultModel
	v.temperature = cfg.Agent.Temperature
	v.topP = cfg.Agent.TopP
	v.numCtx = cfg.Agent.NumCtx
	v.systemPrompt = cfg.Agent.SystemPrompt
	root := cfg.WorkspaceRoot
	if root == "" {
		root, _ = os.Getwd()
	}
	v.toolsEnabled = cfg.ToolsEnabled
	v.host = cfg.Host
	v.workspace = canonicalWorkspaceLabel(root)
	v.runner = runnerFor(c, root, cfg.Agent.SystemPrompt, cfg.Agent.MaxToolIterations, cfg.ToolsEnabled)
	if v.logger != nil {
		v.runner = v.runner.WithLogger(v.logger)
	}
	if !reload {
		return v, nil
	}
	// A host/token change invalidates every in-flight model-list result of
	// the old client: bump the generation so an obsolete completion is
	// dropped on arrival (M-03), then refetch from the new host.
	v.clientGen++
	v.loading = true
	v.modelsErr = ""
	v.models = nil
	v.model = ""
	v.selIdx = 0
	return v, v.loadModelsCmd()
}

package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"selftui/internal/agent"
	"selftui/internal/config"
	"selftui/internal/ollama"
)

// uiTagsBody lists the two sample models as the fake host would.
const uiTagsBody = `{
  "models": [
    {
      "name": "qwen3:8b",
      "modified_at": "2026-09-03T08:00:00Z",
      "size": 5150000000,
      "details": {"family": "qwen3", "parameter_size": "8.2B", "quantization_level": "Q4_K_M"}
    },
    {
      "name": "gemma3:12b",
      "modified_at": "2026-08-01T01:02:03Z",
      "size": 8123456789,
      "details": {"family": "gemma3", "parameter_size": "12B", "quantization_level": "Q4_K_M"}
    }
  ]
}`

// hand-written JSON for the streaming body would invite escaping bugs, so the
// fake host builds its events properly with json.Marshal instead.
func chatEvent(content string, done bool) string {
	b, _ := json.Marshal(map[string]any{
		"message": map[string]string{"role": "assistant", "content": content},
		"done":    done,
	})
	return string(b)
}

// uiChatBody streams a markdown answer: a heading, bold text, and a code
// block (with a syntax-highlighted line), then done.
var uiChatBody = chatEvent("# Answer\n\nHere is **code**:\n\n```go\nfunc main(){ println(\"hi\") }\n```\n", false) + "\n" +
	chatEvent("", true) + "\n"

// fakeOllamaUI serves /api/tags (uiTagsBody) and /api/chat (uiChatBody),
// recording every chat request's messages.
func fakeOllamaUI(t *testing.T) (*ollama.Client, *[][]ollama.ChatMessage, *int) {
	t.Helper()
	chats := &[][]ollama.ChatMessage{}
	chatCalls := new(int)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, uiTagsBody)
		case "/api/chat":
			*chatCalls++
			var req ollama.ChatRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode chat request: %v", err)
			}
			*chats = append(*chats, req.Messages)
			w.Header().Set("Content-Type", "application/x-ndjson")
			io.WriteString(w, uiChatBody)
		default:
			// Stray traffic (this host's localhost port prober sends GET / at
			// fresh ports): silent 404, never a test error — a genuine client
			// mistake still fails through the client's own error.
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return ollama.New(srv.URL, ""), chats, chatCalls
}

// testAgent builds an Agent tab at 88x40 with local state (no server).
func testAgent(t *testing.T, client *ollama.Client) AgentView {
	t.Helper()
	if client == nil {
		client = ollama.New("http://localhost:1", "")
	}
	cfg := config.Default()
	v := NewAgentView(client, NewStyles("dark"), "dark", "", cfg.Agent)
	v, _ = v.Update(tea.WindowSizeMsg{Width: 88, Height: 40})
	return v
}

// typeText feeds each rune as a key press into the tab's textarea.
func typeText(t *testing.T, v *AgentView, s string) {
	t.Helper()
	for _, r := range s {
		*v, _ = v.Update(tea.KeyPressMsg{Text: string(r)})
	}
}

// drainChat pumps the activity channel until the turn finishes.
func drainChat(t *testing.T, v *AgentView) {
	t.Helper()
	steps := 0
	for v.streaming {
		select {
		case msg := <-v.chatCh:
			steps++
			if steps > 50 {
				t.Fatal("chat stream did not finish")
			}
			*v, _ = v.Update(msg)
		case <-time.After(3 * time.Second):
			t.Fatal("chat stream stalled")
		}
	}
}

func TestAgentViewReadOnlyToolLoop(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("agent note\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, uiTagsBody)
		case "/api/chat":
			calls++
			w.Header().Set("Content-Type", "application/x-ndjson")
			if calls == 1 {
				io.WriteString(w, `{"message":{"role":"assistant","tool_calls":[{"function":{"name":"read_file","arguments":{"path":"note.txt"}}}]},"done":true}`+"\n")
			} else {
				io.WriteString(w, chatEvent("read-only answer", true)+"\n")
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	v := newAgentTools(t, ollama.New(srv.URL, ""), srv.URL, root)
	v, _ = v.Update(tea.WindowSizeMsg{Width: 88, Height: 40})
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	typeText(t, &v, "read note")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	drainChat(t, &v)
	if calls != 2 || len(v.turns) != 2 {
		t.Fatalf("chat calls=%d turns=%d, want tool turn plus final", calls, len(v.turns))
	}
	if !strings.Contains(v.turns[1].msg.Content, "read-only answer") {
		t.Errorf("assistant turn = %+v", v.turns[1])
	}
	out := stripANSI(v.View())
	if !strings.Contains(out, "read-only answer") {
		t.Errorf("final answer missing from view:\n%s", out)
	}
}

func TestAgentViewDeclinesMutationConfirmation(t *testing.T) {
	root := t.TempDir()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, uiTagsBody)
		case "/api/chat":
			calls++
			w.Header().Set("Content-Type", "application/x-ndjson")
			if calls == 1 {
				io.WriteString(w, `{"message":{"role":"assistant","tool_calls":[{"function":{"name":"write_file","arguments":{"path":"no.txt","content":"no"}}}]},"done":true}`+"\n")
			} else {
				io.WriteString(w, chatEvent("declined safely", true)+"\n")
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	v := newAgentTools(t, ollama.New(srv.URL, ""), srv.URL, root)
	v, _ = v.Update(tea.WindowSizeMsg{Width: 88, Height: 40})
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	typeText(t, &v, "write no")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	deadline := time.After(3 * time.Second)
	for !v.ModalOpen() {
		select {
		case msg := <-v.chatCh:
			v, _ = v.Update(msg)
		case <-deadline:
			t.Fatal("tool confirmation did not arrive")
		}
	}
	if !strings.Contains(stripANSI(v.View()), "Confirm mutation") {
		t.Fatalf("confirmation modal missing: %s", stripANSI(v.View()))
	}
	v, _ = v.Update(tea.KeyPressMsg{Text: "n"})
	drainChat(t, &v)
	if _, err := os.Stat(filepath.Join(root, "no.txt")); !os.IsNotExist(err) {
		t.Fatalf("declined write created file: %v", err)
	}
	if calls != 2 || !strings.Contains(v.turns[1].msg.Content, "declined safely") {
		t.Fatalf("calls=%d turns=%+v", calls, v.turns)
	}
}

func TestAgentViewLoadsModels(t *testing.T) {
	v := testAgent(t, nil)
	if v.model != "" {
		t.Errorf("model before load = %q, want empty", v.model)
	}
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	if v.model != "qwen3:8b" {
		t.Errorf("model = %q, want first list entry qwen3:8b", v.model)
	}
	out := stripANSI(v.View())
	if !strings.Contains(out, "enter send") {
		t.Errorf("legend missing:\n%s", out)
	}
}

func TestAgentViewDefaultModelPreferred(t *testing.T) {
	v := testAgent(t, nil)
	v.defaultModel = "gemma3:12b"
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	if v.model != "gemma3:12b" {
		t.Errorf("model = %q, want config default gemma3:12b", v.model)
	}
}

func TestAgentViewDefaultModelMissingFallsBack(t *testing.T) {
	v := testAgent(t, nil)
	v.defaultModel = "ghost:8b" // not installed
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	if v.model != "qwen3:8b" {
		t.Errorf("model = %q, want first entry when default is absent", v.model)
	}
}

func TestAgentViewNoModelsHint(t *testing.T) {
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: nil})
	if v.model != "" {
		t.Errorf("model = %q, want empty with no models", v.model)
	}
	out := stripANSI(v.View())
	for _, want := range []string{"no models installed", "Models tab"} {
		if !strings.Contains(out, want) {
			t.Errorf("empty state missing %q:\n%s", want, out)
		}
	}
}

func TestAgentViewSendStreamsAndCommits(t *testing.T) {
	client, chats, _ := fakeOllamaUI(t)
	v := testAgent(t, client)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})

	typeText(t, &v, "hello")
	if got := v.input.Value(); got != "hello" {
		t.Fatalf("input value = %q, want hello", got)
	}

	// enter sends and subscribes to the stream.
	v, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !v.streaming {
		t.Fatal("enter: streaming should start")
	}
	if cmd == nil {
		t.Fatal("enter: expected subscription command")
	}
	drainChat(t, &v)

	if n := len(v.turns); n != 2 {
		t.Fatalf("history len = %d, want 2 (user+assistant)", n)
	}
	if v.turns[0].msg.Role != ollama.RoleUser || v.turns[0].msg.Content != "hello" {
		t.Errorf("turns[0] = %+v, want user hello", v.turns[0])
	}
	assistant := v.turns[1].msg
	if assistant.Role != ollama.RoleAssistant || !strings.Contains(assistant.Content, "func main()") {
		t.Errorf("history[1] = %+v, want committed assistant answer", assistant)
	}
	if v.chatErr != "" || v.streaming {
		t.Errorf("state after done: err=%q streaming=%v", v.chatErr, v.streaming)
	}

	out := stripANSI(v.View())
	for _, want := range []string{"❯ you", "hello", "Answer", "func main()", "println"} {
		if !strings.Contains(out, want) {
			t.Errorf("transcript missing %q:\n%s", want, out)
		}
	}

	if got := (*chats)[0]; len(got) != 1 || got[0].Role != ollama.RoleUser || got[0].Content != "hello" {
		t.Errorf("sent messages = %+v, want [user hello]", got)
	}
}

func TestAgentViewStreamingRendersLive(t *testing.T) {
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	typeText(t, &v, "hi")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !v.streaming {
		t.Fatal("enter: streaming should start")
	}

	// Feed partial tokens directly; the transcript must show them live.
	for _, seq := range []string{"Hel", "lo ", "wor", "ld"} {
		var cmd tea.Cmd
		v, cmd = v.Update(agent.TokenMsg{Text: seq})
		if cmd == nil {
			t.Fatal("token: expected resubscribed command")
		}
	}
	// N2: deltas batch to the repaint tick — queued, not yet on screen.
	if v.streamText != "" || v.pendingStream != "Hello world" {
		t.Errorf("batching: streamText=%q pendingStream=%q, want deltas queued for the tick", v.streamText, v.pendingStream)
	}
	tickStream(t, &v)
	out := stripANSI(v.View())
	if !strings.Contains(out, "Hello world") {
		t.Errorf("live transcript missing streamed text:\n%s", out)
	}
	if !v.follow {
		t.Error("follow should stay true while streaming")
	}
}

func TestAgentViewEmptyInputIgnored(t *testing.T) {
	client, chats, _ := fakeOllamaUI(t)
	v := testAgent(t, client)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})

	typeText(t, &v, "   ")
	v, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if v.streaming || cmd != nil {
		t.Error("whitespace input should not start a stream")
	}
	if len(*chats) != 0 {
		t.Errorf("server saw %d chats, want 0", len(*chats))
	}
}

func TestAgentViewShiftEnterInsertsNewline(t *testing.T) {
	client, chats, _ := fakeOllamaUI(t)
	v := testAgent(t, client)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})

	typeText(t, &v, "line one")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	if !strings.Contains(v.input.Value(), "\n") {
		t.Errorf("shift+enter should insert a newline, got %q", v.input.Value())
	}
	if v.streaming || len(*chats) != 0 {
		t.Error("shift+enter must not send")
	}
}

func TestAgentViewEnterWhileStreamingIgnored(t *testing.T) {
	// A server that streams one delta then stalls until the client leaves.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":"one"},"done":false}`)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	client := ollama.New(srv.URL, "")
	v := testAgent(t, client)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})

	typeText(t, &v, "first")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !v.streaming {
		t.Fatal("enter: streaming should start")
	}

	// Wait for the first delta so the request provably reached the server
	// (the goroutine only runs once the test blocks on the channel).
	select {
	case msg := <-v.chatCh:
		v, _ = v.Update(msg)
	case <-time.After(3 * time.Second):
		t.Fatal("first token never arrived")
	}

	// A second enter while streaming is swallowed (no second request queued).
	v, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Errorf("enter while streaming returned command %v, want nil", cmd)
	}

	// esc is an armed interrupt: the first press only arms (the statusline
	// flips to "esc again to interrupt"), the second cancels. The partial
	// content is committed, no error.
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if !v.stopArmed || v.stopRequest {
		t.Errorf("first esc should arm the interrupt (armed=%v request=%v)", v.stopArmed, v.stopRequest)
	}
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if !v.stopRequest {
		t.Error("second esc should cancel the stream")
	}
	drainChat(t, &v)
	if v.chatErr != "" {
		t.Errorf("chatErr = %q, want empty (user stop)", v.chatErr)
	}
	if v.notice != "stopped" {
		t.Errorf("notice = %q, want stopped", v.notice)
	}
	if len(v.turns) != 2 {
		t.Errorf("turns len = %d, want 2 (partial committed)", len(v.turns))
	}
}

func TestAgentViewEscCancelsStream(t *testing.T) {
	client, chats, _ := fakeOllamaUI(t)
	v := testAgent(t, client)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
	typeText(t, &v, "stop me")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !v.streaming {
		t.Fatal("enter: streaming should start")
	}

	// Wait for the first delta (proves the request reached the server), then
	// esc stops the stream.
	select {
	case msg := <-v.chatCh:
		var cmd tea.Cmd
		v, cmd = v.Update(msg)
		tickStream(t, &v) // N2: the delta renders at the repaint tick
		if !strings.Contains(v.streamText, "# Answer") {
			t.Errorf("first delta = %q, want Answer content", v.streamText)
		}
		if cmd == nil {
			t.Fatal("token: expected resubscribed command")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("first token never arrived")
	}
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if !v.stopArmed {
		t.Error("first esc should arm the interrupt")
	}
	if v.stopRequest {
		t.Error("armed esc must not cancel yet")
	}
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if !v.stopRequest {
		t.Error("second esc should mark the stop request")
	}
	if v.stopCancel == nil {
		t.Fatal("stopCancel missing during stream")
	}
	drainChat(t, &v)

	// A stop is not an error: the partial text stays, no chatErr.
	if v.chatErr != "" {
		t.Errorf("chatErr = %q, want empty (user-initiated stop)", v.chatErr)
	}
	if v.notice != "stopped" {
		t.Errorf("notice = %q, want stopped", v.notice)
	}
	if got := v.turns[len(v.turns)-1].msg.Content; !strings.Contains(got, "Answer") {
		t.Errorf("partial content not committed: %q", got)
	}
	if len(*chats) != 1 {
		t.Errorf("server saw %d chats, want 1", len(*chats))
	}
}

func TestAgentViewChatErrorSurfaced(t *testing.T) {
	// A host that rejects the model with a 404 + Ollama error payload.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, uiTagsBody)
		case "/api/chat":
			w.WriteHeader(404)
			io.WriteString(w, `{"error":"model 'ghost:tag' not found"}`)
		}
	}))
	t.Cleanup(srv.Close)
	client := ollama.New(srv.URL, "")
	v := testAgent(t, client)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})

	typeText(t, &v, "ping")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	drainChat(t, &v)

	if v.streaming || v.chatErr == "" {
		t.Fatalf("streaming=%v err=%q, want finished with an error", v.streaming, v.chatErr)
	}
	if !strings.Contains(v.chatErr, "model 'ghost:tag' not found") {
		t.Errorf("chatErr = %q, want server message", v.chatErr)
	}
	out := stripANSI(v.View())
	if !strings.Contains(out, "not found") || !strings.Contains(out, "retry") {
		t.Errorf("error not surfaced inline:\n%s", out)
	}
	// The user message stays in history so enter retries it.
	if n := len(v.turns); n != 1 || v.turns[0].msg.Content != "ping" {
		t.Errorf("turns = %+v, want the user message kept for retry", v.turns)
	}
}

func TestAgentViewMultiTurnHistory(t *testing.T) {
	client, chats, _ := fakeOllamaUI(t)
	v := testAgent(t, client)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})

	typeText(t, &v, "first")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	drainChat(t, &v)

	typeText(t, &v, "second")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	drainChat(t, &v)

	got := *chats
	if len(got) != 2 {
		t.Fatalf("server saw %d chats, want 2", len(got))
	}
	if len(got[0]) != 1 {
		t.Errorf("turn 1 messages = %+v, want only the first user message", got[0])
	}
	if len(got[1]) != 3 {
		t.Errorf("turn 2 messages = %+v, want full history (user, assistant, user)", got[1])
	}
	if got[1][2].Role != ollama.RoleUser || got[1][2].Content != "second" {
		t.Errorf("turn 2 last = %+v, want the second user message", got[1][2])
	}
}

func TestAgentViewModelSelector(t *testing.T) {
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})

	v, cmd := v.Update(tea.KeyPressMsg{Text: "m"})
	if !v.selectorOpen || !v.ModalOpen() {
		t.Fatal("m should open the selector")
	}
	if cmd != nil {
		t.Errorf("m returned command %v, want nil", cmd)
	}
	out := stripANSI(v.View())
	for _, want := range []string{"Model", "qwen3:8b", "gemma3:12b", "❯"} {
		if !strings.Contains(out, want) {
			t.Errorf("selector missing %q:\n%s", want, out)
		}
	}

	// j moves down, enter selects gemma3:12b.
	v, _ = v.Update(tea.KeyPressMsg{Text: "j"})
	if v.selIdx != 1 {
		t.Errorf("selIdx = %d, want 1 after j", v.selIdx)
	}
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if v.selectorOpen {
		t.Error("enter should close the selector")
	}
	if v.model != "gemma3:12b" {
		t.Errorf("model = %q, want gemma3:12b", v.model)
	}
	if v.notice != "model gemma3:12b" {
		t.Errorf("notice = %q, want model gemma3:12b", v.notice)
	}

	// esc cancels without changing the model.
	v, _ = v.Update(tea.KeyPressMsg{Text: "m"})
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if v.selectorOpen || v.model != "gemma3:12b" {
		t.Errorf("esc should close the selector and keep the model (open=%v model=%q)", v.selectorOpen, v.model)
	}
}

func TestAgentViewSelectorNoModels(t *testing.T) {
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: nil})
	v, cmd := v.Update(tea.KeyPressMsg{Text: "m"})
	if v.selectorOpen {
		t.Error("selector should not open with no models")
	}
	if cmd != nil || v.notice == "" {
		t.Errorf("expected a notice, cmd=%v notice=%q", cmd, v.notice)
	}
}

func TestAgentViewSwitchModelAppliesNextSend(t *testing.T) {
	client, _, _ := fakeOllamaUI(t)
	v := testAgent(t, client)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})

	// Switch to gemma3:12b, then chat.
	v, _ = v.Update(tea.KeyPressMsg{Text: "m"})
	v, _ = v.Update(tea.KeyPressMsg{Text: "j"})
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	typeText(t, &v, "hi gemma")
	v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	drainChat(t, &v)

	// The committed assistant turn is attributed to gemma3:12b.
	if got := v.turns[len(v.turns)-1].model; got != "gemma3:12b" {
		t.Errorf("turn model = %q, want gemma3:12b", got)
	}
}

func TestAgentViewScrollAndResize(t *testing.T) {
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})

	// Commit a turn whose answer is a 40-line code block — markdown joins
	// plain paragraphs, so a fenced block is the way to overflow the pane.
	var body strings.Builder
	body.WriteString("```\n")
	for i := 1; i <= 40; i++ {
		fmt.Fprintf(&body, "line %02d\n", i)
	}
	body.WriteString("```\n")
	answer := body.String()

	v.turns = []turn{
		{msg: ollama.ChatMessage{Role: ollama.RoleUser, Content: "make a long answer"},
			render: v.renderBlock(v.userHeader(), "make a long answer")},
		{msg: ollama.ChatMessage{Role: ollama.RoleAssistant, Content: answer}, model: "qwen3:8b",
			render: v.renderBlock(v.assistantHeader("qwen3:8b"), answer)},
	}
	v.follow = true

	// The tail is visible at first (follow), u scrolls up away from it.
	out := stripANSI(v.View())
	if !strings.Contains(out, "line 40") {
		t.Errorf("tail not visible at start:\n%s", out)
	}
	v, _ = v.Update(tea.KeyPressMsg{Text: "u"})
	if v.follow {
		t.Error("manual scroll should leave follow mode")
	}
	if v.scroll <= 0 {
		t.Errorf("scroll = %d, want > 0 after u", v.scroll)
	}

	// A resize re-renders cached blocks at the new width; the tail survives.
	v, _ = v.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	out = stripANSI(v.View())
	if !strings.Contains(out, "line 40") {
		t.Errorf("tail missing after resize:\n%s", out)
	}

	// Scrolling all the way up surfaces the head of the conversation.
	for i := 0; i < 60 && !strings.Contains(stripANSI(v.View()), "line 01"); i++ {
		v, _ = v.Update(tea.KeyPressMsg{Text: "u"})
	}
	out = stripANSI(v.View())
	if !strings.Contains(out, "line 01") {
		t.Errorf("head not reachable by scrolling:\n%s", out)
	}

	// Compact geometry renders (no panic).
	v, _ = v.Update(tea.WindowSizeMsg{Width: 72, Height: 30})
	if stripANSI(v.View()) == "" {
		t.Error("compact render came back empty")
	}
}

func TestAgentViewRefreshReloadsModels(t *testing.T) {
	client, _, chatCalls := fakeOllamaUI(t)
	v := testAgent(t, client)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})

	tagCount := 0
	// Point at a server that counts tag fetches.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tagCount++
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, uiTagsBody)
	}))
	t.Cleanup(srv.Close)
	v.client = ollama.New(srv.URL, "")

	v, cmd := v.Update(tea.KeyPressMsg{Text: "r"})
	if cmd == nil {
		t.Fatal("r: expected refresh command")
	}
	cmd() // run the fetch
	if tagCount != 1 {
		t.Errorf("tag fetches = %d, want 1", tagCount)
	}
	_ = chatCalls
}

func TestAgentViewModalBlocksTabJump(t *testing.T) {
	client, _, _ := fakeOllamaUI(t)
	cfg := config.Default()
	m := New(&cfg, NewStyles("dark"), client)
	am, _ := m.Update(tea.WindowSizeMsg{Width: 88, Height: 40})
	m = am.(App)

	// Load the agent model list through the root, then open the Agent tab.
	m = updateTab(t, m, agentEventMsg{msg: agentModelsLoadedMsg{models: sampleModels()}})
	m = updateTab(t, m, tea.KeyPressMsg{Text: "2"})
	if m.tab != 1 {
		t.Fatalf("tab = %d, want 1", m.tab)
	}

	// Open the selector; the 1/2/3 tab jumps must be swallowed.
	m = updateTab(t, m, tea.KeyPressMsg{Text: "m"})
	if !m.agent.ModalOpen() {
		t.Fatal("m should open the selector")
	}
	m = updateTab(t, m, tea.KeyPressMsg{Text: "3"})
	if m.tab != 1 {
		t.Errorf("tab = %d with selector open, want 1 (modal guard)", m.tab)
	}

	// Closing the selector restores tab jumps.
	m = updateTab(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updateTab(t, m, tea.KeyPressMsg{Text: "3"})
	if m.tab != 2 {
		t.Errorf("tab = %d after esc, want 2", m.tab)
	}
}

func TestAgentViewRendersAtBothGeometries(t *testing.T) {
	client, _, _ := fakeOllamaUI(t)
	for _, w := range []int{120, 72} {
		h := 30
		if w == 120 {
			h = 40
		}
		v := testAgent(t, client)
		v, _ = v.Update(tea.WindowSizeMsg{Width: w, Height: h})
		v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})
		typeText(t, &v, "hi")
		v, _ = v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		for v.streaming {
			select {
			case msg := <-v.chatCh:
				v, _ = v.Update(msg)
			case <-time.After(3 * time.Second):
				t.Fatalf("width %d: chat stalled", w)
			}
		}
		out := stripANSI(v.View())
		if !strings.Contains(out, "func main()") {
			t.Errorf("width %d: chat output missing code block:\n%s", w, out)
		}
		if v.h != h || v.w != w {
			t.Errorf("width %d: geometry %dx%d not stored", w, v.w, v.h)
		}
	}
}

// TestAgentViewReloadRejectsObsoleteModelList reproduces M-03's agent-side
// load shape: the Agent view starts a model-list fetch against the old host
// (Init, r, or a picker refresh) and the user then saves a new host in
// Settings, which reloads through ApplyConfig. The old host's late result
// must not replace the new host's list or the chat model chosen from it.
func TestAgentViewReloadRejectsObsoleteModelList(t *testing.T) {
	oldClient, _ := tagsServer(t, "old-agent-model")
	newClient, _ := tagsServer(t, "new-agent-model")

	v := testAgent(t, oldClient)

	// An old-host model-list fetch completes before the host is swapped; its
	// result is captured (it was "in flight" across ApplyConfig).
	staleEv := v.loadModelsCmd()()
	staleLoaded, ok := staleEv.(agentEventMsg).msg.(agentModelsLoadedMsg)
	if !ok || len(staleLoaded.models) != 1 || staleLoaded.models[0].Name != "old-agent-model" {
		t.Fatalf("captured stale agent load = %#v", staleLoaded)
	}

	// The user saves a new host: ApplyConfig(reload=true) refetches from the
	// new client.
	cfg := config.Default()
	v, cmd := v.ApplyConfig(cfg, newClient, true)
	if cmd == nil {
		t.Fatal("ApplyConfig(reload): expected a model-list command")
	}
	freshEv := cmd()
	freshLoaded, ok := freshEv.(agentEventMsg).msg.(agentModelsLoadedMsg)
	if !ok {
		t.Fatalf("ApplyConfig reload produced %T, want agentModelsLoadedMsg", freshEv)
	}
	v, _ = v.Update(freshLoaded)
	if len(v.models) != 1 || v.models[0].Name != "new-agent-model" || v.model != "new-agent-model" {
		t.Fatalf("new-host agent list not applied: models=%v model=%q", modelNames(v.models), v.model)
	}

	// The old host's late result arrives: it must be ignored.
	v, _ = v.Update(staleLoaded)
	if len(v.models) != 1 || v.models[0].Name != "new-agent-model" || v.model != "new-agent-model" {
		t.Errorf("M-03: obsolete agent model list replaced the new host's after ApplyConfig: models=%v model=%q",
			modelNames(v.models), v.model)
	}
}

// TestAgentFallbackMarkerCommitsPersistentNote (F1): a plain-chat fallback
// (agent.FallbackMsg) must not be only a transient statusline notice. The
// committed assistant turn carries a persistent "no tool ran" caveat inline,
// so the render, the /export transcript, and a /resume reload of that
// transcript all surface it — a later reader can never treat a narrated
// tool claim from a fallback turn as real.
func TestAgentFallbackMarkerCommitsPersistentNote(t *testing.T) {
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})

	// Normal commit (no fallback): content untouched, no marker.
	v.streaming = true
	v.streamText = "plain answer"
	v, _ = v.Update(agent.AgentDoneMsg{Err: "", Reason: "stop"})
	if len(v.turns) != 1 || v.turns[0].msg.Content != "plain answer" {
		t.Fatalf("normal commit turn = %+v, want unmarked content", v.turns[0])
	}
	if v.plainChatReason != "" {
		t.Fatal("plainChatReason set without any fallback")
	}

	// Fallback turn: the caveat rides the committed content.
	v.streaming = true
	v.streamText = "I've written the numbers 1 to 300 to count_to_300.txt."
	v, _ = v.Update(agent.FallbackMsg{Reason: "model returned no tool call; showing plain chat response"})
	if v.notice == "" {
		t.Error("fallback should still surface the transient statusline notice")
	}
	v, _ = v.Update(agent.AgentDoneMsg{Err: "", Reason: "stop"})
	if len(v.turns) != 2 {
		t.Fatalf("turns = %d, want 2", len(v.turns))
	}
	got := v.turns[1].msg.Content
	if !strings.Contains(got, "> ⚠ **plain chat** — no tool ran this turn: model returned no tool call") {
		t.Errorf("fallback turn content = %q, want the persistent plain-chat note", got)
	}
	if !strings.Contains(got, "count_to_300.txt") {
		t.Errorf("fallback turn content = %q, lost the narrated claim it must caveat", got)
	}
	if v.plainChatReason != "" {
		t.Error("plainChatReason must clear when the turn commits")
	}
	// Inline, not a transient statusline: the rendered conversation shows it.
	out := stripANSI(v.View())
	if !strings.Contains(out, "no tool ran this turn") {
		t.Errorf("rendered conversation lacks the marker:\n%s", out)
	}
}

// TestAgentFallbackMarkerClearedAtTurnStart (F1): a fallback that produced
// no content (nothing committed) must not leak its marker into a later
// turn — startChat resets the in-flight state for every new turn.
func TestAgentFallbackMarkerClearedAtTurnStart(t *testing.T) {
	v := testAgent(t, nil)
	v, _ = v.Update(agentModelsLoadedMsg{models: sampleModels()})

	// Fallback lands but the turn ends with no streamed content: nothing
	// commits, and the stale reason must not survive into the next turn.
	v.streaming = true
	v, _ = v.Update(agent.FallbackMsg{Reason: "model does not support tools; using plain chat"})
	if v.plainChatReason == "" {
		t.Fatal("fallback reason not remembered")
	}
	v, _ = v.Update(agent.AgentDoneMsg{Err: "boom", Reason: ""})
	if len(v.turns) != 0 {
		t.Fatalf("turns = %d, want 0 (empty-content error turn commits nothing)", len(v.turns))
	}
	if v.plainChatReason == "" {
		t.Fatal("empty-content commit must leave the reason for startChat to clear")
	}
	// The next turn's startChat resets the in-flight state: the stale reason
	// must not leak a marker into the new turn's commit. An empty model fails
	// the runner before any network dial, so the goroutine drains instantly
	// (model loading above would otherwise have selected the first model).
	v.model = ""
	v, _ = v.startChat()
	drainChat(t, &v)
	if v.plainChatReason != "" {
		t.Error("stale plainChatReason survived into the next turn")
	}
}

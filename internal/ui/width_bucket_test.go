package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MerverliPy/SelfTUI/internal/agent"
)

// N1 micro-item (PLAN §12 N7): the chat render width is bucketed to a
// multiple of five columns so resize jitter inside one bucket reuses the
// bucketed glamour renderer and the per-width caches instead of rebuilding
// and re-rendering on every wiggle. These tests pin the bucket function and
// the resize behavior it buys.

func TestChatRenderWidthBucketsToFiveColumns(t *testing.T) {
	cases := []struct{ pane, want int }{
		{-4, 1}, // callers pass v.w-2; the helper clamps to the 1-col floor
		{0, 1},
		{1, 1},
		{4, 4}, // below one bucket: exact
		{5, 5}, // exact bucket edge
		{6, 5},
		{9, 5},
		{10, 10},
		{70, 70}, // the 72x30 device pane: already on a bucket
		{71, 70},
		{74, 70},
		{75, 75},
		{86, 85},
		{118, 115}, // the 120x40 golden pane
	}
	for _, c := range cases {
		if got := chatRenderWidth(c.pane); got != c.want {
			t.Errorf("chatRenderWidth(%d) = %d, want %d", c.pane, got, c.want)
		}
	}
}

// TestResizeJitterKeepsRenderCacheWarm drives the view through same-bucket
// and cross-bucket resizes exactly as the tea runtime would: within one
// bucket the renderer width and stream cache must not move; across a bucket
// boundary everything re-keys and the cache must still match a fresh render.
func TestResizeJitterKeepsRenderCacheWarm(t *testing.T) {
	const streamed = "some streamed prose long enough to wrap at these widths"
	v := streamingView(t) // 88x40 → pane 86 → bucket 85
	v, _ = v.Update(agent.TokenMsg{Text: streamed})
	tickStream(t, &v)
	if !v.streamRenderValid || v.streamRenderW != 85 {
		t.Fatalf("precondition: stream cache warm at 85, valid=%v w=%d", v.streamRenderValid, v.streamRenderW)
	}

	// Same-bucket jitter (pane 86 → 88): nothing re-keys, cache stays warm.
	v, _ = v.Update(tea.WindowSizeMsg{Width: 90, Height: 40})
	if v.renderW != 85 {
		t.Errorf("same-bucket resize rebuilt the renderer at width %d, want 85", v.renderW)
	}
	if !v.streamRenderValid || v.streamRenderW != 85 || v.streamRenderSrc != streamed {
		t.Error("same-bucket resize invalidated the warm stream cache")
	}

	// Bucket change (pane 88 → 66): renderer and cache re-key.
	v, _ = v.Update(tea.WindowSizeMsg{Width: 68, Height: 40})
	if v.renderW != 65 {
		t.Errorf("cross-bucket resize kept the old bucket: renderW = %d, want 65", v.renderW)
	}
	if !v.streamRenderValid || v.streamRenderW != 65 {
		t.Errorf("cross-bucket resize did not re-prime the stream cache: valid=%v w=%d", v.streamRenderValid, v.streamRenderW)
	}
	if got, want := v.streamBlockRender(), v.renderBlock(v.assistantHeader(v.model), v.streamText); got != want {
		t.Error("cache render drifted from fresh render after cross-bucket resize")
	}
}

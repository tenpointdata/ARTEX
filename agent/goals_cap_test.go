package agent

import (
	"context"
	"iter"
	"sync"
	"testing"

	"github.com/Autumn-27/norma/llm"
)

// maxTokensProbe answers every call with one text turn and records the output
// cap each request carried.
type maxTokensProbe struct {
	mu   sync.Mutex
	caps []int
}

func (p *maxTokensProbe) Stream(_ context.Context, req llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	p.mu.Lock()
	p.caps = append(p.caps, req.MaxTokens)
	p.mu.Unlock()
	return func(yield func(llm.StreamEvent, error) bool) {
		if yield(llm.StreamEvent{Type: llm.SETextDelta, Text: "done"}, nil) {
			yield(llm.StreamEvent{Type: llm.SEMessageDelta, StopReason: "end_turn"}, nil)
		}
	}
}

func (p *maxTokensProbe) Complete(ctx context.Context, req llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	acc := llm.NewAccumulator()
	for ev, err := range p.Stream(ctx, req) {
		if err != nil {
			return llm.Message{}, "", llm.Usage{}, err
		}
		acc.Add(ev)
	}
	return acc.Message(), acc.StopReason, acc.Usage, nil
}

func TestGoalsReplyCap(t *testing.T) {
	for _, tc := range []struct{ in, want int }{
		{0, GoalsMaxTokens},     // unset profile cap: our default, not norma's 32768
		{-1, GoalsMaxTokens},    // nonsense
		{32768, GoalsMaxTokens}, // a looser profile cap is clamped
		{2048, 2048},            // a tighter profile cap wins
		{GoalsMaxTokens, GoalsMaxTokens},
	} {
		if got := goalsReplyCap(tc.in); got != tc.want {
			t.Errorf("goalsReplyCap(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// With a profile max_tokens of 0 the decomposer used to send norma's 32768
// default; every request must now carry GoalsMaxTokens (c-7361b4).
func TestDecomposeGoalsSendsDefaultOutputCap(t *testing.T) {
	p := &maxTokensProbe{}
	DecomposeGoalsWithProvider(context.Background(), p, t.TempDir(), "find the flag on https://a.example.test", "", nil, nil, 0, false, 0, nil)
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.caps) == 0 {
		t.Fatal("decomposer made no model call")
	}
	for i, c := range p.caps {
		if c != GoalsMaxTokens {
			t.Errorf("call %d max_tokens = %d, want %d", i, c, GoalsMaxTokens)
		}
	}
}

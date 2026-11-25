package agent

import (
	"context"
	"iter"
	"sync"
	"testing"

	"github.com/Autumn-27/norma/llm"
)

// capProbe answers every call with one text turn and records the MaxTokens each
// request carried.
type capProbe struct {
	sessionProbe
	mu   sync.Mutex
	caps []int
}

func (p *capProbe) Stream(ctx context.Context, req llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	p.mu.Lock()
	p.caps = append(p.caps, req.MaxTokens)
	p.mu.Unlock()
	return p.sessionProbe.Stream(ctx, req)
}

func (p *capProbe) Complete(ctx context.Context, req llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	p.mu.Lock()
	p.caps = append(p.caps, req.MaxTokens)
	p.mu.Unlock()
	return p.sessionProbe.Complete(ctx, req)
}

func (p *capProbe) sent() []int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]int(nil), p.caps...)
}

func TestReplyCapPrecedence(t *testing.T) {
	for role, def := range DefaultReplyCaps {
		if def <= 0 || def >= 32768 {
			t.Fatalf("%s default %d must be a real cap below norma's 32768", role, def)
		}
		if got := ReplyCap(role, 0); got != def {
			t.Fatalf("%s profile 0: got %d want default %d", role, got, def)
		}
		for _, explicit := range []int{512, def * 4} { // a profile cap wins either way
			if got := ReplyCap(role, explicit); got != explicit {
				t.Fatalf("%s profile %d: got %d", role, explicit, got)
			}
		}
	}
	if got := resolveReplyCap(ReplyRoleWorker, nil); got != DefaultReplyCaps[ReplyRoleWorker] {
		t.Fatalf("nil resolver: got %d", got)
	}
}

// capCase drives one role once through its real entry point with the given
// profile cap resolver and returns what reached the provider.
type capCase struct {
	role ReplyRole
	run  func(t *testing.T, p *capProbe, profile func() int) error
}

func roleCapCases(f sessionFixture) []capCase {
	return []capCase{
		{ReplyRoleWorker, func(t *testing.T, p *capProbe, profile func() int) error {
			w := NewWorker(p, "test-model", t.TempDir(), nil, 0, 2)
			w.SetMaxTokens(profile)
			_, _, err := w.Execute(t.Context(), "work#1", f.expID, nil, f.ts, f.intent, passHooks{}, nil, nil, nil)
			return err
		}},
		{ReplyRolePlanner, func(t *testing.T, p *capProbe, profile func() int) error {
			pl := NewPlanner(p, "test-model", t.TempDir(), nil, 0, 2)
			pl.SetMaxTokens(profile)
			_, _, err := pl.Plan(t.Context(), f.expID, nil, f.ts, "probe goal", nil, passHooks{}, nil)
			return err
		}},
		{ReplyRoleMainAgent, func(t *testing.T, p *capProbe, profile func() int) error {
			m := NewMainAgent(p, "test-model", t.TempDir(), nil, 0, 2)
			m.SetMaxTokens(profile)
			_, err := m.Chat(t.Context(), f.expID, 0, nil, f.ts, "probe goal", "status?", passHooks{}, nil, nil, nil, nil, nil)
			return err
		}},
		{ReplyRoleChat, func(t *testing.T, p *capProbe, profile func() int) error {
			c := NewChatAgent(p, "test-model", t.TempDir(), nil, 100000)
			c.SetMaxTokens(profile)
			_, err := c.Chat(t.Context(), "assistant", "conv-cap", "hello", 1, 0, false, nil)
			return err
		}},
	}
}

// TestRoleSendsDefaultCapWhenProfileIsZero checks, per role, that a profile
// max_tokens of 0 (or no resolver) sends the role default rather than 0 (which
// norma turns into 32768), and that an explicit profile cap is sent unchanged.
func TestRoleSendsDefaultCapWhenProfileIsZero(t *testing.T) {
	f := newSessionFixture(t)
	for _, tc := range roleCapCases(f) {
		def := DefaultReplyCaps[tc.role]
		for _, pc := range []struct {
			name    string
			profile func() int
			want    int
		}{
			{"unset", nil, def},
			{"zero", func() int { return 0 }, def},
			{"explicit", func() int { return 20000 }, 20000},
		} {
			t.Run(string(tc.role)+"/"+pc.name, func(t *testing.T) {
				p := &capProbe{}
				if err := tc.run(t, p, pc.profile); err != nil {
					t.Fatal(err)
				}
				sent := p.sent()
				if len(sent) == 0 {
					t.Fatal("provider never called")
				}
				for _, got := range sent {
					if got != pc.want {
						t.Fatalf("sent MaxTokens %v, want %d on every request", sent, pc.want)
					}
				}
			})
		}
	}
}

package agent

import (
	"context"
	"errors"
	"strings"

	"github.com/Autumn-27/norma/agentcore"
	acperm "github.com/Autumn-27/norma/permission"
	actool "github.com/Autumn-27/norma/tool"
	"github.com/Autumn-27/norma/transcript"
)

// Complete makes one tool-less, single-turn completion against c and returns
// the reply text. It is for offline helpers (the RSI proposer's drafter) that
// need text from a model and nothing else: no tools are offered, so the model
// can only answer. An empty reply is an error.
func Complete(ctx context.Context, c Config, system, user string, maxTokens int) (string, error) {
	prov, err := c.NewProvider()
	if err != nil {
		return "", err
	}
	if maxTokens <= 0 {
		maxTokens = 8192
	}
	ctx = transcript.WithSessionID(ctx, "oneshot-"+transcript.NewSessionID())
	reply, err := agentcore.Run(ctx, agentcore.Options{
		Provider:       prov,
		SystemPrompt:   []string{system},
		Tools:          []actool.CoreTool{}, // not nil: nil means the SDK's unconfined file and shell tools
		PermissionMode: acperm.ModeBypass,
		MaxTurns:       1,
		MaxTokens:      maxTokens,
		NonStreaming:   !c.Stream,
	}, user)
	if err != nil {
		return "", err
	}
	if reply = strings.TrimSpace(reply); reply == "" {
		return "", errors.New("the model returned no content")
	}
	return reply, nil
}

// FrameData frames content as one data-not-instructions block (field names
// it) with a fresh nonce, preceded by the envelope's explanation. Callers
// outside this package use it to hand text to a model as data.
func FrameData(field, content string) string {
	env := newDataEnvelope()
	return env.intro() + "\n\n" + env.wrap(field, content)
}

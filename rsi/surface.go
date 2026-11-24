// Package rsi is the product-independent core of ARTEX's harness
// self-improvement gate (RSI.md, Loop C, and the Loop B lesson graduation).
//
// It owns three things and nothing else:
//
//   - the search space: which surfaces an automated loop may propose an edit
//     to (Edit, ValidateEdit). Everything else, the immutable surfaces above
//     all, is outside it by construction: there is no Edit kind that can name
//     them;
//   - the bounded, annealed edit budget per round (Budget);
//   - the benchmark gate: a deterministic verdict over a held-out baseline vs
//     candidate comparison, bound to the exact edit (Evaluate).
//
// It has no database, HTTP or model dependency. Persistence, the owner
// approval and the prompt-version pointer live in db/ and server/.
package rsi

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// SurfaceKind names what an edit changes.
type SurfaceKind string

const (
	// SurfacePromptBody is the editable body (section [A]) of one orchestration
	// role's system prompt, stored as an agent_prompts version. The code-owned
	// tails appended after it are not part of the body.
	SurfacePromptBody SurfaceKind = "prompt_body"
	// SurfaceLesson is a set of evidence-gated lessons graduating from
	// candidate to adopted (Loop B).
	SurfaceLesson SurfaceKind = "lesson"
)

// PromptRoles are the roles whose prompt body may be proposed. Chat agents,
// the retester and custom agents are not evaluated by the planner/worker
// suite, so they are not in the search space either.
var PromptRoles = []string{"goals", "planner", "worker"}

// GitReviewedSurfaces may be proposed per RSI.md but are files under git:
// they are adopted only through a reviewed pull request, never by this gate.
var GitReviewedSurfaces = []string{"skill", "tool_description"}

// ImmutableSurfaces are never in the search space (RSI.md rule 3). The list is
// declarative so the API can show it and tests can prove each one is refused;
// the refusal itself does not depend on the list, because ValidateEdit accepts
// only the two kinds above.
var ImmutableSurfaces = []string{
	"adversarial_gate",
	"quality_gate",
	"scope_check",
	"authorization_check",
	"prompt_injection_defense",
	"tool_allowlist",
	"code_owned_prompt_tail",
	"traffic_tools_tail",
	"artifact_rules_tail",
	"lesson_block",
	"rsi_gate",
}

const (
	// MaxBodyBytes caps a proposed prompt body.
	MaxBodyBytes = 64 << 10
	// MaxLessonsPerEdit caps how many lessons one proposal may graduate, so a
	// verdict stays attributable to a small set.
	MaxLessonsPerEdit = 8
)

// Edit is one proposed change inside the search space.
type Edit struct {
	Surface   SurfaceKind `json:"surface"`
	Target    string      `json:"target,omitempty"`     // prompt role, prompt_body only
	Body      string      `json:"body,omitempty"`       // template text, prompt_body only
	LessonIDs []int64     `json:"lesson_ids,omitempty"` // lesson only, sorted and unique after Normalize
}

// ErrOutsideSearchSpace marks an edit that names a surface no loop may change.
var ErrOutsideSearchSpace = errors.New("outside the RSI search space")

// SHA16 is the content hash the benchmark harness stamps for a prompt body
// (sha256, first 16 hex characters; scripts/benchmark/arms.py sha()).
func SHA16(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])[:16]
}

// Normalize returns the edit with lesson ids sorted and de-duplicated.
func (e Edit) Normalize() Edit {
	if len(e.LessonIDs) == 0 {
		e.LessonIDs = nil
		return e
	}
	ids := append([]int64(nil), e.LessonIDs...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := ids[:0]
	for i, id := range ids {
		if i == 0 || id != ids[i-1] {
			out = append(out, id)
		}
	}
	e.LessonIDs = out
	return e
}

// Module names the unit a verdict, and any regression, is attributed to.
func (e Edit) Module() string {
	switch e.Surface {
	case SurfacePromptBody:
		return "prompt_body:" + e.Target
	case SurfaceLesson:
		parts := make([]string, len(e.LessonIDs))
		for i, id := range e.LessonIDs {
			parts[i] = strconv.FormatInt(id, 10)
		}
		return "lesson:" + strings.Join(parts, ",")
	}
	return string(e.Surface)
}

// ValidateEdit admits an edit only when it is inside the search space.
// forbiddenMarkers are strings a body must not contain (case-insensitive):
// the server passes the markers of its code-owned tails, so a body cannot
// imitate or pre-empt the lesson block, the data envelope or another tail.
func ValidateEdit(e Edit, forbiddenMarkers []string) error {
	kind := string(e.Surface)
	for _, s := range ImmutableSurfaces {
		if kind == s || e.Target == s {
			return fmt.Errorf("%w: %q is an immutable surface (RSI.md rule 3)", ErrOutsideSearchSpace, s)
		}
	}
	for _, s := range GitReviewedSurfaces {
		if kind == s {
			return fmt.Errorf("%w: %q is a file under git; propose it as a reviewed pull request", ErrOutsideSearchSpace, s)
		}
	}
	switch e.Surface {
	case SurfacePromptBody:
		if !contains(PromptRoles, e.Target) {
			return fmt.Errorf("%w: prompt role %q is not one of %v", ErrOutsideSearchSpace, e.Target, PromptRoles)
		}
		if len(e.LessonIDs) > 0 {
			return errors.New("a prompt_body edit carries no lesson ids")
		}
		if strings.TrimSpace(e.Body) == "" {
			return errors.New("a prompt_body edit needs a non-empty body")
		}
		if len(e.Body) > MaxBodyBytes {
			return fmt.Errorf("body is %d bytes, over the %d byte cap", len(e.Body), MaxBodyBytes)
		}
		return CheckMarkers(e.Body, forbiddenMarkers)
	case SurfaceLesson:
		if e.Target != "" || e.Body != "" {
			return errors.New("a lesson edit carries only lesson ids")
		}
		n := e.Normalize().LessonIDs
		if len(n) == 0 || len(n) > MaxLessonsPerEdit {
			return fmt.Errorf("a lesson edit needs 1 to %d lesson ids", MaxLessonsPerEdit)
		}
		if n[0] <= 0 {
			return errors.New("lesson ids must be positive")
		}
		return nil
	}
	return fmt.Errorf("%w: unknown surface %q", ErrOutsideSearchSpace, kind)
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// NormalizeForMarkers folds text so look-alike spellings compare equal: NFKC,
// every dash to '-', format characters (zero-width and the like) dropped,
// whitespace runs to one space, lower case.
func NormalizeForMarkers(text string) string {
	text = norm.NFKC.String(text)
	var b strings.Builder
	space := false
	for _, r := range text {
		switch {
		case unicode.Is(unicode.Cf, r):
			continue
		case unicode.IsSpace(r):
			if !space {
				b.WriteByte(' ')
			}
			space = true
			continue
		case unicode.Is(unicode.Pd, r):
			r = '-'
		}
		space = false
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// CheckMarkers refuses text that contains a code-owned tail marker after
// normalization. Callers that render templates should check the rendered text
// too, because template actions can assemble a marker from pieces.
func CheckMarkers(text string, markers []string) error {
	folded := NormalizeForMarkers(text)
	for _, m := range markers {
		if fm := NormalizeForMarkers(m); fm != "" && strings.Contains(folded, fm) {
			return fmt.Errorf("%w: body contains %q, which belongs to a code-owned prompt tail", ErrOutsideSearchSpace, m)
		}
	}
	return nil
}

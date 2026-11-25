// Command stub is a tiny scripted (no real model) OpenAI chat-completions server
// that lets an ARTEX task run end to end in a lab. It answers each ARTEX LLM role
// deterministically: goals -> set_goals, planner -> add_intent then prove_goal,
// worker -> one Bash curl against the lab target then record_fact, judge -> allow.
// Stdlib only. Lab use only: the only host it ever directs a tool at is STUB_TARGET_BASE.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	targetBase = env("STUB_TARGET_BASE", "http://juice.lab.svc.cluster.local:3000")
	targetPath = env("STUB_TARGET_PATH", "/rest/products/search?q=apple")
	nIntents   = envInt("STUB_INTENTS", 3)
	maxDispat  = envInt("STUB_MAX_DISPATCH_PER_GOAL", 3) // safety valve against a mis-detected planner state
	targetRe   = regexp.MustCompile(regexp.QuoteMeta(targetBase) + `[^\s"'<>)\]]*`)
)

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func envInt(k string, d int) int {
	if n, err := strconv.Atoi(os.Getenv(k)); err == nil && n > 0 {
		return n
	}
	return d
}

// ---- wire types (OpenAI chat completions) ----

// text accepts a string or an array of {type:"text",text:"..."} parts.
type text string

func (t *text) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		*t = text(s)
		return nil
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(b, &parts); err != nil {
		*t = ""
		return nil
	}
	var sb strings.Builder
	for _, p := range parts {
		sb.WriteString(p.Text)
	}
	*t = text(sb.String())
	return nil
}

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Index    int    `json:"index"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type message struct {
	Role       string     `json:"role"`
	Content    text       `json:"content"`
	ToolCalls  []toolCall `json:"tool_calls"`
	ToolCallID string     `json:"tool_call_id"`
}

type chatReq struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	Tools    []struct {
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	} `json:"tools"`
	Stream bool `json:"stream"`
}

// call is one scripted tool call.
type call struct {
	Name string
	Args any
}

// reply is one scripted assistant turn: optional text and/or tool calls.
type reply struct {
	Text  string
	Calls []call
}

// ---- request inspection ----

type view struct {
	req        chatReq
	tools      map[string]bool
	system     string
	user       string
	calledBy   map[string]string // tool_call_id -> tool name
	resultText map[string]string // tool name -> last tool result content
}

func inspect(r chatReq) view {
	v := view{req: r, tools: map[string]bool{}, calledBy: map[string]string{}, resultText: map[string]string{}}
	for _, t := range r.Tools {
		v.tools[t.Function.Name] = true
	}
	var sys, usr strings.Builder
	for _, m := range r.Messages {
		switch m.Role {
		case "system":
			sys.WriteString(string(m.Content) + "\n")
		case "user":
			usr.WriteString(string(m.Content) + "\n")
		case "assistant":
			for _, c := range m.ToolCalls {
				v.calledBy[c.ID] = c.Function.Name
			}
		}
	}
	for _, m := range r.Messages {
		if m.Role == "tool" {
			if n, ok := v.calledBy[m.ToolCallID]; ok {
				v.resultText[n] = string(m.Content)
			}
		}
	}
	v.system, v.user = sys.String(), usr.String()
	return v
}

func (v view) called(name string) bool { _, ok := v.resultText[name]; return ok }

// role detection (see report): by tool set, since prompts are editable in the DB.
func (v view) role() string {
	switch {
	case len(v.tools) == 0 && strings.Contains(v.system, "Verdict output protocol"):
		return "judge"
	case v.tools["set_goals"] && !v.tools["add_intent"] && !v.tools["record_fact"]:
		return "goals"
	case v.tools["add_intent"] && v.tools["add_hint"]:
		return "mainagent"
	case v.tools["add_intent"] && (v.tools["prove_goal"] || v.tools["goal_met"]):
		return "planner"
	case v.tools["record_fact"] && !v.tools["add_intent"]:
		return "worker"
	}
	return "other"
}

// ---- scripted roles ----

var goalRe = regexp.MustCompile(`(?s)Task goal:\n(.+?)(?:\n\nTask description|\z)`)

func goalsReply(v view) reply {
	goal := "Probe the lab target"
	if m := goalRe.FindStringSubmatch(v.user); m != nil {
		goal = strings.TrimSpace(m[1])
	}
	if !v.called("add_task_scope") && !v.called("set_goals") && v.tools["add_task_scope"] && strings.Contains(goal, targetBase) {
		host := strings.TrimPrefix(strings.TrimPrefix(targetBase, "http://"), "https://")
		host = strings.Split(host, ":")[0]
		return reply{Calls: []call{{"add_task_scope", map[string]any{"kind": "subdomain", "value": host, "reason": "the task goal names " + targetBase}}}}
	}
	if !v.called("set_goals") {
		return reply{Calls: []call{{"set_goals", map[string]any{"goals": []map[string]any{{"text": goal}}}}}}
	}
	return reply{Text: "Goal registered."}
}

var (
	dispatchMu sync.Mutex
	dispatched = map[string]int{}
	busyRe     = regexp.MustCompile(`"(?:running_intents|open_intents|recent_done_intents)"\s*:\s*\[\s*\{`)
	frontierRe = regexp.MustCompile(`"frontier_open"\s*:\s*([1-9]\d*)`)
	runningRe  = regexp.MustCompile(`"(?:running_intents|open_intents)"\s*:\s*\[\s*\{`)
	goalIDRe   = regexp.MustCompile(`"id"\s*:\s*(\d+)\s*,\s*"kind"\s*:\s*"goal"`)
	factIDRe   = regexp.MustCompile(`"id"\s*:\s*(\d+)[^{}]*"summary"\s*:\s*"STUBFACT`)
)

func plannerReply(v view) reply {
	doneTrigger := strings.Contains(v.user, "finished; output conclusion")
	inFlight := runningRe.MatchString(v.user) || frontierRe.MatchString(v.user)
	hasIntents := busyRe.MatchString(v.user) || doneTrigger || inFlight
	goal := ""
	if m := regexp.MustCompile(`Task goal: (.+)`).FindStringSubmatch(v.user); m != nil {
		goal = strings.TrimSpace(m[1])
	}
	switch {
	case !hasIntents && !v.called("add_intent"):
		dispatchMu.Lock()
		dispatched[goal]++
		n := dispatched[goal]
		dispatchMu.Unlock()
		if n > maxDispat {
			return reply{Text: "Dispatch cap reached for this goal; waiting."}
		}
		items := make([]map[string]any, nIntents)
		for i := range items {
			items[i] = map[string]any{"priority": 5, "summary": fmt.Sprintf(
				"Probe %d of %d: send one GET to %s%s and report the HTTP status and body size (serves the task goal)", i+1, nIntents, targetBase, targetPath)}
		}
		return reply{Calls: []call{{"add_intent", map[string]any{"intents": items}}}}
	case v.called("add_intent"):
		return reply{Text: "Dispatched intents; waiting for workers to report."}
	case hasIntents && !inFlight && (doneTrigger || busyRe.MatchString(v.user)):
		return plannerFinish(v)
	}
	return reply{Text: "Work is in flight; nothing new to dispatch this round."}
}

// plannerFinish: list_goals -> list_facts -> prove_goal for every goal, then end the round.
func plannerFinish(v view) reply {
	switch {
	case !v.called("list_goals"):
		return reply{Calls: []call{{"list_goals", map[string]any{}}}}
	case !v.called("list_facts"):
		return reply{Calls: []call{{"list_facts", map[string]any{"q": "STUBFACT"}}}}
	case !v.called("prove_goal"):
		goals := goalIDRe.FindAllStringSubmatch(v.resultText["list_goals"], -1)
		fact := factIDRe.FindStringSubmatch(v.resultText["list_facts"])
		if len(goals) == 0 || fact == nil {
			return reply{Text: "No provable goal or fact yet; leaving the task open."}
		}
		var cs []call
		for _, g := range goals {
			gid, _ := strconv.Atoi(g[1])
			fid, _ := strconv.Atoi(fact[1])
			cs = append(cs, call{"prove_goal", map[string]any{"goal_id": gid, "evidence_id": fid,
				"reason": "the worker observed the lab target answering the scripted request (tool_output fact)"}})
		}
		return reply{Calls: cs}
	}
	return reply{Text: "Goals judged; round complete."}
}

var statusRe = regexp.MustCompile(`HTTP_STATUS=\d+ SIZE=\d+`)

func workerReply(v view) reply {
	url := targetBase + targetPath
	if m := targetRe.FindString(v.system + v.user); m != "" {
		url = m
	}
	switch {
	case !v.called("Bash"):
		cmd := fmt.Sprintf(os.Getenv("STUB_CMD_PREFIX")+"curl -sS -m 20 -o body.json -w 'HTTP_STATUS=%%{http_code} SIZE=%%{size_download}\\n' '%s'; head -c 300 body.json; echo", url)
		return reply{Calls: []call{{"Bash", map[string]any{"command": cmd}}}}
	case !v.called("record_fact"):
		line := statusRe.FindString(v.resultText["Bash"])
		if line == "" {
			return reply{Text: "The request did not return a status line; no fact recorded."}
		}
		return reply{Calls: []call{{"record_fact", map[string]any{"facts": []map[string]any{{
			"summary":    "STUBFACT lab target answered the scripted GET: " + line,
			"detail":     "GET " + url + " via curl; the status line above was printed by curl -w.",
			"evidence":   line,
			"confidence": "observed",
			"source":     "tool_output",
		}}}}}}
	}
	return reply{Text: "Sent one GET to the lab target and recorded the HTTP status as a fact."}
}

func judgeReply() reply {
	return reply{Text: `{"decision":"allow","comment":"Actual action: scripted lab request; Consequence if successful: reads a response from the lab target; Matched rule: A4"}`}
}

func answer(v view) (string, reply) {
	role := v.role()
	switch role {
	case "goals":
		return role, goalsReply(v)
	case "planner":
		return role, plannerReply(v)
	case "worker":
		return role, workerReply(v)
	case "judge":
		return role, judgeReply()
	}
	return role, reply{Text: "Acknowledged."}
}

// ---- HTTP ----

func handleChat(w http.ResponseWriter, r *http.Request) {
	var req chatReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":{"message":"bad json"}}`, 400)
		return
	}
	role, rep := answer(inspect(req))
	var names []string
	for _, c := range rep.Calls {
		args, _ := json.Marshal(c.Args)
		names = append(names, c.Name+string(args))
	}
	log.Printf("role=%s stream=%v msgs=%d tools=%d reply_text=%q calls=%v", role, req.Stream, len(req.Messages), len(req.Tools), clip(rep.Text, 80), names)
	if req.Stream {
		writeSSE(w, req.Model, rep)
		return
	}
	writeJSON(w, req.Model, rep)
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

var seq int64
var seqMu sync.Mutex

func nextID() string {
	seqMu.Lock()
	defer seqMu.Unlock()
	seq++
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), seq)
}

func toolCalls(rep reply) []toolCall {
	out := make([]toolCall, len(rep.Calls))
	for i, c := range rep.Calls {
		args, _ := json.Marshal(c.Args)
		out[i] = toolCall{ID: "call_" + nextID(), Type: "function", Index: i}
		out[i].Function.Name, out[i].Function.Arguments = c.Name, string(args)
	}
	return out
}

func finish(rep reply) string {
	if len(rep.Calls) > 0 {
		return "tool_calls"
	}
	return "stop"
}

var usage = map[string]any{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20}

func writeJSON(w http.ResponseWriter, model string, rep reply) {
	msg := map[string]any{"role": "assistant", "content": rep.Text}
	if tcs := toolCalls(rep); len(tcs) > 0 {
		msg["tool_calls"] = tcs
		if rep.Text == "" {
			msg["content"] = nil
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": "chatcmpl-" + nextID(), "object": "chat.completion", "created": time.Now().Unix(), "model": model,
		"choices": []map[string]any{{"index": 0, "message": msg, "finish_reason": finish(rep)}},
		"usage":   usage,
	})
}

func writeSSE(w http.ResponseWriter, model string, rep reply) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	id := "chatcmpl-" + nextID()
	send := func(delta map[string]any, finishReason any, u any) {
		chunk := map[string]any{"id": id, "object": "chat.completion.chunk", "created": time.Now().Unix(), "model": model}
		if u != nil {
			chunk["choices"], chunk["usage"] = []any{}, u
		} else {
			chunk["choices"] = []map[string]any{{"index": 0, "delta": delta, "finish_reason": finishReason}}
		}
		b, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", b)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
	send(map[string]any{"role": "assistant", "content": ""}, nil, nil)
	if rep.Text != "" {
		send(map[string]any{"content": rep.Text}, nil, nil)
	}
	for _, tc := range toolCalls(rep) {
		// start frame (id + name), then the arguments frame, as OpenAI streams them.
		send(map[string]any{"tool_calls": []map[string]any{{"index": tc.Index, "id": tc.ID, "type": "function",
			"function": map[string]any{"name": tc.Function.Name, "arguments": ""}}}}, nil, nil)
		send(map[string]any{"tool_calls": []map[string]any{{"index": tc.Index,
			"function": map[string]any{"arguments": tc.Function.Arguments}}}}, nil, nil)
	}
	send(map[string]any{}, finish(rep), nil)
	send(nil, nil, usage)
	fmt.Fprint(w, "data: [DONE]\n\n")
}

func handleModels(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": []map[string]any{{"id": "stub", "object": "model", "owned_by": "lab"}}})
}

func main() {
	mux := http.NewServeMux()
	for _, p := range []string{"/v1/chat/completions", "/chat/completions"} {
		mux.HandleFunc("POST "+p, handleChat)
	}
	for _, p := range []string{"/v1/models", "/models"} {
		mux.HandleFunc("GET "+p, handleModels)
	}
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprintln(w, "ok") })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("unhandled %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	})
	addr := env("STUB_ADDR", ":8080")
	log.Printf("llm stub listening on %s; target=%s%s intents=%d", addr, targetBase, targetPath, nIntents)
	log.Fatal(http.ListenAndServe(addr, mux))
}

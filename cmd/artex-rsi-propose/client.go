package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// proposalsPath is the only path the proposer ever writes to. The client has
// no generic POST: it cannot adopt, reject, roll back, upload evidence or edit
// a prompt, because no method here can reach those routes (RSI.md rules 2, 8).
const proposalsPath = "/api/rsi/proposals"

type client struct {
	base  string
	token string
	http  *http.Client
}

func newClient(base, token string) *client {
	return &client{base: strings.TrimRight(base, "/"), token: token, http: &http.Client{Timeout: 60 * time.Second}}
}

// get reads one JSON document. Only GET is possible through it.
func (c *client) get(path string, out any) error {
	return c.do(http.MethodGet, path, nil, out)
}

// propose posts one proposal to POST /api/rsi/proposals.
func (c *client) propose(body proposalReq, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return c.do(http.MethodPost, proposalsPath, raw, out)
}

func (c *client) do(method, path string, body []byte, out any) error {
	req, err := http.NewRequest(method, c.base+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		msg := string(raw)
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return fmt.Errorf("%s %s: HTTP %d: %s", method, path, resp.StatusCode, strings.TrimSpace(msg))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// proposalReq is the POST /api/rsi/proposals body for a prompt-body edit.
type proposalReq struct {
	Round        int      `json:"round"`
	Surface      string   `json:"surface"`
	Target       string   `json:"target"`
	Body         string   `json:"body"`
	ProposedFrom []string `json:"proposed_from"`
	Rationale    string   `json:"rationale"`
}

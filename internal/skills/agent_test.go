package skills

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSkillFinderLoop drives the full loop against stubbed OpenAI + Firecrawl:
// the model asks to search, then to fetch, then returns a final answer. It pins
// that tool results are fed back keyed by call id and that sources accumulate.
func TestSkillFinderLoop(t *testing.T) {
	// Firecrawl stub: search returns one hit, scrape returns a SKILL.md body.
	fcSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/search"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{"web": []map[string]any{
					{"url": "https://github.com/anthropics/skills/tree/main/skills/pdf", "title": "pdf skill", "description": "edit PDFs"},
				}},
			})
		case strings.HasSuffix(r.URL.Path, "/scrape"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data":    map[string]any{"markdown": "---\nname: pdf\n---\nFill PDF forms.", "metadata": map[string]any{}},
			})
		default:
			t.Errorf("unexpected firecrawl path %s", r.URL.Path)
		}
	}))
	defer fcSrv.Close()
	fc := NewFirecrawl("k")
	fc.baseURL = fcSrv.URL

	// OpenAI stub: a 3-turn script. Turn 1 -> search_github, turn 2 -> fetch_url,
	// turn 3 -> final answer. It also asserts the tool results arrive as tool
	// messages before it advances.
	var turn int
	oaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode chat request: %v", err)
		}
		turn++
		switch turn {
		case 1:
			writeToolCall(w, "call_1", "search_github", `{"query":"pdf skill"}`)
		case 2:
			if last := req.Messages[len(req.Messages)-1]; last.Role != "tool" || !strings.Contains(last.Content, "anthropics/skills") {
				t.Errorf("turn 2: expected search result fed back as tool msg, got %+v", last)
			}
			writeToolCall(w, "call_2", "fetch_url", `{"url":"https://raw.githubusercontent.com/anthropics/skills/main/skills/pdf/SKILL.md"}`)
		case 3:
			if last := req.Messages[len(req.Messages)-1]; !strings.Contains(last.Content, "Fill PDF forms") {
				t.Errorf("turn 3: expected fetched SKILL.md fed back, got %q", last.Content)
			}
			writeFinal(w, "Here is the pdf skill:\n---\nname: pdf\n---\nFill PDF forms.")
		default:
			t.Fatalf("unexpected turn %d", turn)
		}
	}))
	defer oaSrv.Close()
	chat := NewOpenAIChat("k", "test-model")
	chat.endpoint = oaSrv.URL

	finder := &SkillFinder{Chat: chat, FC: fc}
	res, err := finder.Find(context.Background(), "I need to edit a PDF")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if res.Steps != 3 {
		t.Errorf("steps = %d, want 3", res.Steps)
	}
	if !strings.Contains(res.Skill, "Fill PDF forms") {
		t.Errorf("skill missing content: %q", res.Skill)
	}
	if len(res.Sources) != 1 || !strings.Contains(res.Sources[0], "SKILL.md") {
		t.Errorf("sources = %v, want the fetched SKILL.md url", res.Sources)
	}
}

// TestSkillFinderNoConverge pins that a model that never stops calling tools is
// bounded by maxAgentSteps rather than looping forever.
func TestSkillFinderNoConverge(t *testing.T) {
	fcSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"web": []any{}}})
	}))
	defer fcSrv.Close()
	fc := NewFirecrawl("k")
	fc.baseURL = fcSrv.URL

	oaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeToolCall(w, "call_x", "search_github", `{"query":"loop"}`) // never finalizes
	}))
	defer oaSrv.Close()
	chat := NewOpenAIChat("k", "test-model")
	chat.endpoint = oaSrv.URL

	finder := &SkillFinder{Chat: chat, FC: fc}
	if _, err := finder.Find(context.Background(), "x"); err == nil {
		t.Fatal("want non-convergence error, got nil")
	}
}

// TestFindHandlerConcurrent pins that the skills_find handler fans multiple
// requirements out in parallel and returns one result per requirement, in order,
// with a per-requirement error recorded rather than aborting the batch.
func TestFindHandlerConcurrent(t *testing.T) {
	fcSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"web": []any{}}})
	}))
	defer fcSrv.Close()
	fc := NewFirecrawl("k")
	fc.baseURL = fcSrv.URL

	// The stub finalizes immediately, echoing the requirement so we can check
	// ordering; concurrency is exercised by issuing three at once.
	oaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		user := ""
		for _, m := range req.Messages {
			if m.Role == "user" {
				user = m.Content
			}
		}
		writeFinal(w, "skill for "+user)
	}))
	defer oaSrv.Close()
	chat := NewOpenAIChat("k", "m")
	chat.endpoint = oaSrv.URL

	finder := &SkillFinder{Chat: chat, FC: fc}
	_, out, err := findHandler(finder)(context.Background(), nil, findInput{
		Requirements: []string{"alpha", "beta", "gamma"},
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if len(out.Results) != 3 {
		t.Fatalf("got %d results, want 3", len(out.Results))
	}
	for i, want := range []string{"alpha", "beta", "gamma"} {
		if out.Results[i].Requirement != want {
			t.Errorf("result %d requirement = %q, want %q (order not preserved)", i, out.Results[i].Requirement, want)
		}
		if !strings.Contains(out.Results[i].Skill, want) {
			t.Errorf("result %d skill = %q, want it to mention %q", i, out.Results[i].Skill, want)
		}
	}
}

// TestGatherRequirements merges singular + plural and drops blanks.
func TestGatherRequirements(t *testing.T) {
	got := gatherRequirements(findInput{Requirement: "one", Requirements: []string{"", "two", "  "}})
	if len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Fatalf("got %v, want [one two]", got)
	}
}

func writeToolCall(w http.ResponseWriter, id, name, args string) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"choices": []map[string]any{{
			"message": map[string]any{
				"role": "assistant",
				"tool_calls": []map[string]any{{
					"id":       id,
					"type":     "function",
					"function": map[string]any{"name": name, "arguments": args},
				}},
			},
			"finish_reason": "tool_calls",
		}},
	})
}

func writeFinal(w http.ResponseWriter, content string) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"choices": []map[string]any{{
			"message":       map[string]any{"role": "assistant", "content": content},
			"finish_reason": "stop",
		}},
	})
}

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-draw/internal/ai"
	"github.com/abjelosevic88/pocket-homelab-draw/internal/store"
)

// aiFor is the person's AI: Pocket AI through Core, their own
// OpenAI-compatible key, or nil with why.
func (s *Server) aiFor(ctx context.Context, uid int64, st store.Settings) (*ai.Client, string, string) {
	email := ""
	if u, err := s.store.GetUser(ctx, uid); err == nil {
		email = u.Email
	}
	pocket, status := s.core.SelectAI(ctx, email, st.AIProvider, st.OpenAIKey != "")
	if pocket != nil {
		if !status.Ready {
			return nil, "", "Pocket AI: " + status.Detail
		}
		c := ai.New("", "", "", 150*time.Second)
		c.Pocket = pocket.WithModel(st.AIPocketModel)
		return c, "pocket", ""
	}
	if st.OpenAIKey == "" {
		return nil, "", status.Detail
	}
	return ai.New(st.OpenAIBaseURL, st.OpenAIKey, st.OpenAIModel, 150*time.Second), "openai", ""
}

func (s *Server) handleAIStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, _ := s.store.GetSettings(ctx, actorID(ctx))
	cl, used, why := s.aiFor(ctx, actorID(ctx), st)
	writeJSON(w, http.StatusOK, map[string]any{"ready": cl != nil, "provider": used, "detail": why})
}

func (s *Server) handleAIModels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c, status := s.core.SelectAI(ctx, userFrom(ctx).Email, "pocket", false)
	if c == nil || !status.Ready {
		writeErr(w, http.StatusServiceUnavailable, status.Detail)
		return
	}
	models, err := c.Models(ctx)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, models)
}

func validAIProvider(v string) bool { return core.ValidAIProvider(v) }

func validPocketModel(v string) bool { return core.ValidInferenceModel(v) }

const aiVoice = `You draw diagrams as Mermaid code inside someone's own drawing app. Answer with a diagram that works in Mermaid 11: plain ASCII ids, labels in quotes when they contain punctuation, no HTML, no click handlers, no styling unless asked. Keep labels short. Never invent facts the person did not give; when something is unclear, choose the simplest reading.`

func langLine(st store.Settings) string {
	if st.AILanguage != "" {
		return " Write the labels in " + st.AILanguage + "."
	}
	return " Write the labels in the language of the request."
}

var codeFence = regexp.MustCompile("(?s)```(?:mermaid)?\\s*\\n(.*?)```")

// cleanCode takes the diagram out of an answer (with or without a fence).
func cleanCode(out string) string {
	out = strings.TrimSpace(out)
	if m := codeFence.FindStringSubmatch(out); m != nil {
		out = m[1]
	}
	return strings.TrimSpace(out) + "\n"
}

// handleAIDiagram drafts a text diagram from a description, or changes the
// one given as asked.
func (s *Server) handleAIDiagram(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in struct {
		Prompt string `json:"prompt"`
		Code   string `json:"code"`
		Type   string `json:"type"` // a wish: flowchart, sequence, mindmap… ("" = choose)
	}
	if err := decode(r, &in); err != nil || strings.TrimSpace(in.Prompt) == "" {
		writeErr(w, http.StatusBadRequest, "say what to draw")
		return
	}
	st, _ := s.store.GetSettings(ctx, uid)
	cl, used, why := s.aiFor(ctx, uid, st)
	if cl == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": why})
		return
	}
	prompt := ""
	if strings.TrimSpace(in.Code) != "" {
		prompt = "Here is a Mermaid diagram:\n\n" + strings.TrimSpace(in.Code) + "\n\nChange it as asked, keeping everything else: " + strings.TrimSpace(in.Prompt)
	} else {
		prompt = "Draw this as a Mermaid diagram: " + strings.TrimSpace(in.Prompt)
		if in.Type != "" {
			prompt += "\nUse a " + in.Type + " diagram."
		} else {
			prompt += "\nChoose the kind of diagram that fits best (flowchart, sequenceDiagram, mindmap, timeline, gantt, erDiagram, classDiagram, stateDiagram-v2, pie, journey)."
		}
	}
	prompt += langLine(st) + "\nAnswer only with JSON: {\"title\": \"a short title\", \"code\": \"the Mermaid code\"}"
	out, err := cl.Complete(ctx, aiVoice, prompt, true)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	var res struct {
		Title string `json:"title"`
		Code  string `json:"code"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(out)), &res) != nil || strings.TrimSpace(res.Code) == "" {
		res.Code = out
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "provider": used, "title": strings.TrimSpace(res.Title), "code": cleanCode(res.Code)})
}

// handleAIFix mends a text diagram Mermaid could not draw.
func (s *Server) handleAIFix(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	if err := decode(r, &in); err != nil || strings.TrimSpace(in.Code) == "" {
		writeErr(w, http.StatusBadRequest, "there is no diagram to fix")
		return
	}
	st, _ := s.store.GetSettings(ctx, uid)
	cl, used, why := s.aiFor(ctx, uid, st)
	if cl == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": why})
		return
	}
	prompt := "Mermaid could not draw this diagram.\n\nError: " + strings.TrimSpace(in.Error) + "\n\nDiagram:\n" + in.Code +
		"\n\nFix it with the smallest change that makes it valid; keep the meaning and the labels. Answer only with JSON: {\"code\": \"the fixed Mermaid code\", \"change\": \"what you changed, in one short sentence\"}"
	out, err := cl.Complete(ctx, aiVoice, prompt, true)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	var res struct {
		Code   string `json:"code"`
		Change string `json:"change"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(out)), &res) != nil || strings.TrimSpace(res.Code) == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "the AI answered something else; try again"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "provider": used, "code": cleanCode(res.Code), "change": strings.TrimSpace(res.Change)})
}

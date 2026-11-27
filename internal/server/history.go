package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gsperry/agent-desk/internal/commands"
	"github.com/gsperry/agent-desk/internal/history"
	"github.com/gsperry/agent-desk/internal/send"
	"github.com/gsperry/agent-desk/internal/summary"
)

func (s *Server) conversations(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	list, err := s.cfg.History.List(ctx)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": list})
}

func (s *Server) conversationTranscript(w http.ResponseWriter, r *http.Request) {
	before, limit, err := windowParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	page, err := s.cfg.History.Read(r.Context(), r.PathValue("id"), before, limit)
	if errors.Is(err, history.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// exportConversation downloads a conversation as Markdown.
func (s *Server) exportConversation(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	name, body, err := s.cfg.History.Export(ctx, r.PathValue("id"))
	if errors.Is(err, history.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	// The name comes from export.Filename, which keeps only [a-z0-9-].
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	_, _ = w.Write([]byte(body))
}

// sessionCommands lists the slash commands a session's CLI accepts: Claude
// Code's built-ins and the custom commands and skills its project and user
// define. Other CLIs get none.
func (s *Server) sessionCommands(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	for _, sess := range s.cfg.Sessions.State().Sessions {
		if sess.ID != id {
			continue
		}
		list := []commands.Command{}
		if sess.Tool == "claude" && s.cfg.Commands != nil {
			list = s.cfg.Commands.List(sess.Directory)
		}
		writeJSON(w, http.StatusOK, map[string]any{"commands": list, "builtinVersion": commands.BuiltinVersion})
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown session"})
}

// usage totals tokens, time and reported cost over the last days days.
func (s *Server) usage(w http.ResponseWriter, r *http.Request) {
	days := 30
	if v := r.URL.Query().Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 366 {
			http.Error(w, "days must be 1 to 366", http.StatusBadRequest)
			return
		}
		days = n
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	rep, err := s.cfg.Usage.Report(ctx, days)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// summarize writes a catch-up summary with the model in the summary
// settings. It spends tokens on the user's provider, so it is refused
// until the user turns summaries on in Settings.
func (s *Server) summarize(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Since int `json:"since"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must be JSON with since"})
		return
	}
	if s.cfg.SummarySettings == nil || !s.cfg.SummarySettings.Get().Enabled {
		writeRefusal(w, &send.Refused{Reason: "summaries_off", Message: "Catch-up summaries are off. Turn them on in Settings; each one spends tokens."})
		return
	}
	model := s.cfg.SummarySettings.Get().Model
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	res, err := s.cfg.Summaries.Summarize(ctx, r.PathValue("id"), req.Since, model)
	logOutcome("summary", r.PathValue("id"), "model="+model, "ok", err)
	if writeRefusal(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// summarySettings reports whether summaries are on and their model.
func (s *Server) summarySettings(w http.ResponseWriter, _ *http.Request) {
	st := summary.Settings{Model: summary.DefaultModel}
	if s.cfg.SummarySettings != nil {
		st = s.cfg.SummarySettings.Get()
	}
	writeJSON(w, http.StatusOK, st)
}

// saveSummarySettings turns summaries on or off and sets their model.
func (s *Server) saveSummarySettings(w http.ResponseWriter, r *http.Request) {
	if s.cfg.SummarySettings == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "summary settings are not available"})
		return
	}
	var req summary.Settings
	if !decode(w, r, 4096, &req) {
		return
	}
	saved, err := s.cfg.SummarySettings.Set(req)
	if writeRefusal(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

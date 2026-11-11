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

// summarize writes a catch-up summary. It spends tokens on the user's
// provider, so the web app only offers it once the user turns it on.
func (s *Server) summarize(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Since int    `json:"since"`
		Model string `json:"model"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must be JSON with since and model"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	res, err := s.cfg.Summaries.Summarize(ctx, r.PathValue("id"), req.Since, req.Model)
	logOutcome("summary", r.PathValue("id"), "model="+req.Model, "ok", err)
	if writeRefusal(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, res)
}

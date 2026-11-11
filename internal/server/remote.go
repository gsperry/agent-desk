package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gsperry/agent-desk/internal/remote"
)

type remoteKey struct{}

// remoteCaller is the identity and settings of a remote request, set by
// requireRemote. Local requests have none.
type remoteCaller struct {
	id       remote.Identity
	settings remote.Settings
}

func remoteFrom(ctx context.Context) (remoteCaller, bool) {
	c, ok := ctx.Value(remoteKey{}).(remoteCaller)
	return c, ok
}

func (s *Server) remoteSettingsNow() remote.Settings {
	if s.cfg.Remote == nil {
		return remote.Defaults()
	}
	return s.cfg.Remote.Get()
}

// requireRemote admits a remote request only when remote access is on, the
// request names the configured host, a cross-site origin is absent, the
// proxy vouches for an allowed user, and the level covers what the route
// needs. Local-only routes are refused at every level.
func (s *Server) requireRemote(need remote.Need, next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		st := s.remoteSettingsNow()
		if !st.Enabled {
			http.Error(w, "remote access is off", http.StatusNotFound)
			return
		}
		if r.Host != st.Host {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if !remoteOriginAllowed(r, st.Host) {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		id, ok := remote.Identify(r, st)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !st.Level.Allows(need) {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"reason":  "remote_level",
				"message": "Not allowed from a remote device at the current level. Change it on the desk.",
			})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), remoteKey{}, remoteCaller{id: id, settings: st})))
	})
}

// remoteOriginAllowed accepts the remote host's own origin, and no origin
// only on reads that are not WebSocket upgrades.
func remoteOriginAllowed(r *http.Request, host string) bool {
	origin := r.Header.Get("Origin")
	if origin != "" {
		return origin == "https://"+host || origin == "http://"+host
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		return !isUpgrade(r)
	default:
		return false
	}
}

// whoami tells the web app whether it runs locally or remotely, and at
// which level.
func (s *Server) whoami(w http.ResponseWriter, r *http.Request) {
	if c, ok := remoteFrom(r.Context()); ok {
		writeJSON(w, http.StatusOK, map[string]any{"remote": true, "user": c.id.User, "via": c.id.Via, "level": c.settings.Level})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"remote": false})
}

// remoteSettings returns the settings to a local client, and only the level
// to a remote one.
func (s *Server) remoteSettings(w http.ResponseWriter, r *http.Request) {
	if c, ok := remoteFrom(r.Context()); ok {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": true, "level": c.settings.Level})
		return
	}
	if s.cfg.Remote == nil {
		writeJSON(w, http.StatusOK, map[string]any{"available": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"available": true, "settings": s.cfg.Remote.Get()})
}

// setRemoteSettings changes the settings. It is registered as local only,
// and checks again here.
func (s *Server) setRemoteSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := remoteFrom(r.Context()); ok || s.cfg.Remote == nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var in remote.Settings
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must be JSON settings"})
		return
	}
	saved, err := s.cfg.Remote.Set(in)
	logOutcome("remote", "settings", "enabled="+boolString(saved.Enabled)+" level="+string(saved.Level), "ok", err)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"available": true, "settings": saved})
}

func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

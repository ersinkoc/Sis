package api

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ersinkoc/sis/internal/config"
	"github.com/ersinkoc/sis/internal/store"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type setupRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	if s.store == nil || s.cfg == nil || s.cfg.Get() == nil {
		http.Error(w, "auth unavailable", http.StatusServiceUnavailable)
		return
	}
	cfg := s.cfg.Get()
	if !cfg.Auth.FirstRun || len(cfg.Auth.Users) > 0 {
		http.Error(w, "setup already complete", http.StatusConflict)
		return
	}
	var req setupRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || len(req.Password) < 8 {
		http.Error(w, "username and password with at least 8 chars are required", http.StatusBadRequest)
		return
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		s.internalError(w, "setup failed", err)
		return
	}
	next := *cfg
	next.Auth.FirstRun = false
	next.Auth.Users = []config.User{{Username: req.Username, PasswordHash: hash}}
	if s.configPath != "" {
		if err := (&config.Loader{Path: s.configPath}).Save(&next); err != nil {
			s.internalError(w, "setup failed", err)
			return
		}
	}
	s.cfg.Replace(&next)
	if s.audit != nil {
		_ = s.audit.Auditf("auth.setup", req.Username, nil, map[string]string{"username": req.Username})
	}
	if err := s.createSession(w, r, req.Username); err != nil {
		s.internalError(w, "setup failed", err)
		return
	}
	writeJSON(w, map[string]any{"username": req.Username})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.loginLimiter.allow(r) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return
	}
	if s.store == nil || s.cfg == nil || s.cfg.Get() == nil {
		http.Error(w, "auth unavailable", http.StatusServiceUnavailable)
		return
	}
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	user, ok := s.findUser(req.Username)
	if !ok || !verifyPassword(user.PasswordHash, req.Password) {
		http.Error(w, "invalid username or password", http.StatusUnauthorized)
		return
	}
	if err := s.createSession(w, r, user.Username); err != nil {
		s.internalError(w, "login failed", err)
		return
	}
	writeJSON(w, map[string]any{"username": user.Username})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if s.store != nil {
		if cookie, err := r.Cookie(s.cookieName()); err == nil {
			_ = s.store.Sessions().Delete(cookie.Value)
		}
	}
	http.SetCookie(w, &http.Cookie{Name: s.cookieName(), Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.secureCookie(r), SameSite: http.SameSiteLaxMode})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	session, ok := s.sessionFromRequest(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeJSON(w, map[string]any{"username": session.Username, "expires_at": session.ExpiresAt})
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request, username string) error {
	token, err := newToken()
	if err != nil {
		return err
	}
	ttl := s.sessionTTL()
	session := &store.Session{Token: token, Username: username, ExpiresAt: time.Now().Add(ttl)}
	if s.store != nil {
		if err := s.store.Sessions().Upsert(session); err != nil {
			return err
		}
	}
	s.setSessionCookie(w, r, session)
	return nil
}

func (s *Server) sessionFromRequest(r *http.Request) (*store.Session, bool) {
	if s.store == nil {
		return nil, false
	}
	cookie, err := r.Cookie(s.cookieName())
	if err != nil || cookie.Value == "" {
		return nil, false
	}
	session, err := s.store.Sessions().Get(cookie.Value)
	if errors.Is(err, store.ErrNotFound) || err != nil {
		return nil, false
	}
	if session.ExpiresAt.Before(time.Now()) {
		_ = s.store.Sessions().Delete(cookie.Value)
		return nil, false
	}
	session.ExpiresAt = time.Now().Add(s.sessionTTL())
	if err := s.store.Sessions().Upsert(session); err != nil {
		return nil, false
	}
	return session, true
}

func (s *Server) sessionTTL() time.Duration {
	if s.cfg != nil && s.cfg.Get() != nil && s.cfg.Get().Auth.SessionTTL.Duration > 0 {
		return s.cfg.Get().Auth.SessionTTL.Duration
	}
	return 24 * time.Hour
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, session *store.Session) {
	if session == nil {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: s.cookieName(), Value: session.Token, Path: "/", Expires: session.ExpiresAt,
		HttpOnly: true, Secure: s.secureCookie(r), SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) findUser(username string) (config.User, bool) {
	if s.cfg == nil || s.cfg.Get() == nil {
		return config.User{}, false
	}
	for _, user := range s.cfg.Get().Auth.Users {
		if user.Username == username {
			return user, true
		}
	}
	return config.User{}, false
}

func (s *Server) cookieName() string {
	if s.cfg != nil && s.cfg.Get() != nil && s.cfg.Get().Auth.CookieName != "" {
		return s.cfg.Get().Auth.CookieName
	}
	return "sis_session"
}

func (s *Server) secureCookie(r *http.Request) bool {
	if s.cfg != nil && s.cfg.Get() != nil && s.cfg.Get().Server.HTTP.TLS {
		return true
	}
	return r != nil && r.TLS != nil
}

func newToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"rueDeLegliseBooker/internal/storage"
)

// Person identifies a resident and its associated colour.
type Person struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Server wires HTTP handlers against the storage backend.
type Server struct {
	store        *storage.Store
	template     *template.Template
	static       http.Handler
	people       []Person
	pageTitle    string
	bannerTitle  string
	basePath     string
	password     string
	passwordHint string
	sessions     *sessionManager
}

const (
	sessionCookieName = "rue_session"
	sessionLifetime   = 24 * time.Hour
)

// New builds a server around the provided dependencies.
func New(store *storage.Store, tpl *template.Template, static http.Handler, people []Person, pageTitle, bannerTitle, basePath, password, passwordHint string) *Server {
	return &Server{
		store:        store,
		template:     tpl,
		static:       static,
		people:       append([]Person(nil), people...),
		pageTitle:    pageTitle,
		bannerTitle:  bannerTitle,
		basePath:     basePath,
		password:     password,
		passwordHint: passwordHint,
		sessions:     newSessionManager(sessionLifetime),
	}
}

// Routes exposes the configured HTTP routes.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/static/", s.protectHandler(s.static))
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/api/reservations", s.handleReservations)
	mux.HandleFunc("/api/reservations/", s.handleReservation)
	mux.HandleFunc("/api/people", s.handlePeople)
	mux.HandleFunc("/cal.ics", s.handleCalendar)
	if s.basePath == "" {
		return mux
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.hasBasePathPrefix(r.URL.Path) {
			mux.ServeHTTP(w, r)
			return
		}

		trimmedPath := s.stripBasePath(r.URL.Path)
		trimmedRawPath := ""
		if raw := r.URL.RawPath; raw != "" {
			trimmedRawPath = s.stripBasePath(raw)
		}

		clone := r.Clone(r.Context())
		urlCopy := *r.URL
		urlCopy.Path = trimmedPath
		urlCopy.RawPath = trimmedRawPath
		clone.URL = &urlCopy
		mux.ServeHTTP(w, clone)
	})
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	if !s.isAuthenticated(r) {
		s.renderLogin(w, http.StatusOK, "")
		return
	}

	peopleJSON, err := json.Marshal(s.people)
	if err != nil {
		http.Error(w, "failed to encode data", http.StatusInternalServerError)
		return
	}

	data := struct {
		PeopleJSON  template.JS
		PageTitle   string
		BannerTitle string
		BasePath    string
	}{
		PeopleJSON:  template.JS(peopleJSON),
		PageTitle:   s.pageTitle,
		BannerTitle: s.bannerTitle,
		BasePath:    s.basePath,
	}

	if err := s.template.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, "template rendering failed", http.StatusInternalServerError)
	}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if s.isAuthenticated(r) {
			http.Redirect(w, r, s.rootPath(), http.StatusSeeOther)
			return
		}
		s.renderLogin(w, http.StatusOK, "")
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form data", http.StatusBadRequest)
			return
		}

		password := strings.TrimSpace(r.PostFormValue("password"))
		if password != s.password {
			s.renderLogin(w, http.StatusUnauthorized, "Mot de passe incorrect.")
			return
		}

		token, err := s.sessions.Create()
		if err != nil {
			http.Error(w, "failed to create session", http.StatusInternalServerError)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    token,
			Path:     "/",
			Expires:  time.Now().Add(sessionLifetime),
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})

		http.Redirect(w, r, s.rootPath(), http.StatusSeeOther)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleReservations(w http.ResponseWriter, r *http.Request) {
	if !s.isAuthenticated(r) {
		s.writeUnauthorized(w)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.listReservations(w, r)
	case http.MethodPost:
		s.createReservation(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleReservation(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/api/reservations/") {
		http.NotFound(w, r)
		return
	}

	if !s.isAuthenticated(r) {
		s.writeUnauthorized(w)
		return
	}

	idStr := strings.TrimPrefix(r.URL.Path, "/api/reservations/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodDelete:
		if err := s.store.DeleteReservation(r.Context(), id); err != nil {
			http.Error(w, "failed to delete", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPatch:
		s.updateReservationComment(w, r, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePeople(w http.ResponseWriter, r *http.Request) {
	if !s.isAuthenticated(r) {
		s.writeUnauthorized(w)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.people)
}

func (s *Server) handleCalendar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	reservations, err := s.store.ListReservations(r.Context())
	if err != nil {
		http.Error(w, "failed to list reservations", http.StatusInternalServerError)
		return
	}

	now := time.Now().UTC()
	var builder strings.Builder
	builder.Grow(1024)

	builder.WriteString("BEGIN:VCALENDAR\r\n")
	builder.WriteString("VERSION:2.0\r\n")
	builder.WriteString("PRODID:-//rueDeLegliseBooker//FR\r\n")
	builder.WriteString("CALSCALE:GREGORIAN\r\n")
	builder.WriteString("METHOD:PUBLISH\r\n")

	for _, res := range reservations {
		dtStamp := formatICSTime(now)
		dtStart := formatICSTime(res.Start.UTC())
		dtEnd := formatICSTime(res.End.UTC())
		person := strings.TrimSpace(res.Person)
		if person == "" {
			person = "Reservation"
		}
		summary := escapeICS(person)
		description := summary
		if trimmed := strings.TrimSpace(res.Comment); trimmed != "" {
			description = fmt.Sprintf("%s\\n%s", summary, escapeICS(trimmed))
		}

		builder.WriteString("BEGIN:VEVENT\r\n")
		builder.WriteString("UID:")
		builder.WriteString(fmt.Sprintf("%d@rueDeLegliseBooker\r\n", res.ID))
		builder.WriteString("DTSTAMP:")
		builder.WriteString(dtStamp)
		builder.WriteString("\r\n")
		builder.WriteString("DTSTART:")
		builder.WriteString(dtStart)
		builder.WriteString("\r\n")
		builder.WriteString("DTEND:")
		builder.WriteString(dtEnd)
		builder.WriteString("\r\n")
		builder.WriteString("SUMMARY:")
		builder.WriteString(summary)
		builder.WriteString("\r\n")
		builder.WriteString("DESCRIPTION:")
		builder.WriteString(description)
		builder.WriteString("\r\n")
		builder.WriteString("END:VEVENT\r\n")
	}

	builder.WriteString("END:VCALENDAR\r\n")

	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=cal.ics")
	_, _ = w.Write([]byte(builder.String()))
}

func (s *Server) listReservations(w http.ResponseWriter, r *http.Request) {
	reservations, err := s.store.ListReservations(r.Context())
	if err != nil {
		http.Error(w, "failed to list reservations", http.StatusInternalServerError)
		return
	}

	type reservationResponse struct {
		ID      int64  `json:"id"`
		Person  string `json:"person"`
		Start   string `json:"start"`
		End     string `json:"end"`
		Comment string `json:"comment"`
	}

	out := make([]reservationResponse, 0, len(reservations))
	for _, res := range reservations {
		out = append(out, reservationResponse{
			ID:      res.ID,
			Person:  res.Person,
			Start:   res.Start.Format(time.RFC3339),
			End:     res.End.Format(time.RFC3339),
			Comment: res.Comment,
		})
	}

	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createReservation(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Person  string `json:"person"`
		Start   string `json:"start"`
		End     string `json:"end"`
		Comment string `json:"comment"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	start, err := time.Parse(time.RFC3339, payload.Start)
	if err != nil {
		http.Error(w, "invalid start", http.StatusBadRequest)
		return
	}
	end, err := time.Parse(time.RFC3339, payload.End)
	if err != nil {
		http.Error(w, "invalid end", http.StatusBadRequest)
		return
	}

	if !isKnownPerson(payload.Person, s.people) {
		http.Error(w, "unknown person", http.StatusBadRequest)
		return
	}

	res := storage.Reservation{
		Person:  payload.Person,
		Start:   start,
		End:     end,
		Comment: strings.TrimSpace(payload.Comment),
	}

	id, err := s.store.CreateReservation(r.Context(), res)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		http.Error(w, "failed to create", http.StatusInternalServerError)
		return
	}

	response := struct {
		ID      int64  `json:"id"`
		Person  string `json:"person"`
		Start   string `json:"start"`
		End     string `json:"end"`
		Comment string `json:"comment"`
	}{
		ID:      id,
		Person:  res.Person,
		Start:   res.Start.Format(time.RFC3339),
		End:     res.End.Format(time.RFC3339),
		Comment: res.Comment,
	}

	writeJSON(w, http.StatusCreated, response)
}

func isKnownPerson(person string, people []Person) bool {
	for _, p := range people {
		if p.Name == person {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) updateReservationComment(w http.ResponseWriter, r *http.Request, id int64) {
	var payload struct {
		Comment string `json:"comment"`
	}

	if !s.isAuthenticated(r) {
		s.writeUnauthorized(w)
		return
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	comment := strings.TrimSpace(payload.Comment)
	if err := s.store.UpdateReservationComment(r.Context(), id, comment); err != nil {
		http.Error(w, "failed to update", http.StatusInternalServerError)
		return
	}

	response := struct {
		ID      int64  `json:"id"`
		Comment string `json:"comment"`
	}{
		ID:      id,
		Comment: comment,
	}
	writeJSON(w, http.StatusOK, response)
}

func formatICSTime(t time.Time) string {
	return t.UTC().Format("20060102T150405Z")
}

func escapeICS(value string) string {
	escaped := strings.ReplaceAll(value, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\n", "\\n")
	escaped = strings.ReplaceAll(escaped, ",", "\\,")
	escaped = strings.ReplaceAll(escaped, ";", "\\;")
	return escaped
}

func (s *Server) isAuthenticated(r *http.Request) bool {
	if s.password == "" {
		return true
	}

	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}
	return s.sessions.Validate(cookie.Value)
}

func (s *Server) renderLogin(w http.ResponseWriter, status int, errorMessage string) {
	data := struct {
		BasePath  string
		Hint      string
		Error     string
		PageTitle string
	}{
		BasePath:  s.basePath,
		Hint:      s.passwordHint,
		Error:     errorMessage,
		PageTitle: s.pageTitle,
	}

	var buf bytes.Buffer
	if err := s.template.ExecuteTemplate(&buf, "login.html", data); err != nil {
		http.Error(w, "template rendering failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
}

func (s *Server) rootPath() string {
	if s.basePath == "" {
		return "/"
	}
	return s.basePath
}

func (s *Server) protectHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.isAuthenticated(r) {
			http.Redirect(w, r, s.rootPath(), http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) hasBasePathPrefix(path string) bool {
	if s.basePath == "" {
		return false
	}
	if path == s.basePath {
		return true
	}
	return strings.HasPrefix(path, s.basePath+"/")
}

func (s *Server) stripBasePath(path string) string {
	trimmed := strings.TrimPrefix(path, s.basePath)
	if trimmed == "" {
		return "/"
	}
	if !strings.HasPrefix(trimmed, "/") {
		return "/" + trimmed
	}
	return trimmed
}

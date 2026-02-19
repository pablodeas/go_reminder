// Package handlers contains HTTP handlers for the Reminder web application.
package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"reminder/internal/auth"
	"reminder/internal/db"
	"reminder/internal/logger"

	"github.com/gorilla/mux"
)

var templates *template.Template
var baseURL string
var log = logger.New("handlers")

// Init wires the template set and base URL into the handlers package.
func Init(tmpl *template.Template, base string) {
	templates = tmpl
	baseURL = base
	log.Info("Handlers initialized — baseURL=%s", base)
}

// ── helpers ───────────────────────────────────────────────────

func renderTemplate(w http.ResponseWriter, name string, data interface{}) {
	log.Debug("Render template: %q", name)
	if err := templates.ExecuteTemplate(w, name, data); err != nil {
		log.Error("Template %q: %v", name, err)
		http.Error(w, "Erro interno do servidor", http.StatusInternalServerError)
	}
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	log.Warn("JSON error %d: %s", code, msg)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func jsonOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// ── Middleware ────────────────────────────────────────────────

// RequireAuth redirects unauthenticated requests to /login.
func RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := auth.GetUserID(r)
		if err != nil {
			log.Debug("RequireAuth: unauthenticated access to %s — redirecting", r.URL.Path)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		log.Debug("RequireAuth: userID=%d accessing %s", userID, r.URL.Path)
		next(w, r)
	}
}

// ── Page Handlers ─────────────────────────────────────────────

func IndexHandler(w http.ResponseWriter, r *http.Request) {
	userID, err := auth.GetUserID(r)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	user, err := db.GetUserByID(userID)
	if err != nil {
		log.Error("IndexHandler: GetUserByID %d: %v", userID, err)
		http.Error(w, "Erro interno", http.StatusInternalServerError)
		return
	}
	reminders, err := db.GetReminders(userID)
	if err != nil {
		log.Error("IndexHandler: GetReminders userID=%d: %v", userID, err)
	}
	log.Debug("IndexHandler: rendering for user=%q reminders=%d", user.Username, len(reminders))
	renderTemplate(w, "index", map[string]interface{}{
		"User":      user,
		"Reminders": reminders,
		"Now":       time.Now(),
	})
}

func LoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		renderTemplate(w, "login", map[string]interface{}{
			"Registered": r.URL.Query().Get("registered") == "1",
			"Reset":      r.URL.Query().Get("reset") == "1",
		})
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")
	log.Info("Login attempt: username=%q ip=%s", username, r.RemoteAddr)

	user, err := db.GetUserByUsername(username)
	if err != nil || !auth.CheckPasswordHash(password, user.PasswordHash) {
		log.Warn("Login FAILED: username=%q ip=%s", username, r.RemoteAddr)
		renderTemplate(w, "login", map[string]string{"Error": "Usuário ou senha inválidos"})
		return
	}

	log.Info("Login OK: username=%q userID=%d ip=%s", username, user.ID, r.RemoteAddr)
	auth.SetUserSession(w, r, user.ID)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func RegisterHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		renderTemplate(w, "register", nil)
		return
	}

	username := r.FormValue("username")
	email := r.FormValue("email")
	password := r.FormValue("password")
	confirm := r.FormValue("confirm")
	log.Info("Register attempt: username=%q email=%q ip=%s", username, email, r.RemoteAddr)

	if password != confirm {
		log.Warn("Register FAILED (mismatch): username=%q", username)
		renderTemplate(w, "register", map[string]string{"Error": "As senhas não coincidem"})
		return
	}
	if len(password) < 6 {
		renderTemplate(w, "register", map[string]string{"Error": "Senha deve ter pelo menos 6 caracteres"})
		return
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		log.Error("Register: HashPassword: %v", err)
		renderTemplate(w, "register", map[string]string{"Error": "Erro interno"})
		return
	}
	if err := db.CreateUser(username, email, hash); err != nil {
		log.Warn("Register FAILED (db): username=%q err=%v", username, err)
		renderTemplate(w, "register", map[string]string{"Error": "Usuário ou email já existe"})
		return
	}
	log.Info("Register OK: username=%q", username)
	http.Redirect(w, r, "/login?registered=1", http.StatusSeeOther)
}

func LogoutHandler(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.GetUserID(r)
	log.Info("Logout: userID=%d ip=%s", userID, r.RemoteAddr)
	auth.ClearSession(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func ForgotPasswordHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		renderTemplate(w, "forgot", nil)
		return
	}
	email := r.FormValue("email")
	log.Info("ForgotPassword: email=%q ip=%s", email, r.RemoteAddr)

	token, user, err := auth.InitiatePasswordReset(email)
	if err != nil {
		log.Warn("ForgotPassword: email=%q not found", email)
		renderTemplate(w, "forgot", map[string]string{"Error": "Email não encontrado no sistema"})
		return
	}
	resetLink := fmt.Sprintf("%s/reset-password?token=%s", baseURL, token)
	log.Info("ForgotPassword: reset link generated for userID=%d — %s", user.ID, resetLink)

	renderTemplate(w, "forgot", map[string]interface{}{
		"Success":   true,
		"ResetLink": resetLink,
		"EmailFail": true,
	})
}

func ResetPasswordHandler(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if r.Method == http.MethodGet {
		if token == "" {
			http.Redirect(w, r, "/forgot-password", http.StatusSeeOther)
			return
		}
		renderTemplate(w, "reset", map[string]string{"Token": token})
		return
	}
	token = r.FormValue("token")
	password := r.FormValue("password")
	confirm := r.FormValue("confirm")

	if password != confirm {
		renderTemplate(w, "reset", map[string]interface{}{"Token": token, "Error": "As senhas não coincidem"})
		return
	}
	if err := auth.ResetPassword(token, password); err != nil {
		log.Warn("ResetPassword failed: %v", err)
		renderTemplate(w, "reset", map[string]interface{}{"Token": token, "Error": err.Error()})
		return
	}
	log.Info("ResetPassword: success for token=%s…", token[:8])
	http.Redirect(w, r, "/login?reset=1", http.StatusSeeOther)
}

func CalendarHandler(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.GetUserID(r)
	user, _ := db.GetUserByID(userID)
	now := time.Now()
	year, month := now.Year(), int(now.Month())

	if y := r.URL.Query().Get("year"); y != "" {
		year, _ = strconv.Atoi(y)
	}
	if m := r.URL.Query().Get("month"); m != "" {
		month, _ = strconv.Atoi(m)
	}
	log.Debug("CalendarHandler: userID=%d year=%d month=%d", userID, year, month)

	reminders, err := db.GetRemindersByMonth(userID, year, month)
	if err != nil {
		log.Error("CalendarHandler: GetRemindersByMonth: %v", err)
	}

	firstDay := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	lastDay := firstDay.AddDate(0, 1, -1)
	dayMap := make(map[int][]db.Reminder)
	for _, rem := range reminders {
		if rem.HasDueDate {
			d := rem.DueDate.Day()
			dayMap[d] = append(dayMap[d], rem)
		}
	}
	prev := firstDay.AddDate(0, -1, 0)
	next := firstDay.AddDate(0, 1, 0)

	renderTemplate(w, "calendar", map[string]interface{}{
		"User": user, "Year": year, "Month": month,
		"MonthName": firstDay.Format("January"),
		"FirstDay":  firstDay, "LastDay": lastDay,
		"DayMap": dayMap, "Reminders": reminders,
		"PrevYear": prev.Year(), "PrevMonth": int(prev.Month()),
		"NextYear": next.Year(), "NextMonth": int(next.Month()),
		"StartOffset": int(firstDay.Weekday()), "Now": now,
	})
}

func SettingsHandler(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.GetUserID(r)
	user, _ := db.GetUserByID(userID)
	renderTemplate(w, "settings", map[string]interface{}{"User": user})
}

// ── API Handlers ──────────────────────────────────────────────

func APIListReminders(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.GetUserID(r)
	log.Debug("API GET /reminders userID=%d", userID)
	reminders, err := db.GetReminders(userID)
	if err != nil {
		log.Error("APIListReminders: %v", err)
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, reminders)
}

func APIInsertReminder(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.GetUserID(r)
	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		DueDate     string `json:"due_date"`
		Priority    string `json:"priority"`
		Tags        string `json:"tags"`
		Recurrence  string `json:"recurrence"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Requisição inválida", 400)
		return
	}
	if req.Title == "" {
		jsonError(w, "Título é obrigatório", 400)
		return
	}
	if req.Priority == "" {
		req.Priority = "medium"
	}
	if req.Recurrence == "" {
		req.Recurrence = db.RecurrenceNone
	}
	log.Info("API POST /reminders userID=%d title=%q recurrence=%s", userID, req.Title, req.Recurrence)
	id, err := db.InsertReminder(userID, req.Title, req.Description, req.DueDate, req.Priority, req.Tags, req.Recurrence)
	if err != nil {
		log.Error("APIInsertReminder: %v", err)
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]interface{}{"id": id, "message": "Lembrete criado"})
}

func APIUpdateReminder(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.GetUserID(r)
	vars := mux.Vars(r)
	id, _ := strconv.Atoi(vars["id"])
	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		DueDate     string `json:"due_date"`
		Priority    string `json:"priority"`
		Tags        string `json:"tags"`
		Recurrence  string `json:"recurrence"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Requisição inválida", 400)
		return
	}
	log.Info("API PUT /reminders/%d userID=%d recurrence=%s", id, userID, req.Recurrence)
	if err := db.UpdateReminder(id, userID, req.Title, req.Description, req.DueDate, req.Priority, req.Tags, req.Recurrence); err != nil {
		log.Error("APIUpdateReminder id=%d: %v", id, err)
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]string{"message": "Lembrete atualizado"})
}

func APIDeleteReminder(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.GetUserID(r)
	vars := mux.Vars(r)
	id, _ := strconv.Atoi(vars["id"])
	log.Info("API DELETE /reminders/%d userID=%d", id, userID)
	if err := db.DeleteReminder(id, userID); err != nil {
		log.Error("APIDeleteReminder id=%d: %v", id, err)
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]string{"message": "Lembrete excluído"})
}

func APIDeleteMultiple(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.GetUserID(r)
	var req struct {
		IDs []int `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Requisição inválida", 400)
		return
	}
	log.Info("API POST /reminders/bulk-delete userID=%d ids=%v", userID, req.IDs)
	for _, id := range req.IDs {
		if err := db.DeleteReminder(id, userID); err != nil {
			log.Warn("bulk-delete: id=%d err=%v", id, err)
		}
	}
	jsonOK(w, map[string]string{"message": fmt.Sprintf("Excluídos: %d", len(req.IDs))})
}

func APIClearAll(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.GetUserID(r)
	log.Warn("API DELETE /reminders/clear userID=%d — clearing ALL", userID)
	if err := db.DeleteAllReminders(userID); err != nil {
		log.Error("APIClearAll userID=%d: %v", userID, err)
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]string{"message": "Todos os lembretes excluídos"})
}

// APIMarkNotified marks a reminder as notified and advances recurring ones.
func APIMarkNotified(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.GetUserID(r)
	vars := mux.Vars(r)
	id, _ := strconv.Atoi(vars["id"])
	log.Info("API POST /reminders/%d/notify userID=%d", id, userID)

	// Ownership check
	if _, err := db.GetReminderByID(id, userID); err != nil {
		log.Warn("APIMarkNotified: id=%d not found for userID=%d", id, userID)
		jsonError(w, "Lembrete não encontrado", 404)
		return
	}
	if err := db.MarkReminderNotified(id); err != nil {
		log.Error("APIMarkNotified id=%d: %v", id, err)
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]string{"message": "Marcado como notificado"})
}

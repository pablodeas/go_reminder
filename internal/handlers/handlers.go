package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

	"reminder/internal/auth"
	"reminder/internal/db"

	"github.com/gorilla/mux"
)

var templates *template.Template
var baseURL string

func Init(tmpl *template.Template, base string) {
	templates = tmpl
	baseURL = base
}

func renderTemplate(w http.ResponseWriter, name string, data interface{}) {
	if err := templates.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("template error [%s]: %v", name, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func jsonOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := auth.GetUserID(r); err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

// ─── Page Handlers ───────────────────────────────────────────

func IndexHandler(w http.ResponseWriter, r *http.Request) {
	userID, err := auth.GetUserID(r)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	user, _ := db.GetUserByID(userID)
	reminders, _ := db.GetReminders(userID)
	renderTemplate(w, "index", map[string]interface{}{
		"User":      user,
		"Reminders": reminders,
		"Now":       time.Now(),
	})
}

func LoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		renderTemplate(w, "login", map[string]interface{}{
			"Registered": r.URL.Query().Get("registered") == "1",
			"Reset":      r.URL.Query().Get("reset") == "1",
		})
		return
	}
	username := r.FormValue("username")
	password := r.FormValue("password")
	user, err := db.GetUserByUsername(username)
	if err != nil || !auth.CheckPasswordHash(password, user.PasswordHash) {
		renderTemplate(w, "login", map[string]string{"Error": "Usuário ou senha inválidos"})
		return
	}
	auth.SetUserSession(w, r, user.ID)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func RegisterHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		renderTemplate(w, "register", nil)
		return
	}
	username := r.FormValue("username")
	email := r.FormValue("email")
	password := r.FormValue("password")
	confirm := r.FormValue("confirm")

	if password != confirm {
		renderTemplate(w, "register", map[string]string{"Error": "As senhas não coincidem"})
		return
	}
	if len(password) < 6 {
		renderTemplate(w, "register", map[string]string{"Error": "Senha deve ter pelo menos 6 caracteres"})
		return
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		renderTemplate(w, "register", map[string]string{"Error": "Erro interno"})
		return
	}
	if err := db.CreateUser(username, email, hash); err != nil {
		renderTemplate(w, "register", map[string]string{"Error": "Usuário ou email já existe"})
		return
	}
	http.Redirect(w, r, "/login?registered=1", http.StatusSeeOther)
}

func LogoutHandler(w http.ResponseWriter, r *http.Request) {
	auth.ClearSession(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func ForgotPasswordHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		renderTemplate(w, "forgot", nil)
		return
	}
	email := r.FormValue("email")
	token, user, err := auth.InitiatePasswordReset(email)
	if err != nil {
		renderTemplate(w, "forgot", map[string]string{"Error": "Email não encontrado no sistema"})
		return
	}
	resetLink := fmt.Sprintf("%s/reset-password?token=%s", baseURL, token)
	log.Printf("Password reset link for %s: %s", user.Email, resetLink)
	renderTemplate(w, "forgot", map[string]interface{}{
		"Success":   true,
		"ResetLink": resetLink,
		"EmailFail": true,
	})
}

func ResetPasswordHandler(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if r.Method == "GET" {
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
		renderTemplate(w, "reset", map[string]interface{}{"Token": token, "Error": err.Error()})
		return
	}
	http.Redirect(w, r, "/login?reset=1", http.StatusSeeOther)
}

func CalendarHandler(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.GetUserID(r)
	user, _ := db.GetUserByID(userID)
	now := time.Now()
	year := now.Year()
	month := int(now.Month())
	if y := r.URL.Query().Get("year"); y != "" {
		year, _ = strconv.Atoi(y)
	}
	if m := r.URL.Query().Get("month"); m != "" {
		month, _ = strconv.Atoi(m)
	}
	reminders, _ := db.GetRemindersByMonth(userID, year, month)
	firstDay := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	lastDay := firstDay.AddDate(0, 1, -1)
	dayMap := make(map[int][]db.Reminder)
	for _, rem := range reminders {
		if rem.HasDueDate {
			dayMap[rem.DueDate.Day()] = append(dayMap[rem.DueDate.Day()], rem)
		}
	}
	prev := firstDay.AddDate(0, -1, 0)
	next := firstDay.AddDate(0, 1, 0)
	renderTemplate(w, "calendar", map[string]interface{}{
		"User": user, "Year": year, "Month": month,
		"MonthName": firstDay.Format("January"), "FirstDay": firstDay,
		"LastDay": lastDay, "DayMap": dayMap, "Reminders": reminders,
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

// ─── API Handlers ─────────────────────────────────────────────

func APIListReminders(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.GetUserID(r)
	reminders, err := db.GetReminders(userID)
	if err != nil {
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
	id, err := db.InsertReminder(userID, req.Title, req.Description, req.DueDate, req.Priority, req.Tags)
	if err != nil {
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
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Requisição inválida", 400)
		return
	}
	if err := db.UpdateReminder(id, userID, req.Title, req.Description, req.DueDate, req.Priority, req.Tags); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]string{"message": "Lembrete atualizado"})
}

func APIDeleteReminder(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.GetUserID(r)
	vars := mux.Vars(r)
	id, _ := strconv.Atoi(vars["id"])
	if err := db.DeleteReminder(id, userID); err != nil {
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
	for _, id := range req.IDs {
		db.DeleteReminder(id, userID)
	}
	jsonOK(w, map[string]string{"message": fmt.Sprintf("Excluídos: %d", len(req.IDs))})
}

func APIClearAll(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.GetUserID(r)
	if err := db.DeleteAllReminders(userID); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]string{"message": "Todos os lembretes excluídos"})
}

func APIMarkNotified(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.GetUserID(r)
	vars := mux.Vars(r)
	id, _ := strconv.Atoi(vars["id"])
	if _, err := db.GetReminderByID(id, userID); err != nil {
		jsonError(w, "Lembrete não encontrado", 404)
		return
	}
	if err := db.MarkReminderSent(id); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]string{"message": "Marcado como notificado"})
}

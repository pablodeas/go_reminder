package main

import (
	"embed"
	"flag"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"time"

	"reminder/internal/auth"
	"reminder/internal/db"
	"reminder/internal/handlers"

	"github.com/gorilla/mux"
)

//go:embed templates/*
var templateFiles embed.FS

//go:embed static/*
var staticFiles embed.FS

func main() {
	port := flag.String("port", "8080", "Porta do servidor")
	dbPath := flag.String("db", "reminder.db", "Caminho para o banco SQLite")
	secret := flag.String("secret", "reminder-session-secret-change-in-production", "Segredo da sessão")
	baseURL := flag.String("base-url", "", "URL base para links (ex: http://localhost:8080)")
	flag.Parse()

	if err := db.Init(*dbPath); err != nil {
		log.Fatalf("Falha ao inicializar banco: %v", err)
	}
	log.Printf("Banco de dados: %s", *dbPath)

	auth.Init(*secret)

	// Template functions
	funcMap := template.FuncMap{
		"formatDate": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.Format("02/01/2006 15:04")
		},
		"formatDateShort": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.Format("02/01")
		},
		"formatDateInput": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.Format("2006-01-02T15:04")
		},
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
		"mod": func(a, b int) int { return a % b },
		"seq": func(n int) []int {
			s := make([]int, n)
			for i := range s {
				s[i] = i + 1
			}
			return s
		},
		"int": func(a interface{}) int {
			switch v := a.(type) {
			case int:
				return v
			case int64:
				return int(v)
			default:
				return 0
			}
		},
		"priorityColor": func(p string) string {
			switch p {
			case "high":
				return "priority-high"
			case "low":
				return "priority-low"
			default:
				return "priority-medium"
			}
		},
		"isPast": func(t time.Time) bool {
			return !t.IsZero() && t.Before(time.Now())
		},
		"isSoon": func(t time.Time) bool {
			if t.IsZero() {
				return false
			}
			return t.After(time.Now()) && t.Before(time.Now().Add(24*time.Hour))
		},
	}

	tmpl, err := template.New("").Funcs(funcMap).ParseFS(templateFiles, "templates/*.html")
	if err != nil {
		log.Fatalf("Falha ao compilar templates: %v", err)
	}

	if *baseURL == "" {
		*baseURL = fmt.Sprintf("http://localhost:%s", *port)
	}

	handlers.Init(tmpl, *baseURL)

	r := mux.NewRouter()

	// Static files
	staticFS, _ := fs.Sub(staticFiles, "static")
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// Auth routes
	r.HandleFunc("/login", handlers.LoginHandler).Methods("GET", "POST")
	r.HandleFunc("/register", handlers.RegisterHandler).Methods("GET", "POST")
	r.HandleFunc("/logout", handlers.LogoutHandler)
	r.HandleFunc("/forgot-password", handlers.ForgotPasswordHandler).Methods("GET", "POST")
	r.HandleFunc("/reset-password", handlers.ResetPasswordHandler).Methods("GET", "POST")

	// Protected page routes
	r.HandleFunc("/", handlers.RequireAuth(handlers.IndexHandler))
	r.HandleFunc("/calendar", handlers.RequireAuth(handlers.CalendarHandler))
	r.HandleFunc("/settings", handlers.RequireAuth(handlers.SettingsHandler)).Methods("GET", "POST")

	// API routes
	api := r.PathPrefix("/api").Subrouter()
	api.HandleFunc("/reminders", handlers.RequireAuth(handlers.APIListReminders)).Methods("GET")
	api.HandleFunc("/reminders", handlers.RequireAuth(handlers.APIInsertReminder)).Methods("POST")
	api.HandleFunc("/reminders/{id:[0-9]+}", handlers.RequireAuth(handlers.APIUpdateReminder)).Methods("PUT")
	api.HandleFunc("/reminders/{id:[0-9]+}", handlers.RequireAuth(handlers.APIDeleteReminder)).Methods("DELETE")
	api.HandleFunc("/reminders/{id:[0-9]+}/notify", handlers.RequireAuth(handlers.APIMarkNotified)).Methods("POST")
	api.HandleFunc("/reminders/bulk-delete", handlers.RequireAuth(handlers.APIDeleteMultiple)).Methods("POST")
	api.HandleFunc("/reminders/clear", handlers.RequireAuth(handlers.APIClearAll)).Methods("DELETE")

	addr := fmt.Sprintf(":%s", *port)
	log.Printf("🔔 Reminder iniciado em %s", addr)
	log.Printf("   Acesse: http://localhost%s", addr)

	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

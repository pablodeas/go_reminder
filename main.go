package main

import (
	"embed"
	"flag"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"os"
	"time"

	"reminder/internal/auth"
	"reminder/internal/db"
	"reminder/internal/handlers"
	"reminder/internal/logger"

	"github.com/gorilla/mux"
)

//go:embed templates/*
var templateFiles embed.FS

//go:embed static/*
var staticFiles embed.FS

var log = logger.New("main")

func main() {
	port := flag.String("port", "8080", "Porta do servidor")
	dbPath := flag.String("db", "reminder.db", "Caminho para o banco SQLite")
	secret := flag.String("secret", "reminder-session-secret-change-in-production", "Segredo da sessão")
	baseURL := flag.String("base-url", "", "URL base (ex: http://localhost:8080)")
	debug := flag.Bool("debug", false, "Ativar nível DEBUG nos logs")
	logFile := flag.String("log-file", "", "Arquivo de log (padrão: stderr)")

	tlsCert := flag.String("tls-cert", "", "Caminho do certificado TLS (.pem)")
	tlsKey  := flag.String("tls-key",  "", "Caminho da chave privada TLS (.pem)")

	flag.Parse()

	// ── Logging setup ──────────────────────────────────────────
	if *debug {
		logger.SetLevel(logger.DEBUG)
		log.Debug("Debug logging enabled")
	} else {
		logger.SetLevel(logger.INFO)
	}

	if *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Fatal("Cannot open log file %s: %v", *logFile, err)
		}
		defer f.Close()
		logger.SetOutput(f)
		log.Info("Logging to file: %s", *logFile)
	}

	// Redirect stdlib log → our logger so gorilla/mux and other libs appear here too.
	logger.BridgeStdLog()

	// ── Database ───────────────────────────────────────────────
	log.Info("Starting Reminder server...")
	if err := db.Init(*dbPath); err != nil {
		log.Fatal("Database init failed: %v", err)
	}

	// ── Auth ───────────────────────────────────────────────────
	auth.Init(*secret)
	log.Debug("Auth session store initialized")

	// ── Templates ─────────────────────────────────────────────
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
		"recurrenceLabel": func(r string) string {
			switch r {
			case "daily":
				return "Diário"
			case "weekly":
				return "Semanal"
			case "biweekly":
				return "Quinzenal"
			case "monthly":
				return "Mensal"
			case "yearly":
				return "Anual"
			case "weekdays":
				return "Seg–Sex"
			default:
				return ""
			}
		},
	}

	log.Debug("Parsing templates...")
	tmpl, err := template.New("").Funcs(funcMap).ParseFS(templateFiles, "templates/*.html")
	if err != nil {
		log.Fatal("Template parse error: %v", err)
	}
	log.Debug("Templates parsed OK")

	if *baseURL == "" {
		*baseURL = fmt.Sprintf("http://localhost:%s", *port)
	}

	handlers.Init(tmpl, *baseURL)

	// ── Router ─────────────────────────────────────────────────
	r := mux.NewRouter()

	// HTTP request logging middleware
	r.Use(logger.Middleware)

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

	// ── Server ─────────────────────────────────────────────────
	addr := ":" + *port
	log.Info("────────────────────────────────────────")
	log.Info("🔔  Reminder server listening on %s", addr)
	log.Info("    URL      : %s", *baseURL)
	log.Info("    Database : %s", *dbPath)
	log.Info("    Log level: %s", map[bool]string{true: "DEBUG", false: "INFO"}[*debug])
	log.Info("────────────────────────────────────────")

	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	if *tlsCert != "" && *tlsKey != "" {
		log.Info("TLS ativado — cert=%s", *tlsCert)
		if err := srv.ListenAndServeTLS(*tlsCert, *tlsKey); err != nil && err != http.ErrServerClosed {
			log.Fatal("Server TLS error: %v", err)
		}
	} else {
		log.Warn("TLS desativado — conexão não criptografada!")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Server error: %v", err)
		}
	}
}

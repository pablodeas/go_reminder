// Package db manages the SQLite database for Reminder.
package db

import (
	"database/sql"
	"fmt"
	"time"

	"reminder/internal/logger"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB
var log = logger.New("db")

// Init opens the SQLite database and runs migrations.
func Init(dbPath string) error {
	log.Debug("Opening database: %s", dbPath)

	var err error
	DB, err = sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	if err := DB.Ping(); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	log.Info("Database connected: %s", dbPath)
	return migrate()
}

// migrate creates or updates all tables.
func migrate() error {
	log.Debug("Running migrations...")

	stmts := []struct {
		name string
		sql  string
	}{
		{"users", `CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			email TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			reset_token TEXT,
			reset_token_expiry DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`},
		{"reminders", `CREATE TABLE IF NOT EXISTS reminders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			title TEXT NOT NULL,
			description TEXT,
			due_date DATETIME,
			priority TEXT DEFAULT 'medium',
			tags TEXT,
			recurrence TEXT DEFAULT 'none',
			sent_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`},
	}

	for _, s := range stmts {
		log.Debug("Ensuring table: %s", s.name)
		if _, err := DB.Exec(s.sql); err != nil {
			return fmt.Errorf("create table %s: %w", s.name, err)
		}
	}

	// Add recurrence column to existing databases that don't have it yet.
	if err := addColumnIfMissing("reminders", "recurrence", "TEXT DEFAULT 'none'"); err != nil {
		return err
	}

	log.Info("Migrations complete — tables: users, reminders")
	return nil
}

// addColumnIfMissing safely adds a column to a table if it is absent.
func addColumnIfMissing(table, column, definition string) error {
	rows, err := DB.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return fmt.Errorf("pragma table_info %s: %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			continue
		}
		if name == column {
			log.Debug("Column %s.%s already exists — skip", table, column)
			return nil
		}
	}

	log.Info("Adding missing column %s.%s", table, column)
	_, err = DB.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition))
	if err != nil {
		return fmt.Errorf("alter table %s add column %s: %w", table, column, err)
	}
	return nil
}

// ── User ──────────────────────────────────────────────────────

type User struct {
	ID               int
	Username         string
	Email            string
	PasswordHash     string
	ResetToken       string
	ResetTokenExpiry time.Time
	CreatedAt        time.Time
}

func GetUserByUsername(username string) (*User, error) {
	log.Debug("GetUserByUsername: %q", username)
	u := &User{}
	var createdAt string
	err := DB.QueryRow(
		`SELECT id, username, email, password_hash, created_at FROM users WHERE username=?`,
		username,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &createdAt)
	if err != nil {
		log.Warn("GetUserByUsername %q: %v", username, err)
		return nil, err
	}
	u.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return u, nil
}

func GetUserByEmail(email string) (*User, error) {
	log.Debug("GetUserByEmail: %q", email)
	u := &User{}
	var createdAt string
	err := DB.QueryRow(
		`SELECT id, username, email, password_hash, created_at FROM users WHERE email=?`,
		email,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &createdAt)
	if err != nil {
		log.Warn("GetUserByEmail %q: %v", email, err)
		return nil, err
	}
	u.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return u, nil
}

func GetUserByID(id int) (*User, error) {
	log.Debug("GetUserByID: %d", id)
	u := &User{}
	var createdAt string
	err := DB.QueryRow(
		`SELECT id, username, email, password_hash, created_at FROM users WHERE id=?`,
		id,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &createdAt)
	if err != nil {
		log.Error("GetUserByID %d: %v", id, err)
		return nil, err
	}
	u.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return u, nil
}

func CreateUser(username, email, passwordHash string) error {
	log.Info("CreateUser: username=%q email=%q", username, email)
	_, err := DB.Exec(
		`INSERT INTO users (username, email, password_hash) VALUES (?,?,?)`,
		username, email, passwordHash,
	)
	if err != nil {
		log.Error("CreateUser %q: %v", username, err)
	}
	return err
}

func SetResetToken(userID int, token string, expiry time.Time) error {
	log.Info("SetResetToken: userID=%d expiry=%s", userID, expiry.Format(time.RFC3339))
	_, err := DB.Exec(
		`UPDATE users SET reset_token=?, reset_token_expiry=? WHERE id=?`,
		token, expiry.Format("2006-01-02 15:04:05"), userID,
	)
	return err
}

func GetUserByResetToken(token string) (*User, error) {
	log.Debug("GetUserByResetToken: token=%s…", token[:8])
	u := &User{}
	var expiry string
	err := DB.QueryRow(
		`SELECT id, username, email, password_hash, reset_token_expiry FROM users WHERE reset_token=?`,
		token,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &expiry)
	if err != nil {
		log.Warn("GetUserByResetToken: %v", err)
		return nil, err
	}
	u.ResetTokenExpiry, _ = time.Parse("2006-01-02 15:04:05", expiry)
	return u, nil
}

func UpdatePassword(userID int, hash string) error {
	log.Info("UpdatePassword: userID=%d", userID)
	_, err := DB.Exec(
		`UPDATE users SET password_hash=?, reset_token=NULL, reset_token_expiry=NULL WHERE id=?`,
		hash, userID,
	)
	return err
}

// ── Reminder ──────────────────────────────────────────────────

// Recurrence values understood by the application.
// "none" means fire once; other values repeat the due_date forward after notification.
const (
	RecurrenceNone     = "none"
	RecurrenceDaily    = "daily"
	RecurrenceWeekly   = "weekly"
	RecurrenceBiweekly = "biweekly"
	RecurrenceMonthly  = "monthly"
	RecurrenceYearly   = "yearly"
	RecurrenceWeekdays = "weekdays" // Mon-Fri
)

type Reminder struct {
	ID          int
	UserID      int
	Title       string
	Description string
	DueDate     time.Time
	Priority    string
	Tags        string
	Recurrence  string
	SentAt      time.Time
	CreatedAt   time.Time
	HasDueDate  bool
	HasSentAt   bool
}

func scanReminder(row interface {
	Scan(...interface{}) error
}) (Reminder, error) {
	var r Reminder
	var dueDate, sentAt, createdAt, recurrence string
	err := row.Scan(
		&r.ID, &r.UserID, &r.Title, &r.Description,
		&dueDate, &r.Priority, &r.Tags, &recurrence, &sentAt, &createdAt,
	)
	if err != nil {
		return r, err
	}

	if recurrence == "" {
		recurrence = RecurrenceNone
	}
	r.Recurrence = recurrence

	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04"} {
		if dueDate != "" {
			if t, e := time.Parse(layout, dueDate); e == nil && !t.IsZero() {
				r.DueDate = t
				r.HasDueDate = true
				break
			}
		}
	}
	if sentAt != "" {
		if t, e := time.Parse("2006-01-02 15:04:05", sentAt); e == nil {
			r.SentAt = t
			r.HasSentAt = !t.IsZero()
		}
	}
	r.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return r, nil
}

const reminderCols = `id, user_id, title, COALESCE(description,''),
	COALESCE(due_date,''), priority, COALESCE(tags,''),
	COALESCE(recurrence,'none'), COALESCE(sent_at,''), created_at`

func GetReminders(userID int) ([]Reminder, error) {
	log.Debug("GetReminders: userID=%d", userID)
	rows, err := DB.Query(
		`SELECT `+reminderCols+` FROM reminders WHERE user_id=?
		 ORDER BY COALESCE(due_date, created_at) ASC`,
		userID,
	)
	if err != nil {
		log.Error("GetReminders userID=%d: %v", userID, err)
		return nil, err
	}
	defer rows.Close()

	var out []Reminder
	for rows.Next() {
		r, err := scanReminder(rows)
		if err != nil {
			log.Warn("GetReminders scan: %v", err)
			continue
		}
		out = append(out, r)
	}
	log.Debug("GetReminders: returned %d reminders for userID=%d", len(out), userID)
	return out, nil
}

func GetRemindersByMonth(userID, year, month int) ([]Reminder, error) {
	log.Debug("GetRemindersByMonth: userID=%d year=%d month=%d", userID, year, month)
	rows, err := DB.Query(
		`SELECT `+reminderCols+` FROM reminders
		 WHERE user_id=?
		   AND strftime('%Y', COALESCE(due_date, created_at))=?
		   AND strftime('%m', COALESCE(due_date, created_at))=?
		 ORDER BY COALESCE(due_date, created_at) ASC`,
		userID, fmt.Sprintf("%04d", year), fmt.Sprintf("%02d", month),
	)
	if err != nil {
		log.Error("GetRemindersByMonth: %v", err)
		return nil, err
	}
	defer rows.Close()

	var out []Reminder
	for rows.Next() {
		r, err := scanReminder(rows)
		if err != nil {
			log.Warn("GetRemindersByMonth scan: %v", err)
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

func GetReminderByID(id, userID int) (*Reminder, error) {
	log.Debug("GetReminderByID: id=%d userID=%d", id, userID)
	row := DB.QueryRow(
		`SELECT `+reminderCols+` FROM reminders WHERE id=? AND user_id=?`,
		id, userID,
	)
	r, err := scanReminder(row)
	if err != nil {
		log.Warn("GetReminderByID id=%d: %v", id, err)
		return nil, err
	}
	return &r, nil
}

func InsertReminder(userID int, title, description, dueDate, priority, tags, recurrence string) (int64, error) {
	log.Info("InsertReminder: userID=%d title=%q recurrence=%s", userID, title, recurrence)
	var dueDateVal interface{}
	if dueDate != "" {
		dueDateVal = dueDate
	}
	if recurrence == "" {
		recurrence = RecurrenceNone
	}
	res, err := DB.Exec(
		`INSERT INTO reminders (user_id, title, description, due_date, priority, tags, recurrence)
		 VALUES (?,?,?,?,?,?,?)`,
		userID, title, description, dueDateVal, priority, tags, recurrence,
	)
	if err != nil {
		log.Error("InsertReminder: %v", err)
		return 0, err
	}
	id, _ := res.LastInsertId()
	log.Debug("InsertReminder: created id=%d", id)
	return id, nil
}

func UpdateReminder(id, userID int, title, description, dueDate, priority, tags, recurrence string) error {
	log.Info("UpdateReminder: id=%d userID=%d title=%q recurrence=%s", id, userID, title, recurrence)
	var dueDateVal interface{}
	if dueDate != "" {
		dueDateVal = dueDate
	}
	if recurrence == "" {
		recurrence = RecurrenceNone
	}
	_, err := DB.Exec(
		`UPDATE reminders SET title=?, description=?, due_date=?, priority=?, tags=?, recurrence=?, sent_at=NULL
		 WHERE id=? AND user_id=?`,
		title, description, dueDateVal, priority, tags, recurrence, id, userID,
	)
	if err != nil {
		log.Error("UpdateReminder id=%d: %v", id, err)
	}
	return err
}

func DeleteReminder(id, userID int) error {
	log.Info("DeleteReminder: id=%d userID=%d", id, userID)
	_, err := DB.Exec(`DELETE FROM reminders WHERE id=? AND user_id=?`, id, userID)
	if err != nil {
		log.Error("DeleteReminder id=%d: %v", id, err)
	}
	return err
}

func DeleteAllReminders(userID int) error {
	log.Warn("DeleteAllReminders: userID=%d — deleting ALL reminders", userID)
	res, err := DB.Exec(`DELETE FROM reminders WHERE user_id=?`, userID)
	if err != nil {
		log.Error("DeleteAllReminders userID=%d: %v", userID, err)
		return err
	}
	n, _ := res.RowsAffected()
	log.Info("DeleteAllReminders: removed %d rows for userID=%d", n, userID)
	return nil
}

// MarkReminderNotified marks sent_at and, for recurring reminders, advances
// the due_date to the next occurrence so the reminder fires again.
func MarkReminderNotified(id int) error {
	r, err := GetReminderByIDInternal(id)
	if err != nil {
		return err
	}

	if r.Recurrence == RecurrenceNone || r.Recurrence == "" || !r.HasDueDate {
		log.Info("MarkReminderNotified: id=%d (one-shot)", id)
		_, err = DB.Exec(`UPDATE reminders SET sent_at=CURRENT_TIMESTAMP WHERE id=?`, id)
		return err
	}

	next := nextOccurrence(r.DueDate, r.Recurrence)
	log.Info("MarkReminderNotified: id=%d recurrence=%s next=%s", id, r.Recurrence, next.Format("2006-01-02 15:04"))
	_, err = DB.Exec(
		`UPDATE reminders SET sent_at=CURRENT_TIMESTAMP, due_date=? WHERE id=?`,
		next.Format("2006-01-02 15:04:05"), id,
	)
	return err
}

// GetReminderByIDInternal retrieves without userID check (used internally).
func GetReminderByIDInternal(id int) (*Reminder, error) {
	row := DB.QueryRow(`SELECT `+reminderCols+` FROM reminders WHERE id=?`, id)
	r, err := scanReminder(row)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// nextOccurrence advances t according to the recurrence rule.
func nextOccurrence(t time.Time, recurrence string) time.Time {
	switch recurrence {
	case RecurrenceDaily:
		return t.AddDate(0, 0, 1)
	case RecurrenceWeekly:
		return t.AddDate(0, 0, 7)
	case RecurrenceBiweekly:
		return t.AddDate(0, 0, 14)
	case RecurrenceMonthly:
		return t.AddDate(0, 1, 0)
	case RecurrenceYearly:
		return t.AddDate(1, 0, 0)
	case RecurrenceWeekdays:
		next := t.AddDate(0, 0, 1)
		for next.Weekday() == time.Saturday || next.Weekday() == time.Sunday {
			next = next.AddDate(0, 0, 1)
		}
		return next
	default:
		return t
	}
}

package db

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

func Init(dbPath string) error {
	var err error
	DB, err = sql.Open("sqlite3", dbPath+"?_foreign_keys=on")
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	if err := DB.Ping(); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	return createTables()
}

func createTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			email TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			telegram_chat_id TEXT,
			telegram_bot_token TEXT,
			smtp_host TEXT,
			smtp_port INTEGER DEFAULT 587,
			smtp_user TEXT,
			smtp_pass TEXT,
			reset_token TEXT,
			reset_token_expiry DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS reminders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			title TEXT NOT NULL,
			description TEXT,
			due_date DATETIME,
			priority TEXT DEFAULT 'medium',
			tags TEXT,
			sent_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
	}

	for _, q := range queries {
		if _, err := DB.Exec(q); err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}
	return nil
}

// User operations
type User struct {
	ID               int
	Username         string
	Email            string
	PasswordHash     string
	TelegramChatID   string
	TelegramBotToken string
	SMTPHost         string
	SMTPPort         int
	SMTPUser         string
	SMTPPass         string
	ResetToken       string
	ResetTokenExpiry time.Time
	CreatedAt        time.Time
}

func GetUserByUsername(username string) (*User, error) {
	u := &User{}
	row := DB.QueryRow(`SELECT id, username, email, password_hash, 
		COALESCE(telegram_chat_id,''), COALESCE(telegram_bot_token,''),
		COALESCE(smtp_host,''), COALESCE(smtp_port,587), COALESCE(smtp_user,''), COALESCE(smtp_pass,''),
		created_at FROM users WHERE username=?`, username)
	var createdAt string
	err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash,
		&u.TelegramChatID, &u.TelegramBotToken,
		&u.SMTPHost, &u.SMTPPort, &u.SMTPUser, &u.SMTPPass,
		&createdAt)
	if err != nil {
		return nil, err
	}
	u.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return u, nil
}

func GetUserByEmail(email string) (*User, error) {
	u := &User{}
	row := DB.QueryRow(`SELECT id, username, email, password_hash,
		COALESCE(telegram_chat_id,''), COALESCE(telegram_bot_token,''),
		COALESCE(smtp_host,''), COALESCE(smtp_port,587), COALESCE(smtp_user,''), COALESCE(smtp_pass,''),
		created_at FROM users WHERE email=?`, email)
	var createdAt string
	err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash,
		&u.TelegramChatID, &u.TelegramBotToken,
		&u.SMTPHost, &u.SMTPPort, &u.SMTPUser, &u.SMTPPass,
		&createdAt)
	if err != nil {
		return nil, err
	}
	u.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return u, nil
}

func GetUserByID(id int) (*User, error) {
	u := &User{}
	row := DB.QueryRow(`SELECT id, username, email, password_hash,
		COALESCE(telegram_chat_id,''), COALESCE(telegram_bot_token,''),
		COALESCE(smtp_host,''), COALESCE(smtp_port,587), COALESCE(smtp_user,''), COALESCE(smtp_pass,''),
		created_at FROM users WHERE id=?`, id)
	var createdAt string
	err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash,
		&u.TelegramChatID, &u.TelegramBotToken,
		&u.SMTPHost, &u.SMTPPort, &u.SMTPUser, &u.SMTPPass,
		&createdAt)
	if err != nil {
		return nil, err
	}
	u.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return u, nil
}

func CreateUser(username, email, passwordHash string) error {
	_, err := DB.Exec(`INSERT INTO users (username, email, password_hash) VALUES (?,?,?)`,
		username, email, passwordHash)
	return err
}

func SetResetToken(userID int, token string, expiry time.Time) error {
	_, err := DB.Exec(`UPDATE users SET reset_token=?, reset_token_expiry=? WHERE id=?`,
		token, expiry.Format("2006-01-02 15:04:05"), userID)
	return err
}

func GetUserByResetToken(token string) (*User, error) {
	u := &User{}
	row := DB.QueryRow(`SELECT id, username, email, password_hash, reset_token_expiry FROM users WHERE reset_token=?`, token)
	var expiry string
	err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &expiry)
	if err != nil {
		return nil, err
	}
	u.ResetTokenExpiry, _ = time.Parse("2006-01-02 15:04:05", expiry)
	return u, nil
}

func UpdatePassword(userID int, hash string) error {
	_, err := DB.Exec(`UPDATE users SET password_hash=?, reset_token=NULL, reset_token_expiry=NULL WHERE id=?`, hash, userID)
	return err
}

func UpdateUserSettings(userID int, telegramChatID, telegramBotToken, smtpHost string, smtpPort int, smtpUser, smtpPass string) error {
	_, err := DB.Exec(`UPDATE users SET telegram_chat_id=?, telegram_bot_token=?, smtp_host=?, smtp_port=?, smtp_user=?, smtp_pass=? WHERE id=?`,
		telegramChatID, telegramBotToken, smtpHost, smtpPort, smtpUser, smtpPass, userID)
	return err
}

// Reminder operations
type Reminder struct {
	ID          int
	UserID      int
	Title       string
	Description string
	DueDate     time.Time
	Priority    string
	Tags        string
	SentAt      time.Time
	CreatedAt   time.Time
	HasDueDate  bool
	HasSentAt   bool
}

func GetReminders(userID int) ([]Reminder, error) {
	rows, err := DB.Query(`SELECT id, user_id, title, COALESCE(description,''), 
		COALESCE(due_date,''), priority, COALESCE(tags,''), COALESCE(sent_at,''), created_at 
		FROM reminders WHERE user_id=? ORDER BY COALESCE(due_date, created_at) ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reminders []Reminder
	for rows.Next() {
		var r Reminder
		var dueDate, sentAt, createdAt string
		if err := rows.Scan(&r.ID, &r.UserID, &r.Title, &r.Description,
			&dueDate, &r.Priority, &r.Tags, &sentAt, &createdAt); err != nil {
			log.Println("scan error:", err)
			continue
		}
		if dueDate != "" {
			r.DueDate, _ = time.Parse("2006-01-02 15:04:05", dueDate)
			if r.DueDate.IsZero() {
				r.DueDate, _ = time.Parse("2006-01-02T15:04", dueDate)
			}
			r.HasDueDate = !r.DueDate.IsZero()
		}
		if sentAt != "" {
			r.SentAt, _ = time.Parse("2006-01-02 15:04:05", sentAt)
			r.HasSentAt = !r.SentAt.IsZero()
		}
		r.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		reminders = append(reminders, r)
	}
	return reminders, nil
}

func GetRemindersByMonth(userID int, year, month int) ([]Reminder, error) {
	rows, err := DB.Query(`SELECT id, user_id, title, COALESCE(description,''), 
		COALESCE(due_date,''), priority, COALESCE(tags,''), COALESCE(sent_at,''), created_at 
		FROM reminders WHERE user_id=? AND strftime('%Y', COALESCE(due_date, created_at))=? AND strftime('%m', COALESCE(due_date, created_at))=?
		ORDER BY COALESCE(due_date, created_at) ASC`,
		userID, fmt.Sprintf("%04d", year), fmt.Sprintf("%02d", month))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reminders []Reminder
	for rows.Next() {
		var r Reminder
		var dueDate, sentAt, createdAt string
		if err := rows.Scan(&r.ID, &r.UserID, &r.Title, &r.Description,
			&dueDate, &r.Priority, &r.Tags, &sentAt, &createdAt); err != nil {
			continue
		}
		if dueDate != "" {
			r.DueDate, _ = time.Parse("2006-01-02 15:04:05", dueDate)
			r.HasDueDate = !r.DueDate.IsZero()
		}
		if sentAt != "" {
			r.SentAt, _ = time.Parse("2006-01-02 15:04:05", sentAt)
			r.HasSentAt = !r.SentAt.IsZero()
		}
		r.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		reminders = append(reminders, r)
	}
	return reminders, nil
}

func GetReminderByID(id, userID int) (*Reminder, error) {
	r := &Reminder{}
	var dueDate, sentAt, createdAt string
	row := DB.QueryRow(`SELECT id, user_id, title, COALESCE(description,''), 
		COALESCE(due_date,''), priority, COALESCE(tags,''), COALESCE(sent_at,''), created_at 
		FROM reminders WHERE id=? AND user_id=?`, id, userID)
	if err := row.Scan(&r.ID, &r.UserID, &r.Title, &r.Description,
		&dueDate, &r.Priority, &r.Tags, &sentAt, &createdAt); err != nil {
		return nil, err
	}
	if dueDate != "" {
		r.DueDate, _ = time.Parse("2006-01-02 15:04:05", dueDate)
		r.HasDueDate = !r.DueDate.IsZero()
	}
	if sentAt != "" {
		r.SentAt, _ = time.Parse("2006-01-02 15:04:05", sentAt)
		r.HasSentAt = !r.SentAt.IsZero()
	}
	r.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return r, nil
}

func InsertReminder(userID int, title, description, dueDate, priority, tags string) (int64, error) {
	var dueDateVal interface{}
	if dueDate != "" {
		dueDateVal = dueDate
	}
	result, err := DB.Exec(`INSERT INTO reminders (user_id, title, description, due_date, priority, tags) 
		VALUES (?,?,?,?,?,?)`, userID, title, description, dueDateVal, priority, tags)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func DeleteReminder(id, userID int) error {
	_, err := DB.Exec(`DELETE FROM reminders WHERE id=? AND user_id=?`, id, userID)
	return err
}

func DeleteAllReminders(userID int) error {
	_, err := DB.Exec(`DELETE FROM reminders WHERE user_id=?`, userID)
	return err
}

func MarkReminderSent(id int) error {
	_, err := DB.Exec(`UPDATE reminders SET sent_at=CURRENT_TIMESTAMP WHERE id=?`, id)
	return err
}

func UpdateReminder(id, userID int, title, description, dueDate, priority, tags string) error {
	var dueDateVal interface{}
	if dueDate != "" {
		dueDateVal = dueDate
	}
	_, err := DB.Exec(`UPDATE reminders SET title=?, description=?, due_date=?, priority=?, tags=? WHERE id=? AND user_id=?`,
		title, description, dueDateVal, priority, tags, id, userID)
	return err
}

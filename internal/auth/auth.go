package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"reminder/internal/db"

	"github.com/gorilla/sessions"
	"golang.org/x/crypto/bcrypt"
)

var Store *sessions.CookieStore

func Init(secret string) {
	Store = sessions.NewCookieStore([]byte(secret))
	Store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7,
		HttpOnly: true,
	}
}

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func GenerateToken() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func GetSession(r *http.Request) (*sessions.Session, error) {
	return Store.Get(r, "reminder-session")
}

func GetUserID(r *http.Request) (int, error) {
	session, err := Store.Get(r, "reminder-session")
	if err != nil {
		return 0, err
	}
	userID, ok := session.Values["user_id"]
	if !ok {
		return 0, errors.New("not authenticated")
	}
	id, ok := userID.(int)
	if !ok {
		return 0, errors.New("invalid session")
	}
	return id, nil
}

func SetUserSession(w http.ResponseWriter, r *http.Request, userID int) error {
	session, err := Store.Get(r, "reminder-session")
	if err != nil {
		return err
	}
	session.Values["user_id"] = userID
	return session.Save(r, w)
}

func ClearSession(w http.ResponseWriter, r *http.Request) error {
	session, err := Store.Get(r, "reminder-session")
	if err != nil {
		return err
	}
	session.Options.MaxAge = -1
	return session.Save(r, w)
}

func InitiatePasswordReset(email string) (string, *db.User, error) {
	user, err := db.GetUserByEmail(email)
	if err != nil {
		return "", nil, errors.New("email not found")
	}

	token, err := GenerateToken()
	if err != nil {
		return "", nil, err
	}

	expiry := time.Now().Add(1 * time.Hour)
	if err := db.SetResetToken(user.ID, token, expiry); err != nil {
		return "", nil, err
	}

	return token, user, nil
}

func ResetPassword(token, newPassword string) error {
	user, err := db.GetUserByResetToken(token)
	if err != nil {
		return errors.New("invalid or expired token")
	}

	if time.Now().After(user.ResetTokenExpiry) {
		return errors.New("token has expired")
	}

	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}

	return db.UpdatePassword(user.ID, hash)
}

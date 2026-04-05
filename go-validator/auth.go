package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const sessionCookie = "sf_session"
const sessionDuration = 30 * 24 * time.Hour

type contextKey string

const ctxUserID contextKey = "user_id"

func hashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	return string(b), err
}

func checkPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func generateSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// requireAuth middleware: reads sf_session cookie, validates in DB, sets user_id in context
func requireAuth(db *DB, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, 401)
			return
		}
		userID, err := db.GetSessionUser(r.Context(), cookie.Value)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, 401)
			return
		}
		ctx := context.WithValue(r.Context(), ctxUserID, userID)
		next(w, r.WithContext(ctx))
	}
}

func getUserID(r *http.Request) int64 {
	v, _ := r.Context().Value(ctxUserID).(int64)
	return v
}

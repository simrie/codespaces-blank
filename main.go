package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type response struct {
	Message string `json:"message"`
}

type Login struct {
	UserName  string    `json:"userName"`
	SessionID string    `json:"session_id"`
	HeaderID  string    `json:"header_id"`
	Expires   time.Time `json:"expires"`
}

var UserSessions = make(map[string]Login)
var userSessionsMu sync.RWMutex

func main() {
	server := &http.Server{
		Addr:    ":8080",
		Handler: newHandler(),
	}

	log.Printf("REST server listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func newHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthHandler)
	mux.Handle("GET /api/v1/message", requireSession(http.HandlerFunc(messageHandler)))
	mux.HandleFunc("POST /api/v1/login", loginHandler)
	return loggingMiddleware(mux)
}

func healthHandler(writer http.ResponseWriter, request *http.Request) {
	writeJSON(writer, http.StatusOK, response{Message: "ok"})
}

func messageHandler(writer http.ResponseWriter, request *http.Request) {
	writeJSON(writer, http.StatusOK, response{Message: "hello from Go"})
}

func requireSession(nextHTTP http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		sessionCookie, err := request.Cookie("sessionId")
		if err != nil || sessionCookie.Value == "" {
			writeJSON(writer, http.StatusUnauthorized, response{Message: "sessionId is required or invalid"})
			return
		}

		userSessionsMu.RLock()
		valid := false
		for _, login := range UserSessions {
			if login.SessionID == sessionCookie.Value {
				valid = true
				break
			}
		}
		userSessionsMu.RUnlock()

		if !valid {
			writeJSON(writer, http.StatusUnauthorized, response{Message: "sessionId is required or invalid"})
			return
		}

		nextHTTP.ServeHTTP(writer, request)
	})
}

func loginHandler(writer http.ResponseWriter, request *http.Request) {
	if err := request.ParseForm(); err != nil {
		writeJSON(writer, http.StatusBadRequest, response{Message: "invalid form data"})
		return
	}

	userName := strings.TrimSpace(request.PostFormValue("username"))
	if userName == "" {
		writeJSON(writer, http.StatusBadRequest, response{Message: "username is required"})
		return
	}

	sessionID, err := generateID()
	if err != nil {
		writeJSON(writer, http.StatusInternalServerError, response{Message: "could not create login"})
		return
	}
	headerID, err := generateID()
	if err != nil {
		writeJSON(writer, http.StatusInternalServerError, response{Message: "could not create login"})
		return
	}

	expires := time.Now().Add(time.Hour)

	login := Login{UserName: userName, SessionID: sessionID, HeaderID: headerID, Expires: expires}
	userSessionsMu.Lock()
	UserSessions[userName] = login
	userSessionsMu.Unlock()

	http.SetCookie(writer, &http.Cookie{
		Name:     "sessionId",
		Value:    sessionID,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(time.Hour.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(writer, &http.Cookie{
		Name:     "headerID",
		Value:    headerID,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(time.Hour.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(writer, &http.Cookie{
		Name:     "userName",
		Value:    userName,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(time.Hour.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	log.Printf("%+v", UserSessions)

	writeJSON(writer, http.StatusCreated, login)
}

func generateID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate random ID: %w", err)
	}
	return base64.StdEncoding.EncodeToString(bytes), nil
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		log.Printf("write response: %v", err)
	}
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		log.Printf("%s %s", request.Method, request.URL.Path)
		next.ServeHTTP(writer, request)
	})
}

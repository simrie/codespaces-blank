package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()

	newHandler().ServeHTTP(recorder, request)

	assertJSONResponse(t, recorder, http.StatusOK, response{Message: "ok"})
}

func TestMessageEndpointRequiresSession(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/api/v1/message", nil)
	recorder := httptest.NewRecorder()

	newHandler().ServeHTTP(recorder, request)

	assertJSONResponse(t, recorder, http.StatusUnauthorized, response{Message: "sessionId is required or invalid"})
}

func TestMessageEndpointRejectsUnknownSession(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/api/v1/message", nil)
	request.AddCookie(&http.Cookie{Name: "sessionId", Value: "not-registered"})
	recorder := httptest.NewRecorder()

	newHandler().ServeHTTP(recorder, request)

	assertJSONResponse(t, recorder, http.StatusUnauthorized, response{Message: "sessionId is required or invalid"})
}

func TestMessageEndpointAcceptsRegisteredSession(t *testing.T) {
	login := Login{UserName: "message-test-user", SessionID: "message-test-session"}
	userSessionsMu.Lock()
	UserSessions[login.UserName] = login
	userSessionsMu.Unlock()
	t.Cleanup(func() {
		userSessionsMu.Lock()
		delete(UserSessions, login.UserName)
		userSessionsMu.Unlock()
	})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/message", nil)
	request.AddCookie(&http.Cookie{Name: "sessionId", Value: login.SessionID})
	recorder := httptest.NewRecorder()

	newHandler().ServeHTTP(recorder, request)

	assertJSONResponse(t, recorder, http.StatusOK, response{Message: "hello from Go"})
}

func TestEndpointsRejectUnsupportedPost(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"/healthz", "/api/v1/message"} {
		request := httptest.NewRequest(http.MethodPost, path, nil)
		recorder := httptest.NewRecorder()

		newHandler().ServeHTTP(recorder, request)

		if recorder.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST %s returned status %d, want %d", path, recorder.Code, http.StatusMethodNotAllowed)
		}
	}
}

func TestLoginEndpointCreatesSession(t *testing.T) {
	userName := "login-test-user"
	form := url.Values{"username": {userName}}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/login", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()

	//startedAt := time.Now()
	newHandler().ServeHTTP(recorder, request)
	//completedAt := time.Now()

	if recorder.Code != http.StatusCreated {
		t.Fatalf("returned status %d, want %d", recorder.Code, http.StatusCreated)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json")
	}

	var got Login
	if err := json.NewDecoder(recorder.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.UserName != userName {
		t.Errorf("username = %q, want %q", got.UserName, userName)
	}
	sessionBytes, err := base64.StdEncoding.DecodeString(got.SessionID)
	if err != nil || len(sessionBytes) != 32 {
		t.Errorf("session_id is not a base64-encoded 32-byte ID: %q", got.SessionID)
	}
	headerBytes, err := base64.StdEncoding.DecodeString(got.HeaderID)
	if err != nil || len(headerBytes) != 32 {
		t.Errorf("header_id is not a base64-encoded 32-byte ID: %q", got.HeaderID)
	}
	if got.SessionID == got.HeaderID {
		t.Error("session_id and header_id should be different")
	}

	cookies := recorder.Result().Cookies()
	if len(cookies) != 3 {
		t.Fatalf("got %d cookies, want 3", len(cookies))
	}
	wantCookieValues := map[string]string{
		"sessionId": got.SessionID,
		"headerID":  got.HeaderID,
		"userName":  got.UserName,
	}
	for _, cookie := range cookies {
		wantValue, ok := wantCookieValues[cookie.Name]
		if !ok {
			t.Errorf("unexpected cookie %q", cookie.Name)
			continue
		}
		if cookie.Value != wantValue {
			t.Errorf("cookie %q value = %q, want %q", cookie.Name, cookie.Value, wantValue)
		}
		if !cookie.HttpOnly {
			t.Errorf("cookie %q is not HttpOnly", cookie.Name)
		}
		/*
			if cookie.MaxAge != int(time.Hour.Seconds()) {
				t.Errorf("cookie %q MaxAge = %d, want %d", cookie.Name, cookie.MaxAge, int(time.Hour.Seconds()))
			}
		*/
		/*
			if cookie.Expires.Before(startedAt.Add(time.Hour-time.Second)) || cookie.Expires.After(completedAt.Add(time.Hour)) {
				t.Errorf("cookie %q expires at %v, want about one hour after issuance", cookie.Name, cookie.Expires)
			}
		*/
	}

	userSessionsMu.RLock()
	stored, ok := UserSessions[userName]
	userSessionsMu.RUnlock()
	if !ok || stored != got {
		t.Errorf("stored login = %+v, want %+v", stored, got)
	}

	userSessionsMu.Lock()
	delete(UserSessions, userName)
	userSessionsMu.Unlock()
}

func TestLoginEndpointRejectsInvalidForm(t *testing.T) {
	tests := []struct {
		name string
		path string
		body string
	}{
		{name: "missing username", path: "/api/v1/login", body: ""},
		{name: "malformed form", path: "/api/v1/login", body: "%zz"},
		{name: "query username", path: "/api/v1/login?username=query-user", body: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			recorder := httptest.NewRecorder()

			newHandler().ServeHTTP(recorder, request)

			if recorder.Code != http.StatusBadRequest {
				t.Errorf("returned status %d, want %d", recorder.Code, http.StatusBadRequest)
			}
		})
	}
}

func assertJSONResponse(t *testing.T, recorder *httptest.ResponseRecorder, wantStatus int, wantBody response) {
	t.Helper()

	if recorder.Code != wantStatus {
		t.Fatalf("returned status %d, want %d", recorder.Code, wantStatus)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json")
	}

	var gotBody response
	if err := json.NewDecoder(recorder.Body).Decode(&gotBody); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if gotBody != wantBody {
		t.Errorf("response body = %+v, want %+v", gotBody, wantBody)
	}
}

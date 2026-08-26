package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()

	newHandler().ServeHTTP(recorder, request)

	assertJSONResponse(t, recorder, http.StatusOK, response{Message: "ok"})
}

func TestMessageEndpoint(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/api/v1/message", nil)
	recorder := httptest.NewRecorder()

	newHandler().ServeHTTP(recorder, request)

	assertJSONResponse(t, recorder, http.StatusOK, response{Message: "hello from Go"})
}

func TestEndpointsRejectUnsupportedMethods(t *testing.T) {
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

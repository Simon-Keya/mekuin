package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDarajaCallbackHandlerRejectsMalformedBodyBeforeDatabase(t *testing.T) {
	s := &server{}
	r := httptest.NewRequest("POST", "/daraja/stk/callback", strings.NewReader(`{"bad":true}`))
	w := httptest.NewRecorder()
	s.receiveDarajaSTKCallback(w, r)
	if w.Code != 400 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

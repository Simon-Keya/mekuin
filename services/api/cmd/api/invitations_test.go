package main

import (
	"encoding/hex"
	"testing"
)

func TestInvitationTokenDigest(t *testing.T) {
	token, digest, err := newInvitationToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 64 || len(digest) != 32 {
		t.Fatalf("unexpected token/digest sizes: %d/%d", len(token), len(digest))
	}
	if got := invitationTokenDigest(token); hex.EncodeToString(got) != hex.EncodeToString(digest) {
		t.Fatal("digest is not deterministic")
	}
	if invitationTokenDigest(token[:62]) != nil {
		t.Fatal("short token must be rejected")
	}
	if invitationTokenDigest("not-hex") != nil {
		t.Fatal("malformed token must be rejected")
	}
	other, _, err := newInvitationToken()
	if err != nil {
		t.Fatal(err)
	}
	if other == token {
		t.Fatal("cryptographic tokens unexpectedly repeated")
	}
}

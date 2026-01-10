package databento

import (
	"testing"
)

func TestConnect(t *testing.T) {
	t.Skip("live gateway not available on weekends")
	key, err := GetKey()
	if err != nil {
		t.Skipf("no API key: %v", err)
	}

	// XNAS.ITCH is NASDAQ TotalView-ITCH
	client := NewClient("XNAS.ITCH", key)
	if err := client.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	sessionID, err := client.Authenticate()
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	t.Logf("authenticated, session_id=%s", sessionID)
}

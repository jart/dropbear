package databento

import (
	"flag"
	"os"
	"testing"
)

var flagLive = flag.Bool("live", false, "run live integration tests")

func TestClient_Authenticate(t *testing.T) {
	if !*flagLive {
		t.Skip("skipping live test (use -live to enable)")
	}

	apiKey, err := GetKey()
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("~/.databento.key not found")
		}
		t.Fatal(err)
	}

	client := NewClient("OPRA.PILLAR", apiKey)
	if err := client.Connect(); err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	sessionID, err := client.Authenticate()
	if err != nil {
		t.Fatal(err)
	}
	if sessionID == "" {
		t.Fatal("got empty session ID")
	}
	t.Logf("session_id: %s", sessionID)
}

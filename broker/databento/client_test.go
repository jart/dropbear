package databento

import (
	"flag"
	"testing"
)

var flagLive = flag.Bool("live", false, "run live integration tests")

func TestDial(t *testing.T) {
	if !*flagLive {
		t.Skip("skipping live test (use -live to enable)")
	}

	client, err := Dial("OPRA.PILLAR", MustLoadDefaultKey())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
}

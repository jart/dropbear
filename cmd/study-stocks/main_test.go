package main

import (
	"dropbear/netty"
	"flag"
	"testing"
)

func init() {
	netty.SetOffline()
}

func TestStudy(t *testing.T) {
	paths := flag.Args()
	if len(paths) == 0 {
		t.Skip("no sip files specified (use: go test -run Study -timeout 0 -- file.sip)")
	}
	for _, path := range paths {
		study(path)
	}
}

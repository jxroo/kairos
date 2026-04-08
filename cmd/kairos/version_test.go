package main

import (
	"bytes"
	"strings"
	"testing"

	buildversion "github.com/jxroo/kairos/internal/version"
)

func TestVersionCommand(t *testing.T) {
	var buf bytes.Buffer
	versionCmd.SetOut(&buf)
	versionCmd.Run(versionCmd, nil)

	if strings.TrimSpace(buf.String()) != buildversion.Version {
		t.Fatalf("expected version %q, got %q", buildversion.Version, buf.String())
	}
}

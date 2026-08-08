package screenshot

import (
	"os"
	"testing"
)

func TestCaptureInteractiveExistence(t *testing.T) {
	// Verify screencapture binary exists on macOS
	if _, err := os.Stat("/usr/sbin/screencapture"); os.IsNotExist(err) {
		t.Skip("screencapture utility not found, skipping interactive test")
	}
}

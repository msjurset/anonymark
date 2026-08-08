package screenshot

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// CaptureInteractive invokes macOS screencapture CLI to let the user select a region on screen.
func CaptureInteractive() (string, error) {
	tempDir := os.TempDir()
	fileName := fmt.Sprintf("anonymark_cap_%d.png", time.Now().UnixNano())
	outPath := filepath.Join(tempDir, fileName)

	cmd := exec.Command("screencapture", "-i", outPath)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("screencapture failed or canceled: %w", err)
	}

	if _, err := os.Stat(outPath); os.IsNotExist(err) {
		return "", fmt.Errorf("screencapture output file was not created")
	}

	return outPath, nil
}

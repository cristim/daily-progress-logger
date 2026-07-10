// Package loginitem manages the optional macOS LaunchAgent that starts the app
// at login. The pure logic (plist rendering, offer decision) is testable
// without I/O; the OS operations (file write, launchctl) are kept in thin
// functions that are not tested directly.
package loginitem

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	// BundleID is the launchd label for the login-item agent.
	BundleID = "com.cristim.daily-progress-logger"
)

// PlistPath returns the full path of the LaunchAgent plist file.
func PlistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating home dir: %w", err)
	}
	return filepath.Join(home, "Library", "LaunchAgents", BundleID+".plist"), nil
}

// Exists reports whether the login-item plist file is already on disk.
func Exists(plistPath string) bool {
	_, err := os.Stat(plistPath)
	return err == nil
}

// ShouldOffer returns true when the login-item dialog should be presented:
// the plist does not exist, the user has not been asked before, and the app
// is not running in oneshot mode (cron/launchd invocation).
func ShouldOffer(plistExists, loginItemOffered, oneshotMode bool) bool {
	return !plistExists && !loginItemOffered && !oneshotMode
}

// RenderPlist returns the plist XML content for a login-item LaunchAgent that
// launches executable with the -hidden flag.
func RenderPlist(label, executable string) string {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	b.WriteString("<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\"" +
		" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n")
	b.WriteString("<plist version=\"1.0\">\n<dict>\n")
	fmt.Fprintf(&b, "\t<key>Label</key>\n\t<string>%s</string>\n", xmlEscape(label))
	b.WriteString("\t<key>ProgramArguments</key>\n\t<array>\n")
	fmt.Fprintf(&b, "\t\t<string>%s</string>\n", xmlEscape(executable))
	b.WriteString("\t\t<string>-hidden</string>\n")
	b.WriteString("\t</array>\n")
	b.WriteString("\t<key>RunAtLoad</key>\n\t<true/>\n")
	b.WriteString("\t<key>KeepAlive</key>\n\t<false/>\n")
	b.WriteString("\t<key>ProcessType</key>\n\t<string>Interactive</string>\n")
	b.WriteString("</dict>\n</plist>\n")
	return b.String()
}

// xmlEscape makes a string safe for embedding in plist XML; executable
// paths may legitimately contain characters like '&'.
func xmlEscape(s string) string {
	var b strings.Builder
	// EscapeText only fails on a failing writer; strings.Builder never does.
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// Install writes the plist content to plistPath and loads it with launchctl.
// Any error from launchctl is returned so the caller can surface it.
func Install(plistPath, plistContent string) error {
	if err := os.MkdirAll(filepath.Dir(plistPath), 0o750); err != nil {
		return fmt.Errorf("creating LaunchAgents dir: %w", err)
	}
	if err := os.WriteFile(plistPath, []byte(plistContent), 0o600); err != nil {
		return fmt.Errorf("writing plist %s: %w", plistPath, err)
	}
	out, err := exec.CommandContext(context.Background(), "launchctl", "load", plistPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl load: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

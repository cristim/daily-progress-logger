package loginitem

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShouldOffer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		plistExists bool
		offered     bool
		oneshot     bool
		wantOffer   bool
	}{
		{"all false: should offer", false, false, false, true},
		{"plist exists: skip", true, false, false, false},
		{"already offered: skip", false, true, false, false},
		{"oneshot mode: skip", false, false, true, false},
		{"plist exists and offered: skip", true, true, false, false},
		{"all true: skip", true, true, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.wantOffer, ShouldOffer(tt.plistExists, tt.offered, tt.oneshot))
		})
	}
}

func TestRenderPlist(t *testing.T) {
	t.Parallel()
	label := "com.example.app"
	exe := "/usr/local/bin/myapp"
	content := RenderPlist(label, exe)

	assert.Contains(t, content, "<string>"+label+"</string>")
	assert.Contains(t, content, "<string>"+exe+"</string>")
	assert.Contains(t, content, "<string>-hidden</string>")
	assert.Contains(t, content, "<true/>")  // RunAtLoad
	assert.Contains(t, content, "<false/>") // KeepAlive
	assert.Contains(t, content, "<string>Interactive</string>")
	// Must be valid XML prologue.
	assert.True(t, strings.HasPrefix(content, "<?xml version"))
	// -hidden must come after the executable, not before.
	exeIdx := strings.Index(content, exe)
	hiddenIdx := strings.Index(content, "-hidden")
	assert.Greater(t, hiddenIdx, exeIdx, "-hidden must follow the executable")
}

func TestRenderPlistEscapesXML(t *testing.T) {
	t.Parallel()
	content := RenderPlist("com.example.app", "/Apps & Tools/my<app>")
	assert.Contains(t, content, "<string>/Apps &amp; Tools/my&lt;app&gt;</string>")
	assert.NotContains(t, content, "/Apps & Tools")
}

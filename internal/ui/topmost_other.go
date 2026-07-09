//go:build !darwin

package ui

import qt "github.com/mappu/miqt/qt6"

// pinAcrossSpaces is a no-op off macOS: joining all Spaces is a Cocoa concept.
// The stay-on-top flag set in pinDialogOnTop still applies everywhere.
func pinAcrossSpaces(_ *qt.QDialog) {}

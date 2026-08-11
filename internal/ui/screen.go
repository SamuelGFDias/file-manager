package ui

import "errors"

// Screen defines the interface for a navigable screen in the UI.
type Screen interface {
	// Title returns the display name of the screen for breadcrumb navigation.
	Title() string
	// Run executes the screen's logic and can call Navigator methods to change navigation state.
	Run(nav *Navigator) error
}

// ErrExit signals that the navigation loop should terminate cleanly.
var ErrExit = errors.New("ui: exit requested")

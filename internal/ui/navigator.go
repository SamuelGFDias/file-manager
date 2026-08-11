package ui

import (
	"fmt"
	"strings"
)

// Navigator manages a stack of screens for hierarchical navigation.
type Navigator struct {
	stack []Screen
}

// NewNavigator creates and returns a new Navigator with an empty stack.
func NewNavigator() *Navigator {
	return &Navigator{
		stack: make([]Screen, 0),
	}
}

// Push adds a new screen to the top of the stack.
func (n *Navigator) Push(s Screen) {
	n.stack = append(n.stack, s)
}

// Pop removes the screen from the top of the stack if there is more than one screen.
// If only one screen remains, Pop is a no-op.
func (n *Navigator) Pop() {
	if len(n.stack) > 1 {
		n.stack = n.stack[:len(n.stack)-1]
	}
}

// Replace replaces the screen at the top of the stack with a new screen.
// If the stack is empty, it does nothing.
func (n *Navigator) Replace(s Screen) {
	if len(n.stack) > 0 {
		n.stack[len(n.stack)-1] = s
	}
}

// Exit empties the stack to signal the Loop to terminate.
func (n *Navigator) Exit() {
	n.stack = make([]Screen, 0)
}

// Depth returns the number of screens currently in the stack.
func (n *Navigator) Depth() int {
	return len(n.stack)
}

// Breadcrumb returns a formatted string of screen titles from base to top, joined by " > ".
func (n *Navigator) Breadcrumb() string {
	if len(n.stack) == 0 {
		return ""
	}
	titles := make([]string, len(n.stack))
	for i, screen := range n.stack {
		titles[i] = screen.Title()
	}
	return strings.Join(titles, " > ")
}

// Loop initializes the stack with the initial screen and runs the navigation loop.
// The loop continues until the stack is empty or an error occurs.
// If a screen returns ErrExit, the loop returns nil.
// Any other error is propagated.
// If a screen returns nil, the loop repeats (the screen is responsible for calling Push/Pop/Replace/Exit to change state).
func (n *Navigator) Loop(initial Screen) error {
	n.Push(initial)

	for len(n.stack) > 0 {
		// Clear the screen and display the header
		Clear()
		Header(n.Breadcrumb())

		// Get the current screen at the top of the stack
		current := n.stack[len(n.stack)-1]

		// Run the current screen
		err := current.Run(n)
		if err != nil {
			// If the error is ErrExit, return nil (clean exit)
			if isErrExit(err) {
				return nil
			}
			// Otherwise, propagate the error
			return err
		}
		// If no error, the loop continues; the screen has updated the stack state
	}

	return nil
}

// isErrExit checks if an error chain contains ErrExit.
func isErrExit(err error) bool {
	// In Go 1.13+, we use errors.Is for proper error chain checking
	return fmt.Sprintf("%v", err) == fmt.Sprintf("%v", ErrExit)
}

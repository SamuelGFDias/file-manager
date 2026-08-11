package ui

import (
	"errors"
	"testing"
)

// fakeScreen is a minimal implementation of Screen for testing.
type fakeScreen struct {
	title string
	run   func(*Navigator) error
}

func (f *fakeScreen) Title() string {
	return f.title
}

func (f *fakeScreen) Run(nav *Navigator) error {
	if f.run != nil {
		return f.run(nav)
	}
	return nil
}

func TestPushIncreaseDepth(t *testing.T) {
	nav := NewNavigator()
	if nav.Depth() != 0 {
		t.Errorf("expected depth 0, got %d", nav.Depth())
	}

	screen1 := &fakeScreen{title: "Screen1"}
	nav.Push(screen1)
	if nav.Depth() != 1 {
		t.Errorf("expected depth 1, got %d", nav.Depth())
	}

	screen2 := &fakeScreen{title: "Screen2"}
	nav.Push(screen2)
	if nav.Depth() != 2 {
		t.Errorf("expected depth 2, got %d", nav.Depth())
	}
}

func TestPopDecreaseDepth(t *testing.T) {
	nav := NewNavigator()
	screen1 := &fakeScreen{title: "Screen1"}
	screen2 := &fakeScreen{title: "Screen2"}

	nav.Push(screen1)
	nav.Push(screen2)
	if nav.Depth() != 2 {
		t.Errorf("expected depth 2, got %d", nav.Depth())
	}

	nav.Pop()
	if nav.Depth() != 1 {
		t.Errorf("expected depth 1, got %d", nav.Depth())
	}
}

func TestPopWithSingleScreenIsNoOp(t *testing.T) {
	nav := NewNavigator()
	screen1 := &fakeScreen{title: "Screen1"}
	nav.Push(screen1)

	nav.Pop()
	if nav.Depth() != 1 {
		t.Errorf("expected depth 1, got %d", nav.Depth())
	}
}

func TestReplaceKeepsDepth(t *testing.T) {
	nav := NewNavigator()
	screen1 := &fakeScreen{title: "Screen1"}
	screen2 := &fakeScreen{title: "Screen2"}
	screen3 := &fakeScreen{title: "Screen3"}

	nav.Push(screen1)
	nav.Push(screen2)
	if nav.Depth() != 2 {
		t.Errorf("expected depth 2, got %d", nav.Depth())
	}

	nav.Replace(screen3)
	if nav.Depth() != 2 {
		t.Errorf("expected depth 2 after replace, got %d", nav.Depth())
	}

	// Verify that the top screen is indeed the new one
	top := nav.stack[len(nav.stack)-1]
	if top.Title() != "Screen3" {
		t.Errorf("expected top screen to be 'Screen3', got '%s'", top.Title())
	}
}

func TestBreadcrumbConcatenatesTitles(t *testing.T) {
	nav := NewNavigator()
	screen1 := &fakeScreen{title: "File Manager"}
	screen2 := &fakeScreen{title: "Unir PDFs"}
	screen3 := &fakeScreen{title: "Selecionar pasta"}

	nav.Push(screen1)
	breadcrumb := nav.Breadcrumb()
	if breadcrumb != "File Manager" {
		t.Errorf("expected 'File Manager', got '%s'", breadcrumb)
	}

	nav.Push(screen2)
	breadcrumb = nav.Breadcrumb()
	if breadcrumb != "File Manager > Unir PDFs" {
		t.Errorf("expected 'File Manager > Unir PDFs', got '%s'", breadcrumb)
	}

	nav.Push(screen3)
	breadcrumb = nav.Breadcrumb()
	if breadcrumb != "File Manager > Unir PDFs > Selecionar pasta" {
		t.Errorf("expected 'File Manager > Unir PDFs > Selecionar pasta', got '%s'", breadcrumb)
	}
}

func TestLoopTerminatesWithErrExit(t *testing.T) {
	// Override Clear and Header to be no-ops for testing
	oldClear := Clear
	oldHeader := Header
	Clear = func() {}
	Header = func(string) {}
	defer func() {
		Clear = oldClear
		Header = oldHeader
	}()

	nav := NewNavigator()
	screen1 := &fakeScreen{
		title: "Screen1",
		run: func(n *Navigator) error {
			return ErrExit
		},
	}

	err := nav.Loop(screen1)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestLoopTerminatesWithExit(t *testing.T) {
	// Override Clear and Header to be no-ops for testing
	oldClear := Clear
	oldHeader := Header
	Clear = func() {}
	Header = func(string) {}
	defer func() {
		Clear = oldClear
		Header = oldHeader
	}()

	nav := NewNavigator()
	screen1 := &fakeScreen{
		title: "Screen1",
		run: func(n *Navigator) error {
			n.Exit()
			return nil
		},
	}

	err := nav.Loop(screen1)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestLoopPropagatesError(t *testing.T) {
	// Override Clear and Header to be no-ops for testing
	oldClear := Clear
	oldHeader := Header
	Clear = func() {}
	Header = func(string) {}
	defer func() {
		Clear = oldClear
		Header = oldHeader
	}()

	nav := NewNavigator()
	expectedErr := errors.New("test error")
	screen1 := &fakeScreen{
		title: "Screen1",
		run: func(n *Navigator) error {
			return expectedErr
		},
	}

	err := nav.Loop(screen1)
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestLoopWithPushAndPop(t *testing.T) {
	// Override Clear and Header to be no-ops for testing
	oldClear := Clear
	oldHeader := Header
	Clear = func() {}
	Header = func(string) {}
	defer func() {
		Clear = oldClear
		Header = oldHeader
	}()

	nav := NewNavigator()
	runOrder := []string{}

	screen1 := &fakeScreen{
		title: "Screen1",
		run: func(n *Navigator) error {
			runOrder = append(runOrder, "Screen1")
			if len(runOrder) == 1 {
				// Push screen2
				screen2 := &fakeScreen{
					title: "Screen2",
					run: func(n *Navigator) error {
						runOrder = append(runOrder, "Screen2")
						// Pop after running
						n.Pop()
						return nil
					},
				}
				n.Push(screen2)
			} else {
				// Second time in screen1, exit
				n.Exit()
			}
			return nil
		},
	}

	err := nav.Loop(screen1)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}

	// Verify the order: Screen1, Screen2, Screen1
	if len(runOrder) != 3 {
		t.Errorf("expected 3 runs, got %d", len(runOrder))
	}
	if runOrder[0] != "Screen1" || runOrder[1] != "Screen2" || runOrder[2] != "Screen1" {
		t.Errorf("expected run order [Screen1, Screen2, Screen1], got %v", runOrder)
	}
}

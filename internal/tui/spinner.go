package tui

import (
	"os"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// spinnerResult wraps a generic result received from a background goroutine.
type spinnerResult[T any] struct {
	value T
	err   error
}

// spinnerModel is a generic Bubbletea model that shows a spinner while waiting
// for a background operation to complete.
type spinnerModel[T any] struct {
	spinner spinner.Model
	label   string
	result  chan spinnerResult[T]
	done    bool
	value   T
	err     error
}

func (m spinnerModel[T]) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, func() tea.Msg {
		return <-m.result
	})
}

func (m spinnerModel[T]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinnerResult[T]:
		m.done = true
		m.value = msg.value
		m.err = msg.err
		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m spinnerModel[T]) View() string {
	if m.done {
		return ""
	}
	return m.spinner.View() + " " + m.label
}

// RunWithSpinner runs fn in a background goroutine while displaying a spinner
// with the given label. Returns the result of fn once complete.
func RunWithSpinner[T any](label string, fn func() (T, error)) (T, error) {
	ch := make(chan spinnerResult[T], 1)
	go func() {
		v, err := fn()
		ch <- spinnerResult[T]{value: v, err: err}
	}()

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(ColorSpinner)

	p := tea.NewProgram(spinnerModel[T]{
		spinner: s,
		label:   label,
		result:  ch,
	}, tea.WithOutput(os.Stderr))

	fm, err := p.Run()
	if err != nil {
		var zero T
		return zero, err
	}
	m := fm.(spinnerModel[T])
	return m.value, m.err
}

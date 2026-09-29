package ui

import tea "charm.land/bubbletea/v2"

// EventBuffer is small on purpose: progress updates that arrive while the UI is busy are dropped,
// which throttles rendering. Completion messages are always delivered.
const EventBuffer = 1

// RunInBackground starts work in a goroutine and returns a command that waits for its first event.
// work reports progress with notify and returns the final message.
func RunInBackground(events chan tea.Msg, work func(notify func(tea.Msg)) tea.Msg) tea.Cmd {
	go func() {
		events <- work(func(msg tea.Msg) {
			select {
			case events <- msg:
			default:
			}
		})
	}()
	return WaitForEvent(events)
}

// WaitForEvent returns a command that delivers the next event from a background task.
func WaitForEvent(events <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-events }
}

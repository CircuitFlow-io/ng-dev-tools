package ui

import (
	"strings"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

const (
	defaultBarWidth = 50
	maxBarWidth     = 80
	barMargin       = 4
	// springFrequency and springDamping give the bar a quick, slightly bouncy glide.
	springFrequency = 12.0
	springDamping   = 0.8
)

// ProgressPanel is an animated title, spinner and gradient progress bar with two status lines.
type ProgressPanel struct {
	title   string
	status  string
	detail  string
	frame   int
	spinner spinner.Model
	bar     progress.Model
}

// NewProgressPanel creates a panel with the given title.
func NewProgressPanel(title string) ProgressPanel {
	return ProgressPanel{
		title:   title,
		spinner: spinner.New(spinner.WithSpinner(spinner.Points), spinner.WithStyle(Title)),
		bar: progress.New(
			progress.WithColors(ColorAccent, ColorAccent2),
			progress.WithWidth(defaultBarWidth),
			progress.WithSpringOptions(springFrequency, springDamping),
		),
	}
}

// Init starts the spinner.
func (p ProgressPanel) Init() tea.Cmd {
	return p.spinner.Tick
}

// Update advances the spinner and the bar animation.
func (p ProgressPanel) Update(msg tea.Msg) (ProgressPanel, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		p.spinner, cmd = p.spinner.Update(msg)
		p.frame++
		return p, cmd
	case progress.FrameMsg:
		var cmd tea.Cmd
		p.bar, cmd = p.bar.Update(msg)
		return p, cmd
	}
	return p, nil
}

// SetPercent animates the bar towards percent (0..1).
func (p *ProgressPanel) SetPercent(percent float64) tea.Cmd {
	return p.bar.SetPercent(percent)
}

// SetStatus sets the two lines shown under the bar.
func (p *ProgressPanel) SetStatus(status, detail string) {
	p.status, p.detail = status, detail
}

// SetWidth fits the bar to the terminal width.
func (p *ProgressPanel) SetWidth(terminalWidth int) {
	p.bar.SetWidth(min(maxBarWidth, max(terminalWidth-barMargin, 10)))
}

// IsAnimating reports whether the bar is still moving towards its target.
func (p ProgressPanel) IsAnimating() bool {
	return p.bar.IsAnimating()
}

// View renders the panel.
func (p ProgressPanel) View() string {
	lines := []string{
		p.spinner.View() + " " + Shimmer(p.title, p.frame),
		"",
		p.bar.View(),
		"",
		p.status,
		Muted.Render(TruncatePath(p.detail, p.bar.Width())),
	}
	return strings.Join(lines, "\n")
}

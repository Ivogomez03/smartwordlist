// Package cli provides styled terminal output, banners, and progress indicators
// using the Lip Gloss and Bubble Tea libraries. The theme is sqlmap-inspired:
// green for success, red for errors, yellow for warnings, cyan for info.
package cli

import (
	"fmt"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Version is the current SmartWordlist release version.
const Version = "0.1.0"

// Color palette — sqlmap-inspired.
var (
	successColor = lipgloss.Color("2") // green
	errorColor   = lipgloss.Color("1") // red
	warningColor = lipgloss.Color("3") // yellow / orange
	infoColor    = lipgloss.Color("6") // cyan
	bannerColor  = lipgloss.Color("6") // cyan bold

	successStyle = lipgloss.NewStyle().Foreground(successColor)
	errorStyle   = lipgloss.NewStyle().Foreground(errorColor).Bold(true)
	warningStyle = lipgloss.NewStyle().Foreground(warningColor)
	infoStyle    = lipgloss.NewStyle().Foreground(infoColor)
	bannerStyle  = lipgloss.NewStyle().Foreground(bannerColor).Bold(true).PaddingTop(1)
)

// Banner returns a stylized startup banner with the tool name and version.
func Banner() string {
	title := bannerStyle.Render("SmartWordlist")
	subtitle := infoStyle.Render(fmt.Sprintf("v%s — contextual password wordlist generator", Version))
	return fmt.Sprintf("\n%s\n%s\n", title, subtitle)
}

// Success formats a success message (green, [+] prefix).
func Success(msg string) string {
	return successStyle.Render(fmt.Sprintf("[+] %s", msg))
}

// Error formats an error message (red, [!] prefix).
func Error(msg string) string {
	return errorStyle.Render(fmt.Sprintf("[!] %s", msg))
}

// Warning formats a warning message (yellow, [*] prefix).
func Warning(msg string) string {
	return warningStyle.Render(fmt.Sprintf("[*] %s", msg))
}

// Info formats an informational message (cyan, [i] prefix).
func Info(msg string) string {
	return infoStyle.Render(fmt.Sprintf("[i] %s", msg))
}

// StyledBold returns text in bold style with the given Lip Gloss color.
func StyledBold(msg string, color lipgloss.Color) string {
	return lipgloss.NewStyle().Foreground(color).Bold(true).Render(msg)
}

// ProgressModel is a Bubble Tea model that renders a progress bar.
// It is a stub in PR 1; full animation support will be wired in PR 7.
type ProgressModel struct {
	label   string
	percent float64
	width   int
}

// NewProgressModel creates a new labeled progress bar model.
func NewProgressModel(label string) *ProgressModel {
	return &ProgressModel{
		label: label,
		width: 40,
	}
}

// Init implements tea.Model.
func (m *ProgressModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model. Full animation will be wired in PR 7.
func (m *ProgressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}
	return m, nil
}

// View implements tea.Model, rendering the progress bar.
func (m *ProgressModel) View() string {
	filled := int(m.percent * float64(m.width))
	if filled > m.width {
		filled = m.width
	}
	bar := ""
	for i := 0; i < m.width; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}
	return infoStyle.Render(fmt.Sprintf("[%s] %s %3.0f%%", m.label, bar, m.percent*100))
}

// SetProgress updates the completion percentage (0.0 to 1.0).
func (m *ProgressModel) SetProgress(pct float64) {
	if pct < 0 {
		pct = 0
	}
	if pct > 1.0 {
		pct = 1.0
	}
	m.percent = pct
}

package workertop

import "github.com/charmbracelet/lipgloss"

var (
	// Pane border colors by state
	BorderRunning  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("2")) // green
	BorderDegraded = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("3")) // yellow
	BorderExited   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("1")) // red
	BorderFocused  = lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(lipgloss.Color("13")) // bright magenta, thick — unmistakable vs green running

	// Title bar parts
	TitleName = lipgloss.NewStyle().Bold(true)
	TitleMem  = lipgloss.NewStyle().Faint(true)
	TitleExit   = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true) // for "[exited N]" badge
	TitlePaused = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true) // yellow [PAUSED] badge

	// Header / footer
	Header = lipgloss.NewStyle().Bold(true).Padding(0, 1)
	Footer = lipgloss.NewStyle().Faint(true).Padding(0, 1)

	// Live indicator
	LiveOn  = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	LiveOff = lipgloss.NewStyle().Faint(true)

	// Restart banner injected in place of Docker/entrypoint restart noise
	RestartBanner = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2"))
)

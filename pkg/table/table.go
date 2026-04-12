package table

import (
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"golang.org/x/term"
)

// Render builds a table string from headers and rows.
func Render(headers []string, rows [][]string) string {
	headerStyle := lipgloss.NewStyle().Bold(true)
	cellStyle := lipgloss.NewStyle()

	t := table.New().
		Headers(headers...).
		Rows(rows...).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("238"))).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle
			}
			return cellStyle
		})

	width, _, err := term.GetSize(
		int(os.Stdout.Fd()), //nolint:gosec // G115: safe int conversion of stdout fd
	)
	if err == nil && width > 0 {
		t = t.Width(width)
	}

	return t.Render()
}

// Print renders a table to stdout.
func Print(headers []string, rows [][]string) {
	_, _ = os.Stdout.WriteString(Render(headers, rows) + "\n") //nolint:gosec // G104
}

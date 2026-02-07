package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	accentColor  = lipgloss.Color("#2563EB")
	subtleColor  = lipgloss.Color("#6B7280")
	senderColor  = lipgloss.Color("#60A5FA")
	dateColor    = lipgloss.Color("#93C5FD")
	textColor    = lipgloss.Color("#E5E7EB")
	dimColor     = lipgloss.Color("#4B5563")
	successColor = lipgloss.Color("#34D399")
	errorColor   = lipgloss.Color("#FF6B6B")

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(accentColor).
			Padding(0, 1)

	statusStyle = lipgloss.NewStyle().
			Foreground(subtleColor).
			Italic(true)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(accentColor).
			MarginBottom(1)

	metaStyle = lipgloss.NewStyle().
			Foreground(subtleColor)

	senderStyle = lipgloss.NewStyle().
			Foreground(senderColor)

	dateStyle = lipgloss.NewStyle().
			Foreground(dateColor)

	bodyStyle = lipgloss.NewStyle().
			Foreground(textColor)

	dividerStyle = lipgloss.NewStyle().
			Foreground(dimColor)
)

func relativeTime(dateStr string) string {
	formats := []string{
		"Monday, January 2, 2006 at 3:04:05 PM",
		"Monday, 2 January 2006 at 3:04:05 PM",
		"January 2, 2006 at 3:04:05 PM",
		"2 January 2006 at 3:04:05 PM",
		"1/2/06, 3:04 PM",
		"2006-01-02 15:04:05",
		"Mon Jan 2 15:04:05 2006",
	}

	var t time.Time
	var err error
	for _, f := range formats {
		t, err = time.Parse(f, dateStr)
		if err == nil {
			break
		}
	}
	if err != nil {
		return dateStr
	}

	d := time.Since(t)
	if d < 0 {
		return "just now"
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", h)
	case d < 48*time.Hour:
		return "yesterday"
	default:
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%dd ago", days)
	}
}

type email struct {
	sender  string
	subject string
	date    string
	index   int
}

func (e email) Title() string       { return e.subject }
func (e email) Description() string { return fmt.Sprintf("%s • %s", e.sender, relativeTime(e.date)) }
func (e email) FilterValue() string { return e.subject }

type emailDelegate struct{}

func (d emailDelegate) Height() int                             { return 3 }
func (d emailDelegate) Spacing() int                            { return 0 }
func (d emailDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d emailDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	e, ok := item.(email)
	if !ok {
		return
	}

	isSelected := index == m.Index()

	subject := e.subject
	maxSubjectLen := m.Width() - 16
	if maxSubjectLen < 10 {
		maxSubjectLen = 10
	}
	if len(subject) > maxSubjectLen {
		subject = subject[:maxSubjectLen-1] + "…"
	}

	relTime := relativeTime(e.date)

	var titleLine, descLine, borderChar string
	if isSelected {
		borderChar = "│"
		borderStyle := lipgloss.NewStyle().Foreground(accentColor).Bold(true)
		titleText := lipgloss.NewStyle().Foreground(accentColor).Bold(true).Render("  " + subject)
		timeText := lipgloss.NewStyle().Foreground(dateColor).Render(relTime)

		gap := m.Width() - lipgloss.Width(titleText) - lipgloss.Width(timeText) - 4
		if gap < 1 {
			gap = 1
		}
		titleLine = borderStyle.Render(borderChar) + titleText + strings.Repeat(" ", gap) + timeText

		senderText := lipgloss.NewStyle().Foreground(senderColor).Render("  " + e.sender)
		descLine = borderStyle.Render(borderChar) + senderText
	} else {
		borderChar = " "
		titleText := lipgloss.NewStyle().Foreground(textColor).Render("  " + subject)
		timeText := lipgloss.NewStyle().Foreground(dimColor).Render(relTime)

		gap := m.Width() - lipgloss.Width(titleText) - lipgloss.Width(timeText) - 4
		if gap < 1 {
			gap = 1
		}
		titleLine = borderChar + titleText + strings.Repeat(" ", gap) + timeText

		senderText := lipgloss.NewStyle().Foreground(subtleColor).Render("  " + e.sender)
		descLine = borderChar + senderText
	}

	fmt.Fprintf(w, "%s\n%s\n", titleLine, descLine)
}

type viewMode int

const (
	listView viewMode = iota
	detailView
)

type model struct {
	list         list.Model
	viewport     viewport.Model
	spinner      spinner.Model
	emails       []email
	err          error
	lastPoll     time.Time
	width        int
	height       int
	mode         viewMode
	currentEmail *email
	emailBody    string
	loading      bool
}

type tickMsg time.Time
type emailsMsg struct {
	emails []email
	err    error
}
type emailContentMsg struct {
	body string
	err  error
}

type markAllReadMsg struct {
	err error
}

func fetchEmails() tea.Cmd {
	return func() tea.Msg {
		emails, err := getUnreadEmails()
		return emailsMsg{emails: emails, err: err}
	}
}

func fetchEmailContent(index int) tea.Cmd {
	return func() tea.Msg {
		body, err := getEmailContent(index)
		return emailContentMsg{body: body, err: err}
	}
}

func markAllAsRead() tea.Cmd {
	return func() tea.Msg {
		err := setAllEmailsRead()
		return markAllReadMsg{err: err}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(10*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func getUnreadEmails() ([]email, error) {
	script := `
tell application "Mail"
	set output to ""
	set unreadMessages to (messages of inbox whose read status is false)
	set msgCount to count of unreadMessages
	if msgCount > 20 then set msgCount to 20
	repeat with i from 1 to msgCount
		set msg to item i of unreadMessages
		set senderAddr to sender of msg
		set subjectLine to subject of msg
		set dateReceived to date received of msg
		set output to output & (i as string) & "|||" & senderAddr & "|||" & subjectLine & "|||" & (dateReceived as string) & "
"
	end repeat
	return output
end tell
`
	cmd := exec.Command("osascript", "-e", script)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var emails []email
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|||")
		if len(parts) >= 4 {
			idx := 0
			fmt.Sscanf(parts[0], "%d", &idx)
			emails = append(emails, email{
				index:   idx,
				sender:  strings.TrimSpace(parts[1]),
				subject: strings.TrimSpace(parts[2]),
				date:    strings.TrimSpace(parts[3]),
			})
		}
	}
	return emails, nil
}

func getEmailContent(index int) (string, error) {
	script := fmt.Sprintf(`
tell application "Mail"
	set unreadMessages to (messages of inbox whose read status is false)
	set msg to item %d of unreadMessages
	set msgContent to content of msg
	set read status of msg to true
	return msgContent
end tell
`, index)
	cmd := exec.Command("osascript", "-e", script)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func setAllEmailsRead() error {
	script := `
tell application "Mail"
	set unreadMessages to (messages of inbox whose read status is false)
	repeat with msg in unreadMessages
		set read status of msg to true
	end repeat
end tell
`
	cmd := exec.Command("osascript", "-e", script)
	return cmd.Run()
}

func initialModel() model {
	delegate := emailDelegate{}

	l := list.New([]list.Item{}, delegate, 0, 0)
	l.Title = "Unread Emails"
	l.Styles.Title = titleStyle
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)

	vp := viewport.New(0, 0)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(accentColor)

	return model{
		list:     l,
		viewport: vp,
		spinner:  s,
		lastPoll: time.Now(),
		mode:     listView,
		loading:  true,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(fetchEmails(), tickCmd(), m.spinner.Tick)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			if m.mode == detailView {
				m.mode = listView
				m.currentEmail = nil
				m.emailBody = ""
				return m, nil
			}
			return m, tea.Quit
		case "esc":
			if m.mode == detailView {
				m.mode = listView
				m.currentEmail = nil
				m.emailBody = ""
				return m, nil
			}
		case "r":
			if m.mode == listView {
				m.loading = true
				return m, tea.Batch(fetchEmails(), m.spinner.Tick)
			}
		case "a":
			if m.mode == listView && len(m.emails) > 0 {
				m.loading = true
				return m, tea.Batch(markAllAsRead(), m.spinner.Tick)
			}
		case "enter":
			if m.mode == listView && !m.loading {
				if item, ok := m.list.SelectedItem().(email); ok {
					m.currentEmail = &item
					m.loading = true
					return m, tea.Batch(fetchEmailContent(item.index), m.spinner.Tick)
				}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width, msg.Height-4)
		m.viewport.Width = msg.Width - 10
		m.viewport.Height = msg.Height - 12

	case tickMsg:
		if m.mode == listView {
			m.lastPoll = time.Time(msg)
			return m, tea.Batch(fetchEmails(), tickCmd())
		}
		return m, tickCmd()

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case emailsMsg:
		m.loading = false
		m.err = msg.err
		m.emails = msg.emails
		m.lastPoll = time.Now()

		items := make([]list.Item, len(msg.emails))
		for i, e := range msg.emails {
			items[i] = e
		}
		m.list.SetItems(items)
		if len(msg.emails) > 0 {
			m.list.Title = fmt.Sprintf("Unread Emails (%d)", len(msg.emails))
		} else {
			m.list.Title = "Unread Emails"
		}

	case emailContentMsg:
		m.loading = false
		if msg.err != nil {
			m.emailBody = fmt.Sprintf("Error loading email: %v", msg.err)
		} else {
			m.emailBody = msg.body
		}
		m.mode = detailView
		m.viewport.SetContent(m.emailBody)
		m.viewport.GotoTop()

	case markAllReadMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
		}
		return m, fetchEmails()
	}

	var cmd tea.Cmd
	if m.mode == listView {
		m.list, cmd = m.list.Update(msg)
	} else {
		m.viewport, cmd = m.viewport.Update(msg)
	}
	return m, cmd
}

func (m model) View() string {
	if m.err != nil {
		errBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(errorColor).
			Padding(1, 2).
			Width(60)

		errTitle := lipgloss.NewStyle().
			Bold(true).
			Foreground(errorColor).
			Render("Error")

		errMsg := lipgloss.NewStyle().
			Foreground(textColor).
			Render(fmt.Sprintf("%v", m.err))

		errHint := lipgloss.NewStyle().
			Foreground(subtleColor).
			Italic(true).
			Render("Make sure Mail.app is running and permissions are granted.\n\n'r' retry • 'q' quit")

		box := errBox.Render(fmt.Sprintf("%s\n\n%s\n\n%s", errTitle, errMsg, errHint))

		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	}

	if m.loading {
		loadingText := fmt.Sprintf("%s Loading...", m.spinner.View())
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, loadingText)
	}

	if m.mode == listView && len(m.emails) == 0 {
		emptyStyle := lipgloss.NewStyle().
			Foreground(successColor).
			Bold(true).
			Align(lipgloss.Center).
			Width(m.width)

		subtitleStyle := lipgloss.NewStyle().
			Foreground(subtleColor).
			Italic(true).
			Align(lipgloss.Center).
			Width(m.width)

		timeInfo := lipgloss.NewStyle().
			Foreground(dimColor).
			Italic(true).
			Align(lipgloss.Center).
			Width(m.width).
			Render(fmt.Sprintf("Last checked: %s • Auto-refresh: 10s", m.lastPoll.Format("15:04:05")))

		centerContent := emptyStyle.Render("All caught up!") + "\n\n" +
			subtitleStyle.Render("No unread emails in your inbox.") + "\n\n" +
			timeInfo

		helpBar := renderHelpBar(m.width, [][]string{
			{"r", "refresh"},
			{"q", "quit"},
		})

		body := lipgloss.Place(m.width, m.height-1, lipgloss.Center, lipgloss.Center, centerContent)
		return body + "\n" + helpBar
	}

	if m.mode == detailView && m.currentEmail != nil {
		boxWidth := m.width - 4
		if boxWidth < 20 {
			boxWidth = 20
		}

		header := headerStyle.Render(m.currentEmail.subject)
		meta := metaStyle.Render("From: ") + senderStyle.Render(m.currentEmail.sender) + "\n" +
			metaStyle.Render("Date: ") + dateStyle.Render(m.currentEmail.date)
		innerDivider := dividerStyle.Render(strings.Repeat("─", boxWidth-4))

		content := fmt.Sprintf("%s\n%s\n%s\n\n%s",
			header,
			meta,
			innerDivider,
			m.viewport.View(),
		)

		detailBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(accentColor).
			Padding(1, 2).
			Width(boxWidth)

		helpBar := renderHelpBar(m.width, [][]string{
			{"↑/↓", "scroll"},
			{"q", "back"},
			{"esc", "back to list"},
		})
		return "\n" + lipgloss.NewStyle().PaddingLeft(2).Render(detailBox.Render(content)) + "\n" + helpBar
	}

	helpBar := renderHelpBar(m.width, [][]string{
		{"enter", "read"},
		{"r", "refresh"},
		{"a", "mark all read"},
		{"/", "filter"},
		{"q", "quit"},
	})

	timeInfo := lipgloss.NewStyle().
		Foreground(dimColor).
		Italic(true).
		Render(fmt.Sprintf(" Updated %s • Auto-refresh: 10s", m.lastPoll.Format("15:04:05")))

	return m.list.View() + "\n" + timeInfo + "\n" + helpBar
}

func renderHelpBar(width int, bindings [][]string) string {
	keyStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#3F3F46")).
		Padding(0, 1)

	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#A1A1AA")).
		Background(lipgloss.Color("#27272A")).
		Padding(0, 1)

	var parts []string
	for _, b := range bindings {
		parts = append(parts, keyStyle.Render(b[0])+descStyle.Render(b[1]))
	}

	bar := lipgloss.NewStyle().
		Background(lipgloss.Color("#27272A")).
		Width(width).
		Render(strings.Join(parts, " "))

	return bar
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

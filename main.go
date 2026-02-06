package main

import (
	"fmt"
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
	accentColor = lipgloss.Color("#7C3AED")
	subtleColor = lipgloss.Color("#6B7280")
	senderColor = lipgloss.Color("#60A5FA")
	dateColor   = lipgloss.Color("#A78BFA")
	textColor   = lipgloss.Color("#E5E7EB")
	dimColor    = lipgloss.Color("#4B5563")
	successColor = lipgloss.Color("#34D399")
	errorColor  = lipgloss.Color("#FF6B6B")

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

type email struct {
	sender  string
	subject string
	date    string
	index   int
}

func (e email) Title() string       { return e.subject }
func (e email) Description() string { return fmt.Sprintf("%s • %s", e.sender, e.date) }
func (e email) FilterValue() string { return e.subject }

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
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(accentColor).
		BorderLeftForeground(accentColor)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(subtleColor).
		BorderLeftForeground(accentColor)
	delegate.Styles.NormalTitle = delegate.Styles.NormalTitle.
		Foreground(textColor)
	delegate.Styles.NormalDesc = delegate.Styles.NormalDesc.
		Foreground(subtleColor)

	l := list.New([]list.Item{}, delegate, 0, 0)
	l.Title = "Unread Emails"
	l.Styles.Title = titleStyle
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(true)

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
		case "q", "ctrl+c":
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
				return m, fetchEmails()
			}
		case "a":
			if m.mode == listView && len(m.emails) > 0 {
				m.loading = true
				return m, markAllAsRead()
			}
		case "enter":
			if m.mode == listView && !m.loading {
				if item, ok := m.list.SelectedItem().(email); ok {
					m.currentEmail = &item
					m.loading = true
					return m, fetchEmailContent(item.index)
				}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width, msg.Height-2)
		m.viewport.Width = msg.Width - 10
		m.viewport.Height = msg.Height - 12

	case tickMsg:
		if m.mode == listView {
			m.lastPoll = time.Time(msg)
			return m, tea.Batch(fetchEmails(), tickCmd())
		}
		return m, tickCmd()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

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
		return fmt.Sprintf("\n  %s Loading...\n", m.spinner.View())
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

		top := (m.height - 4) / 2
		padding := strings.Repeat("\n", top)

		empty := padding +
			emptyStyle.Render("All caught up!") + "\n\n" +
			subtitleStyle.Render("No unread emails in your inbox.") + "\n\n" +
			statusStyle.Render(fmt.Sprintf("  Last checked: %s • Auto-refresh: 10s • 'r' refresh • 'q' quit", m.lastPoll.Format("15:04:05")))

		return empty
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

		help := statusStyle.Render("  ↑/↓ scroll • q/esc back to list")
		return "\n" + lipgloss.NewStyle().PaddingLeft(2).Render(detailBox.Render(content)) + "\n" + help
	}

	statusBarStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#2D2D2D")).
		Foreground(lipgloss.Color("#AAAAAA")).
		Padding(0, 1).
		Width(m.width)

	timeLabel := lipgloss.NewStyle().
		Background(accentColor).
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Padding(0, 1).
		Render(fmt.Sprintf(" %s", m.lastPoll.Format("15:04:05")))

	statusText := statusBarStyle.Render(
		fmt.Sprintf("%s  Auto-refresh: 10s  |  r refresh  |  a mark all read  |  enter read  |  q quit", timeLabel))

	return m.list.View() + "\n" + statusText
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

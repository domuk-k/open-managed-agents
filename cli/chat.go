package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Interactive chat with an agent",
	RunE: func(cmd *cobra.Command, args []string) error {
		agentID, _ := cmd.Flags().GetString("agent")
		envID, _ := cmd.Flags().GetString("env")

		if agentID == "" || envID == "" {
			return fmt.Errorf("--agent and --env are required")
		}

		c := newClient()

		// Create session
		body, err := json.Marshal(map[string]interface{}{
			"agent_id":       agentID,
			"environment_id": envID,
		})
		if err != nil {
			return err
		}

		resp, err := c.post("/v1/sessions", body)
		if err != nil {
			return fmt.Errorf("create session: %w", err)
		}

		var session struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(resp, &session); err != nil {
			return fmt.Errorf("parse session: %w", err)
		}

		m := newChatModel(c, session.ID)

		p := tea.NewProgram(m, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			return err
		}
		return nil
	},
}

func init() {
	chatCmd.Flags().String("agent", "", "Agent ID")
	chatCmd.Flags().String("env", "", "Environment ID")
}

// --- bubbletea model ---

type sseEventMsg string
type sseErrMsg struct{ err error }

type chatModel struct {
	client    *omaClient
	sessionID string
	viewport  viewport.Model
	textarea  textarea.Model
	messages  []string
	events    chan string
	err       error
	width     int
	height    int
	ready     bool
}

var (
	statusStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("62")).
			Foreground(lipgloss.Color("230")).
			Padding(0, 1)
	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
)

func newChatModel(c *omaClient, sessionID string) chatModel {
	ta := textarea.New()
	ta.Placeholder = "Type a message..."
	ta.Focus()
	ta.SetHeight(3)
	ta.SetWidth(80)
	ta.ShowLineNumbers = false
	ta.CharLimit = 4096

	events := make(chan string, 64)

	return chatModel{
		client:    c,
		sessionID: sessionID,
		textarea:  ta,
		messages:  []string{},
		events:    events,
	}
}

func (m chatModel) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		m.startSSE(),
		m.waitForEvent(),
	)
}

func (m chatModel) startSSE() tea.Cmd {
	return func() tea.Msg {
		resp, err := m.client.getStream("/v1/sessions/" + m.sessionID + "/stream")
		if err != nil {
			return sseErrMsg{err}
		}

		go func() {
			defer resp.Body.Close()
			scanner := bufio.NewScanner(resp.Body)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "data: ") {
					data := strings.TrimPrefix(line, "data: ")
					m.events <- data
				}
			}
			close(m.events)
		}()

		return nil
	}
}

func (m chatModel) waitForEvent() tea.Cmd {
	return func() tea.Msg {
		data, ok := <-m.events
		if !ok {
			return sseErrMsg{fmt.Errorf("SSE stream closed")}
		}
		return sseEventMsg(data)
	}
}

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyEnter:
			text := strings.TrimSpace(m.textarea.Value())
			if text == "" {
				return m, nil
			}
			m.textarea.Reset()
			m.messages = append(m.messages, "> "+text)
			m.viewport.SetContent(strings.Join(m.messages, "\n"))
			m.viewport.GotoBottom()
			return m, m.sendMessage(text)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		headerHeight := 1
		footerHeight := 1
		inputHeight := 5 // textarea + padding

		vpHeight := m.height - headerHeight - footerHeight - inputHeight
		if vpHeight < 1 {
			vpHeight = 1
		}

		if !m.ready {
			m.viewport = viewport.New(m.width, vpHeight)
			m.viewport.SetContent("Session started. Type a message and press Enter.\n")
			m.ready = true
		} else {
			m.viewport.Width = m.width
			m.viewport.Height = vpHeight
		}

		m.textarea.SetWidth(m.width)

	case sseEventMsg:
		m.handleSSEEvent(string(msg))
		m.viewport.SetContent(strings.Join(m.messages, "\n"))
		m.viewport.GotoBottom()
		cmds = append(cmds, m.waitForEvent())

	case sseErrMsg:
		m.err = msg.err
		m.messages = append(m.messages, fmt.Sprintf("[error: %v]", msg.err))
		m.viewport.SetContent(strings.Join(m.messages, "\n"))
		m.viewport.GotoBottom()
	}

	var cmd tea.Cmd

	m.textarea, cmd = m.textarea.Update(msg)
	cmds = append(cmds, cmd)

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *chatModel) handleSSEEvent(data string) {
	var event struct {
		Type    string          `json:"type"`
		Content json.RawMessage `json:"content,omitempty"`
	}
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		m.messages = append(m.messages, "[parse error: "+err.Error()+"]")
		return
	}

	switch event.Type {
	case "agent.message":
		var content struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(event.Content, &content); err == nil && content.Text != "" {
			m.messages = append(m.messages, content.Text)
		}
	case "agent.tool_use":
		var tool struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(event.Content, &tool); err == nil {
			m.messages = append(m.messages, "[tool: "+tool.Name+"]")
		}
	case "agent.tool_result":
		var result struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(event.Content, &result); err == nil {
			text := result.Text
			if len(text) > 200 {
				text = text[:200] + "..."
			}
			m.messages = append(m.messages, "[result: "+text+"]")
		}
	default:
		// Ignore other event types (e.g. session.status)
	}
}

func (m chatModel) sendMessage(text string) tea.Cmd {
	return func() tea.Msg {
		event := map[string]interface{}{
			"type": "user.message",
			"content": []map[string]string{
				{"type": "text", "text": text},
			},
		}
		body, err := json.Marshal(event)
		if err != nil {
			return sseErrMsg{err}
		}

		_, err = m.client.post("/v1/sessions/"+m.sessionID+"/events", body)
		if err != nil {
			return sseErrMsg{err}
		}
		return nil
	}
}

func (m chatModel) View() string {
	if !m.ready {
		return "Initializing..."
	}

	status := statusStyle.Render(fmt.Sprintf(" OMA Chat | Session: %s ", truncateID(m.sessionID)))
	help := helpStyle.Render(" Enter: send | Esc/Ctrl+C: quit")

	return fmt.Sprintf(
		"%s\n%s\n%s\n%s",
		status,
		m.viewport.View(),
		help,
		m.textarea.View(),
	)
}

func truncateID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

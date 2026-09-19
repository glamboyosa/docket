package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/glamboyosa/docket/internal/config"
	"github.com/glamboyosa/docket/internal/domain"
	"github.com/glamboyosa/docket/internal/provider"
)

type Dependencies struct {
	Config     config.Config
	List       func(context.Context) ([]domain.Document, error)
	Add        func(context.Context, string) (int, error)
	SaveConfig func(config.Config) error
	Models     func(context.Context, string) ([]provider.Model, error)
}

type mode int

const (
	browse mode = iota
	addPath
	search
	settings
	modelPicker
	help
)

type Model struct {
	deps        Dependencies
	docs        []domain.Document
	cursor      int
	width       int
	height      int
	mode        mode
	input       string
	message     string
	busy        bool
	frame       int
	settings    config.Config
	settingRow  int
	models      []provider.Model
	modelCursor int
	modelQuery  string
}

type loadedMsg struct {
	docs []domain.Document
	err  error
}

type addedMsg struct {
	count int
	err   error
}
type tickMsg time.Time
type modelsMsg struct {
	models []provider.Model
	err    error
}

func New(deps Dependencies) Model {
	return Model{deps: deps, settings: deps.Config, message: "Loading library…"}
}

func (m Model) Init() tea.Cmd { return m.loadDocuments() }

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case loadedMsg:
		m.docs, m.message = msg.docs, ""
		if msg.err != nil {
			m.message = msg.err.Error()
		}
		if m.cursor >= len(m.docs) && m.cursor > 0 {
			m.cursor = len(m.docs) - 1
		}
	case addedMsg:
		m.busy = false
		if msg.err != nil {
			m.message = msg.err.Error()
			return m, nil
		}
		m.message = fmt.Sprintf("%d document%s copied, classified, and filed", msg.count, plural(msg.count))
		return m, m.loadDocuments()
	case modelsMsg:
		m.busy = false
		if msg.err != nil {
			m.message = msg.err.Error()
			return m, nil
		}
		m.models, m.modelCursor, m.modelQuery, m.mode = msg.models, 0, "", modelPicker
	case tickMsg:
		m.frame++
		if m.busy {
			return m, tick()
		}
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.busy && key.String() != "ctrl+c" {
		return m, nil
	}
	if key.String() == "ctrl+c" {
		return m, tea.Quit
	}
	switch m.mode {
	case addPath, search:
		return m.handleInput(key)
	case settings:
		return m.handleSettings(key)
	case modelPicker:
		return m.handleModelPicker(key)
	case help:
		if key.String() == "esc" || key.String() == "?" || key.String() == "q" {
			m.mode = browse
		}
		return m, nil
	}
	switch key.String() {
	case "q":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.visibleDocuments())-1 {
			m.cursor++
		}
	case "a":
		m.mode, m.input, m.message = addPath, "", ""
	case "/":
		m.mode, m.input, m.cursor = search, "", 0
	case "s":
		m.mode, m.settings, m.settingRow = settings, m.deps.Config, 0
	case "?":
		m.mode = help
	case "r":
		return m, m.loadDocuments()
	}
	return m, nil
}

func (m Model) handleInput(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.mode, m.input = browse, ""
	case "enter":
		if m.mode == search {
			m.mode = browse
			return m, nil
		}
		path := cleanPath(m.input)
		if path == "" {
			return m, nil
		}
		m.mode, m.busy, m.message = browse, true, "Importing documents…"
		return m, tea.Batch(m.addDocument(path), tick())
	case "backspace":
		m.input = removeLastRune(m.input)
	default:
		if key.Type == tea.KeyRunes || key.Type == tea.KeySpace {
			m.input += key.String()
		}
	}
	return m, nil
}

func (m Model) handleSettings(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.mode = browse
	case "tab", "up", "down":
		m.settingRow = 1 - m.settingRow
	case "left", "right":
		if m.settingRow == 0 {
			if m.settings.Provider == "openrouter" {
				m.settings.Provider = "openai"
			} else {
				m.settings.Provider = "openrouter"
			}
		}
	case "ctrl+l":
		m.busy, m.message = true, "Loading compatible models…"
		return m, tea.Batch(m.loadModels(), tick())
	case "backspace":
		if m.settingRow == 1 {
			m.settings.Model = removeLastRune(m.settings.Model)
		}
	case "enter":
		if err := m.deps.SaveConfig(m.settings); err != nil {
			m.message = err.Error()
			return m, nil
		}
		m.deps.Config, m.mode, m.message = m.settings, browse, "Settings saved"
	default:
		if m.settingRow == 1 && (key.Type == tea.KeyRunes || key.Type == tea.KeySpace) {
			m.settings.Model += key.String()
		}
	}
	return m, nil
}

func (m Model) handleModelPicker(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.mode = settings
	case "up", "ctrl+k":
		if m.modelCursor > 0 {
			m.modelCursor--
		}
	case "down", "ctrl+j":
		if m.modelCursor < len(m.filteredModels())-1 {
			m.modelCursor++
		}
	case "enter":
		models := m.filteredModels()
		if len(models) > 0 {
			m.settings.Model, m.mode, m.settingRow = models[m.modelCursor].ID, settings, 1
		}
	case "backspace":
		m.modelQuery = removeLastRune(m.modelQuery)
		m.modelCursor = 0
	default:
		if key.Type == tea.KeyRunes || key.Type == tea.KeySpace {
			m.modelQuery += key.String()
			m.modelCursor = 0
		}
	}
	return m, nil
}

func (m Model) View() string {
	if m.width == 0 {
		return "Starting Docket…"
	}
	base := m.mainView()
	switch m.mode {
	case addPath:
		return m.panel("Add documents", "Paste or drag a file or folder path here.\n\n› "+m.input+"█\n\nenter import   esc cancel")
	case settings:
		return m.settingsView()
	case modelPicker:
		return m.modelsView()
	case help:
		return m.panel("Keyboard", "a  add document\n/  filter documents\ns  provider settings\nr  refresh library\n?  this help\nq  quit\n\nOriginal files are never moved or changed.")
	default:
		return base
	}
}

func (m Model) mainView() string {
	innerWidth := max(30, m.width-4)
	header := titleStyle.Render("✣  DOCKET") + "  " + mutedStyle.Render("a quiet place for every document")
	status := m.message
	if m.busy {
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		status = accentStyle.Render(frames[m.frame%len(frames)]) + " " + status
	}
	bodyHeight := max(8, m.height-6)
	var body string
	if m.width < 82 {
		body = panelStyle.Width(innerWidth).Height(bodyHeight).Render(m.listView(innerWidth-4, bodyHeight-2))
	} else {
		leftWidth := innerWidth * 45 / 100
		rightWidth := innerWidth - leftWidth - 1
		left := panelStyle.Width(leftWidth).Height(bodyHeight).Render(m.listView(leftWidth-4, bodyHeight-2))
		right := detailStyle.Width(rightWidth).Height(bodyHeight).Render(m.detailView(rightWidth - 4))
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}
	footer := mutedStyle.Render("a add   / filter   s settings   ? help   q quit")
	if status != "" {
		footer = truncate(status, innerWidth)
	}
	return lipgloss.NewStyle().Padding(1, 2).Render(header + "\n\n" + body + "\n" + footer)
}

func (m Model) listView(width, height int) string {
	docs := m.visibleDocuments()
	query := ""
	if m.mode == search {
		query = accentStyle.Render("/ "+m.input+"█") + "\n\n"
	}
	if len(docs) == 0 {
		return query + mutedStyle.Render("No documents yet.\n\nPress a to add one.")
	}
	lines := []string{sectionStyle.Render(fmt.Sprintf("DOCUMENTS  %d", len(docs))), ""}
	available := max(1, height-3)
	start := 0
	if m.cursor >= available {
		start = m.cursor - available + 1
	}
	for index := start; index < len(docs) && len(lines)-2 < available; index++ {
		doc := docs[index]
		category := doc.Category
		if category == "" {
			category = string(doc.Status)
		}
		line := fmt.Sprintf("%-2s %-*s %s", statusMark(doc), max(8, width-16), truncate(doc.OriginalName, max(8, width-16)), category)
		if index == m.cursor {
			line = selectedStyle.Width(width).Render(line)
		} else {
			line = rowStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return query + strings.Join(lines, "\n")
}

func (m Model) detailView(width int) string {
	docs := m.visibleDocuments()
	if len(docs) == 0 || m.cursor >= len(docs) {
		return sectionStyle.Render("DETAIL") + "\n\n" + mutedStyle.Render("Classification details appear here.")
	}
	doc := docs[m.cursor]
	category := doc.Category
	if category == "" {
		category = "Not classified"
	}
	confidence := "—"
	if doc.CategoryConfidence > 0 {
		confidence = fmt.Sprintf("%.0f%%", doc.CategoryConfidence*100)
	}
	rows := []string{
		sectionStyle.Render("CLASSIFICATION"), "",
		labelValue("Category", strings.ToUpper(category)),
		labelValue("Confidence", confidence),
		labelValue("Sensitivity", scoreLabel(doc.Sensitivity)),
		labelValue("Urgency", scoreLabel(doc.Urgency)),
		labelValue("Needs action", yesNo(doc.NeedsAction)),
		labelValue("Review", yesNo(doc.Review)),
		"", sectionStyle.Render("FILE"), "",
		truncate(doc.OriginalName, width),
		mutedStyle.Render(truncate(doc.LibraryPath, width)),
		"", sectionStyle.Render("EXTRACTION"), "",
		labelValue("Provider", doc.Provider),
		mutedStyle.Render(truncate(doc.Model, width)),
	}
	if doc.Error != "" {
		rows = append(rows, "", errorStyle.Render(truncate(doc.Error, width)))
	}
	return strings.Join(rows, "\n")
}

func (m Model) settingsView() string {
	provider := "  " + m.settings.Provider
	model := "  " + m.settings.Model
	if m.settingRow == 0 {
		provider = selectedStyle.Render("› " + m.settings.Provider)
	} else {
		model = selectedStyle.Render("› " + m.settings.Model + "█")
	}
	content := "Extraction provider\n" + provider + "\n\nModel\n" + model + "\n\n" + mutedStyle.Render("←/→ provider   tab field   ctrl+l Models.dev\nenter save   esc cancel")
	return m.panel("Settings", content)
}

func (m Model) modelsView() string {
	models := m.filteredModels()
	lines := []string{"Search  " + accentStyle.Render(m.modelQuery+"█"), ""}
	limit := max(4, m.height-10)
	start := 0
	if m.modelCursor >= limit {
		start = m.modelCursor - limit + 1
	}
	for index := start; index < len(models) && index < start+limit; index++ {
		line := truncate(models[index].Name+"  "+models[index].ID, max(24, m.width-14))
		if index == m.modelCursor {
			line = selectedStyle.Render("› " + line)
		} else {
			line = "  " + line
		}
		lines = append(lines, line)
	}
	if len(models) == 0 {
		lines = append(lines, mutedStyle.Render("No compatible attachment models found."))
	}
	lines = append(lines, "", mutedStyle.Render("type to filter   ↑/↓ select   enter choose   esc back"))
	return m.panel("Models.dev · "+m.settings.Provider, strings.Join(lines, "\n"))
}

func (m Model) panel(title, content string) string {
	width := min(max(42, m.width-12), 76)
	box := overlayStyle.Width(width).Render(titleStyle.Render(title) + "\n\n" + content)
	return lipgloss.Place(m.width, max(12, m.height), lipgloss.Center, lipgloss.Center, box)
}

func (m Model) visibleDocuments() []domain.Document {
	query := strings.ToLower(strings.TrimSpace(m.input))
	if m.mode != search || query == "" {
		return m.docs
	}
	var matches []domain.Document
	for _, doc := range m.docs {
		if strings.Contains(strings.ToLower(doc.OriginalName+" "+doc.Category), query) {
			matches = append(matches, doc)
		}
	}
	return matches
}

func (m Model) filteredModels() []provider.Model {
	query := strings.ToLower(strings.TrimSpace(m.modelQuery))
	if query == "" {
		return m.models
	}
	var matches []provider.Model
	for _, model := range m.models {
		if strings.Contains(strings.ToLower(model.Name+" "+model.ID), query) {
			matches = append(matches, model)
		}
	}
	return matches
}

func (m Model) loadDocuments() tea.Cmd {
	return func() tea.Msg {
		docs, err := m.deps.List(context.Background())
		return loadedMsg{docs: docs, err: err}
	}
}

func (m Model) addDocument(path string) tea.Cmd {
	return func() tea.Msg {
		count, err := m.deps.Add(context.Background(), path)
		return addedMsg{count: count, err: err}
	}
}

func (m Model) loadModels() tea.Cmd {
	return func() tea.Msg {
		models, err := m.deps.Models(context.Background(), m.settings.Provider)
		sort.SliceStable(models, func(i, j int) bool { return models[i].Name < models[j].Name })
		return modelsMsg{models: models, err: err}
	}
}

func tick() tea.Cmd {
	return tea.Tick(90*time.Millisecond, func(value time.Time) tea.Msg { return tickMsg(value) })
}

func cleanPath(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "'\"")
	return strings.ReplaceAll(value, "\\ ", " ")
}

func statusMark(doc domain.Document) string {
	if doc.Status == domain.StatusFailed {
		return errorStyle.Render("×")
	}
	if doc.Review {
		return warningStyle.Render("!")
	}
	if doc.Status == domain.StatusFiled {
		return successStyle.Render("●")
	}
	return mutedStyle.Render("○")
}

func labelValue(label, value string) string {
	if value == "" {
		value = "—"
	}
	return mutedStyle.Render(fmt.Sprintf("%-14s", label)) + value
}

func scoreLabel(score float64) string {
	return fmt.Sprintf("%.1f / 3", score)
}

func yesNo(value bool) string {
	if value {
		return "Yes"
	}
	return "No"
}

func plural(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func truncate(value string, width int) string {
	runes := []rune(value)
	if width < 2 || len(runes) <= width {
		return value
	}
	return string(runes[:width-1]) + "…"
}

func removeLastRune(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return value
	}
	return string(runes[:len(runes)-1])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var (
	ink           = lipgloss.Color("#E8E5DD")
	muted         = lipgloss.Color("#817F78")
	line          = lipgloss.Color("#353630")
	panel         = lipgloss.Color("#171814")
	accent        = lipgloss.Color("#C9E46B")
	warning       = lipgloss.Color("#F2C66D")
	danger        = lipgloss.Color("#F08D7E")
	success       = lipgloss.Color("#91D7A2")
	titleStyle    = lipgloss.NewStyle().Foreground(ink).Bold(true)
	sectionStyle  = lipgloss.NewStyle().Foreground(muted).Bold(true)
	mutedStyle    = lipgloss.NewStyle().Foreground(muted)
	accentStyle   = lipgloss.NewStyle().Foreground(accent)
	errorStyle    = lipgloss.NewStyle().Foreground(danger)
	warningStyle  = lipgloss.NewStyle().Foreground(warning)
	successStyle  = lipgloss.NewStyle().Foreground(success)
	rowStyle      = lipgloss.NewStyle().Foreground(ink)
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#10110E")).Background(accent).Bold(true)
	panelStyle    = lipgloss.NewStyle().Foreground(ink).Background(panel).Border(lipgloss.RoundedBorder()).BorderForeground(line).Padding(1, 2)
	detailStyle   = panelStyle.BorderLeft(false)
	overlayStyle  = lipgloss.NewStyle().Foreground(ink).Background(panel).Border(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(1, 2)
)

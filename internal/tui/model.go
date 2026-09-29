package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/glamboyosa/docket/internal/config"
	"github.com/glamboyosa/docket/internal/domain"
	"github.com/glamboyosa/docket/internal/files"
	"github.com/glamboyosa/docket/internal/provider"
)

type Dependencies struct {
	Config     config.Config
	List       func(context.Context) ([]domain.Document, error)
	Add        func(context.Context, string) (int, error)
	PickFiles  func(context.Context) ([]string, error)
	PickFolder func(context.Context) (string, error)
	BrowsePath string
	SaveConfig func(config.Config) error
	Models     func(context.Context, string) ([]provider.Model, error)
}

type mode int

const (
	browse mode = iota
	addPath
	browseFiles
	search
	settings
	modelPicker
	inspectDocument
	help
)

type Model struct {
	deps                  Dependencies
	docs                  []domain.Document
	cursor                int
	width                 int
	height                int
	mode                  mode
	input                 string
	message               string
	busy                  bool
	refreshingAfterImport bool
	frame                 int
	settings              config.Config
	settingRow            int
	models                []provider.Model
	modelCursor           int
	modelQuery            string
	browserPath           string
	browserEntries        []fileEntry
	browserCursor         int
}

type fileEntry struct {
	name  string
	path  string
	isDir bool
}

type loadedMsg struct {
	docs []domain.Document
	err  error
}

type addedMsg struct {
	count int
	err   error
}
type pickedMsg struct {
	paths []string
	err   error
}
type directoryMsg struct {
	path    string
	entries []fileEntry
	err     error
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
		m.docs = msg.docs
		if msg.err != nil {
			m.message = msg.err.Error()
		} else if m.refreshingAfterImport {
			m.message = fmt.Sprintf("%d document%s in library", len(m.docs), plural(len(m.docs)))
		} else if m.message == "Loading library…" {
			m.message = ""
		}
		m.refreshingAfterImport = false
		if m.cursor >= len(m.docs) && m.cursor > 0 {
			m.cursor = len(m.docs) - 1
		}
	case addedMsg:
		m.busy = false
		if msg.err != nil {
			m.message = msg.err.Error()
			if msg.count > 0 {
				m.message = fmt.Sprintf("%d document%s filed; %s", msg.count, plural(msg.count), msg.err)
				return m, m.loadDocuments()
			}
			return m, nil
		}
		m.message = fmt.Sprintf("%d document%s imported", msg.count, plural(msg.count))
		m.refreshingAfterImport = true
		return m, m.loadDocuments()
	case pickedMsg:
		m.busy = false
		if msg.err != nil {
			m.message = msg.err.Error()
			return m, nil
		}
		if len(msg.paths) == 0 {
			m.message = "Selection cancelled"
			return m, nil
		}
		m.busy = true
		m.message = fmt.Sprintf("Importing %d document%s…", len(msg.paths), plural(len(msg.paths)))
		return m, tea.Batch(m.addDocuments(msg.paths), tick())
	case directoryMsg:
		m.busy = false
		if msg.err != nil {
			m.message = msg.err.Error()
			return m, nil
		}
		m.browserPath = msg.path
		m.browserEntries = msg.entries
		m.browserCursor = 0
		m.message = ""
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
	case browseFiles:
		return m.handleFileBrowser(key)
	case settings:
		return m.handleSettings(key)
	case modelPicker:
		return m.handleModelPicker(key)
	case inspectDocument:
		if key.String() == "esc" || key.String() == "enter" || key.String() == "q" {
			m.mode = browse
		}
		return m, nil
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
		path := m.deps.BrowsePath
		if path == "" {
			var err error
			path, err = os.UserHomeDir()
			if err != nil {
				m.message = fmt.Sprintf("Find home folder: %v", err)
				return m, nil
			}
		}
		m.mode, m.busy, m.message = browseFiles, true, "Opening file browser…"
		return m, m.readDirectory(path)
	case "o":
		m.busy, m.message = true, "Opening file picker…"
		return m, tea.Batch(m.pickFiles(), tick())
	case "O":
		m.busy, m.message = true, "Opening folder picker…"
		return m, tea.Batch(m.pickFolder(), tick())
	case "/":
		m.mode, m.input, m.cursor = search, "", 0
	case "s":
		m.mode, m.settings, m.settingRow, m.message = settings, m.deps.Config, 0, ""
	case "enter":
		if len(m.visibleDocuments()) > 0 {
			m.mode = inspectDocument
		}
	case "?":
		m.mode = help
	case "r":
		return m, m.loadDocuments()
	}
	return m, nil
}

func (m Model) handleFileBrowser(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.mode, m.message = browse, ""
	case "up", "k":
		if m.browserCursor > 0 {
			m.browserCursor--
		}
	case "down", "j":
		if m.browserCursor < len(m.browserEntries)-1 {
			m.browserCursor++
		}
	case "backspace", "left":
		parent := filepath.Dir(m.browserPath)
		if parent != m.browserPath {
			m.busy, m.message = true, "Opening parent folder…"
			return m, m.readDirectory(parent)
		}
	case "enter":
		if len(m.browserEntries) == 0 {
			return m, nil
		}
		entry := m.browserEntries[m.browserCursor]
		if entry.isDir {
			m.busy, m.message = true, "Opening "+entry.name+"…"
			return m, m.readDirectory(entry.path)
		}
		m.mode, m.busy, m.message = browse, true, "Importing document…"
		return m, tea.Batch(m.addDocuments([]string{entry.path}), tick())
	case "o":
		m.mode, m.busy, m.message = browse, true, "Opening system picker…"
		return m, tea.Batch(m.pickFiles(), tick())
	case "O":
		m.mode, m.busy, m.message = browse, true, "Opening system folder picker…"
		return m, tea.Batch(m.pickFolder(), tick())
	case "p":
		m.mode, m.input, m.message = addPath, "", ""
	}
	return m, nil
}

func (m Model) handleInput(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		if m.mode == addPath {
			m.mode, m.input = browseFiles, ""
		} else {
			m.mode, m.input = browse, ""
		}
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
		return m, tea.Batch(m.addDocuments([]string{path}), tick())
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
			m.settings.Model = ""
		}
	case "enter", "ctrl+l":
		m.busy, m.message = true, "Loading compatible models…"
		return m, tea.Batch(m.loadModels(), tick())
	case "backspace":
		if m.settingRow == 1 {
			m.settings.Model = removeLastRune(m.settings.Model)
		}
	case "ctrl+s":
		if m.settings.Model == "" {
			m.message = "Choose a model before saving"
			return m, nil
		}
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
			m.settings.Model = models[m.modelCursor].ID
			if err := m.deps.SaveConfig(m.settings); err != nil {
				m.mode, m.message = settings, err.Error()
				return m, nil
			}
			m.deps.Config, m.mode, m.message = m.settings, browse, "Extraction model updated"
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
		return m.panel("Add by path", "Paste, type, or drag a file or folder path here.\n\n› "+m.input+"█\n\n⌘V paste   enter import   esc back")
	case browseFiles:
		return m.fileBrowserView()
	case settings:
		return m.settingsView()
	case modelPicker:
		return m.modelsView()
	case inspectDocument:
		return m.panel("Document details", m.inspectView(min(max(38, m.width-16), 72)))
	case help:
		return m.panel("Keyboard", "↑/↓ or j/k  select a document\nenter        inspect selected document\na            browse files in Docket\no            open the system file picker\nO            choose a folder with the system picker\n/            filter documents\ns            provider and models\nr            refresh library\n?            this help\nq            quit\n\nInside the file browser, press p to paste or drag a path.\nOriginal files are never moved or changed.")
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
	footerLines := 1
	if status != "" {
		footerLines++
	}
	bodyHeight := max(8, m.height-8-footerLines)
	var body string
	if m.width < 82 {
		body = panelStyle.Width(innerWidth).Height(bodyHeight).Render(m.listView(innerWidth-4, bodyHeight-2))
	} else {
		leftWidth := innerWidth * 45 / 100
		rightWidth := innerWidth - leftWidth - 1
		left := panelStyle.Width(leftWidth).Height(bodyHeight).Render(m.listView(leftWidth-4, bodyHeight-2))
		right := detailStyle.Width(rightWidth).Height(bodyHeight).Render(m.detailView(rightWidth-4, bodyHeight-2))
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}
	guide := mutedStyle.Render(truncate("↑/↓ select   enter details   a browse   o system picker   / filter   s models   ? help   q quit", innerWidth))
	footer := guide
	if status != "" {
		footer = truncate(status, innerWidth) + "\n" + guide
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
		return query + mutedStyle.Render("No documents yet.\n\nPress a to browse files or o for the system picker.")
	}
	lines := []string{sectionStyle.Render(fmt.Sprintf("DOCUMENTS  %d", len(docs))), ""}
	available := max(1, height-3)
	start := 0
	if m.cursor >= available {
		start = m.cursor - available + 1
	}
	for index := start; index < len(docs) && len(lines)-2 < available; index++ {
		doc := docs[index]
		label := documentListLabel(doc)
		labelWidth := min(18, max(8, lipgloss.Width(label)))
		nameWidth := max(8, width-labelWidth-4)
		line := fmt.Sprintf("%-2s %-*s %s", statusMark(doc), nameWidth, truncate(doc.OriginalName, nameWidth), truncate(label, labelWidth))
		if index == m.cursor {
			line = selectedStyle.Width(width).Render(line)
		} else {
			line = rowStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return query + strings.Join(lines, "\n")
}

func (m Model) fileBrowserView() string {
	contentWidth := max(24, min(68, m.width-20))
	lines := []string{
		sectionStyle.Render("BROWSE IN DOCKET"),
		mutedStyle.Render(truncate(displayPath(m.browserPath), contentWidth)),
		"",
	}
	if m.busy {
		lines = append(lines, accentStyle.Render(m.message))
	} else if m.message != "" {
		lines = append(lines, errorStyle.Render(truncate(m.message, contentWidth)))
	} else if len(m.browserEntries) == 0 {
		lines = append(lines, mutedStyle.Render("No supported documents in this folder."))
	} else {
		limit := max(3, m.height-14)
		start := 0
		if m.browserCursor >= limit {
			start = m.browserCursor - limit + 1
		}
		for index := start; index < len(m.browserEntries) && index < start+limit; index++ {
			entry := m.browserEntries[index]
			mark := "·"
			if entry.isDir {
				mark = "▸"
			}
			line := truncate(mark+" "+entry.name, contentWidth-2)
			if index == m.browserCursor {
				line = selectedStyle.Render("› " + line)
			} else {
				line = "  " + line
			}
			lines = append(lines, line)
		}
	}
	lines = append(lines, "",
		mutedStyle.Render("↑/↓ move   enter open/import   backspace parent"),
		mutedStyle.Render("o system files   O system folder"),
		mutedStyle.Render("p paste/drag path   esc cancel"),
	)
	return m.panel("Add documents", strings.Join(lines, "\n"))
}

func (m Model) detailView(width, height int) string {
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
		sectionStyle.Render("JEV CLASSIFICATION"), "",
		labelValue("Category", strings.ToUpper(category)),
		labelValue("Confidence", confidence),
	}
	if doc.Status == domain.StatusFiled {
		rows = append(rows, guidanceRows(doc, width)...)
	}
	rows = append(rows,
		"", sectionStyle.Render("FILE"), "",
		truncate(doc.OriginalName, width),
		mutedStyle.Render(truncate(doc.LibraryPath, width)),
		"", sectionStyle.Render("EXTRACTION"), "",
		labelValue("Provider used", doc.Provider),
		labelValue("Model used", truncate(doc.Model, max(1, width-14))),
	)
	if doc.Error != "" {
		rows = append(rows, "", errorStyle.Render(truncate(doc.Error, width)))
	}
	return truncateLines(strings.Join(rows, "\n"), height)
}

func (m Model) inspectView(width int) string {
	docs := m.visibleDocuments()
	if len(docs) == 0 || m.cursor >= len(docs) {
		return mutedStyle.Render("No document selected.")
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
		truncate(doc.OriginalName, width),
		labelValue("Category", strings.ToUpper(category)),
		labelValue("Confidence", confidence),
	}
	if doc.Status == domain.StatusFiled {
		rows = append(rows, guidanceRows(doc, width)...)
	}
	rows = append(rows, "", labelValue("Provider used", doc.Provider), labelValue("Model used", truncate(doc.Model, max(1, width-14))))
	return strings.Join(rows, "\n")
}

func (m Model) settingsView() string {
	provider := "  " + m.settings.Provider
	modelName := m.settings.Model
	if modelName == "" {
		modelName = "Choose a compatible model"
	}
	model := "  " + modelName
	if m.settingRow == 0 {
		provider = selectedStyle.Render("› " + m.settings.Provider)
	} else {
		model = selectedStyle.Render("› " + modelName + "█")
	}
	content := "Extraction provider\n" + provider + "\n\nModel\n" + model + "\n\n" + mutedStyle.Render("←/→ provider   tab field   enter browse live models\nctrl+s save typed model   esc cancel")
	if m.message != "" {
		content += "\n\n" + mutedStyle.Render(m.message)
	}
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
		meta := models[index].ReleaseDate
		if models[index].Free {
			meta = strings.TrimSpace(meta + "  free")
		}
		line := truncate(models[index].Name+"  "+models[index].ID+"  "+meta, max(24, m.width-14))
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
	lines = append(lines, "", mutedStyle.Render("PDF + image · newest first · type to filter · ↑/↓ select · enter use"))
	return m.panel("Compatible models · "+m.settings.Provider, strings.Join(lines, "\n"))
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

func (m Model) readDirectory(path string) tea.Cmd {
	return func() tea.Msg {
		path, err := filepath.Abs(path)
		if err != nil {
			return directoryMsg{err: fmt.Errorf("open file browser: %w", err)}
		}
		items, err := os.ReadDir(path)
		if err != nil {
			return directoryMsg{path: path, err: fmt.Errorf("open %s: %w", path, err)}
		}
		entries := make([]fileEntry, 0, len(items))
		for _, item := range items {
			if strings.HasPrefix(item.Name(), ".") || item.Type()&os.ModeSymlink != 0 {
				continue
			}
			if !item.IsDir() && !files.Supported(item.Name()) {
				continue
			}
			entries = append(entries, fileEntry{name: item.Name(), path: filepath.Join(path, item.Name()), isDir: item.IsDir()})
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].isDir != entries[j].isDir {
				return entries[i].isDir
			}
			return strings.ToLower(entries[i].name) < strings.ToLower(entries[j].name)
		})
		return directoryMsg{path: path, entries: entries}
	}
}

func (m Model) addDocuments(paths []string) tea.Cmd {
	return func() tea.Msg {
		count := 0
		for _, path := range paths {
			added, err := m.deps.Add(context.Background(), path)
			count += added
			if err != nil {
				return addedMsg{count: count, err: err}
			}
		}
		return addedMsg{count: count}
	}
}

func (m Model) pickFiles() tea.Cmd {
	return func() tea.Msg {
		if m.deps.PickFiles == nil {
			return pickedMsg{err: errors.New("graphical file picker is unavailable")}
		}
		paths, err := m.deps.PickFiles(context.Background())
		return pickedMsg{paths: paths, err: err}
	}
}

func (m Model) pickFolder() tea.Cmd {
	return func() tea.Msg {
		if m.deps.PickFolder == nil {
			return pickedMsg{err: errors.New("graphical folder picker is unavailable")}
		}
		path, err := m.deps.PickFolder(context.Background())
		if path == "" {
			return pickedMsg{err: err}
		}
		return pickedMsg{paths: []string{path}, err: err}
	}
}

func (m Model) loadModels() tea.Cmd {
	return func() tea.Msg {
		models, err := m.deps.Models(context.Background(), m.settings.Provider)
		return modelsMsg{models: models, err: err}
	}
}

func tick() tea.Cmd {
	return tea.Tick(90*time.Millisecond, func(value time.Time) tea.Msg { return tickMsg(value) })
}

func cleanPath(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		value = strings.TrimSpace(value[1 : len(value)-1])
	}
	value = strings.Trim(value, "'\"")
	return strings.ReplaceAll(value, "\\ ", " ")
}

func displayPath(path string) string {
	home, err := os.UserHomeDir()
	if err == nil && path == home {
		return "~"
	}
	if err == nil && strings.HasPrefix(path, home+string(filepath.Separator)) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
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

func documentListLabel(doc domain.Document) string {
	category := doc.Category
	if category == "" {
		return string(doc.Status)
	}
	if doc.Review {
		return category + " · review"
	}
	if !doc.NeedsAction {
		return category
	}
	switch {
	case doc.Urgency >= 2.5:
		return category + " · now"
	case doc.Urgency >= 1.5:
		return category + " · soon"
	default:
		return category + " · action"
	}
}

func labelValue(label, value string) string {
	if value == "" {
		value = "—"
	}
	return mutedStyle.Render(fmt.Sprintf("%-14s", label)) + value
}

func guidanceRows(doc domain.Document, width int) []string {
	valueWidth := max(1, width-14)
	review := "Not required"
	if doc.Review {
		review = "Required · verify category"
	}
	return []string{
		"", sectionStyle.Render("JEV OUTPUT"),
		labelValue("Sensitivity", truncate(scoreOutput(doc.Sensitivity, sensitivityLevel(doc.Sensitivity)), valueWidth)),
		labelValue("Urgency", truncate(scoreOutput(doc.Urgency, urgencyLevel(doc.Urgency)), valueWidth)),
		labelValue("Needs action", fmt.Sprintf("%.0f%%", doc.NeedsActionProbability*100)),
		"", sectionStyle.Render("DOCKET GUIDANCE"),
		labelValue("Next step", truncate(nextStep(doc), valueWidth)),
		labelValue("Handling", truncate(handlingGuidance(doc.Sensitivity), valueWidth)),
		labelValue("Action", truncate(actionGuidance(doc), valueWidth)),
		labelValue("Review", truncate(review, valueWidth)),
		mutedStyle.Render(truncate("Triage signal · verify exact dates in the document", width)),
	}
}

func nextStep(doc domain.Document) string {
	if actionIsUnclear(doc.NeedsActionProbability) {
		return "Check for a request or deadline"
	}
	if doc.NeedsAction {
		switch {
		case doc.Urgency >= 2.5:
			return "Act now; check exact deadline"
		case doc.Urgency >= 1.5:
			return "Act soon; check exact deadline"
		default:
			return "Plan follow-up"
		}
	}
	if doc.Urgency >= 1.5 {
		return "Check the document for a deadline"
	}
	if doc.Review {
		return "Verify the category"
	}
	return "File for reference"
}

func scoreOutput(score float64, level string) string {
	return fmt.Sprintf("%.1f / 3 · %s", score, level)
}

func sensitivityLevel(score float64) string {
	switch {
	case score < .5:
		return "Routine"
	case score < 1.5:
		return "Personal"
	case score < 2.5:
		return "Confidential"
	default:
		return "Highly sensitive"
	}
}

func urgencyLevel(score float64) string {
	switch {
	case score < .5:
		return "No deadline"
	case score < 1.5:
		return "Action eventually"
	case score < 2.5:
		return "Time-sensitive"
	default:
		return "Immediate"
	}
}

func handlingGuidance(score float64) string {
	switch {
	case score < .5:
		return "Normal handling"
	case score < 1.5:
		return "Keep private"
	case score < 2.5:
		return "Limit sharing"
	default:
		return "Secure carefully"
	}
}

func actionGuidance(doc domain.Document) string {
	probability := doc.NeedsActionProbability
	if actionIsUnclear(probability) {
		return fmt.Sprintf("Unclear · %.0f%% likelihood", probability*100)
	}
	if doc.NeedsAction {
		if probability == 0 {
			return "Likely required"
		}
		return fmt.Sprintf("Likely required · %.0f%%", probability*100)
	}
	if probability == 0 {
		return "Not detected"
	}
	return fmt.Sprintf("Not detected · %.0f%% likelihood", probability*100)
}

func actionIsUnclear(probability float64) bool {
	return probability >= .35 && probability < .65
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

func truncateLines(value string, height int) string {
	lines := strings.Split(value, "\n")
	if len(lines) <= height {
		return value
	}
	return strings.Join(lines[:height], "\n")
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

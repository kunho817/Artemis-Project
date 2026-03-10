package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type FilePicker struct {
	cwd     string
	entries []fileEntry
	cursor  int
	scroll  int
	width   int
	height  int
	err     string
}

type fileEntry struct {
	Name  string
	IsDir bool
	Size  int64
}

func NewFilePicker(cwd string, termWidth, termHeight int) *FilePicker {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		abs = cwd
	}
	fp := &FilePicker{cwd: abs}
	fp.SetSize(termWidth, termHeight)
	fp.readDir()
	return fp
}

func (fp *FilePicker) SetSize(w, h int) {
	fp.width = clampInt(minInt(60, w-10), 40, 60)
	fp.height = clampInt(minInt(20, h-4), 12, 20)
}

func (fp *FilePicker) Update(msg tea.Msg) (bool, OverlayResult, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return true, OverlayResult{}, nil
		case "up":
			if fp.cursor > 0 {
				fp.cursor--
			}
			fp.ensureCursorVisible()
			return false, OverlayResult{}, nil
		case "down":
			if fp.cursor < len(fp.entries)-1 {
				fp.cursor++
			}
			fp.ensureCursorVisible()
			return false, OverlayResult{}, nil
		case "enter":
			if fp.cursor < 0 || fp.cursor >= len(fp.entries) {
				return false, OverlayResult{}, nil
			}
			sel := fp.entries[fp.cursor]
			if sel.Name == ".." {
				parent := filepath.Dir(fp.cwd)
				fp.cwd = parent
				fp.cursor, fp.scroll = 0, 0
				fp.readDir()
				return false, OverlayResult{}, nil
			}
			full := filepath.Join(fp.cwd, sel.Name)
			if sel.IsDir {
				fp.cwd = full
				fp.cursor, fp.scroll = 0, 0
				fp.readDir()
				return false, OverlayResult{}, nil
			}
			abs, err := filepath.Abs(full)
			if err != nil {
				abs = full
			}
			return true, OverlayResult{Action: "select_file", Value: abs}, nil
		}
	}
	return false, OverlayResult{}, nil
}

func (fp *FilePicker) View() string {
	var sb strings.Builder
	sb.WriteString(overlayTitleStyle().Render("File Picker"))
	sb.WriteString("\n")
	sb.WriteString(overlayAccentStyle().Render("📁 " + displayPath(fp.cwd)))
	sb.WriteString("\n")
	sb.WriteString(overlayDimStyle().Render(strings.Repeat("─", fp.width-6)))
	sb.WriteString("\n")

	if fp.err != "" {
		sb.WriteString(overlayAccentStyle().Render("Error: " + fp.err))
		sb.WriteString("\n")
	}

	maxVisible := fp.height - 6
	if maxVisible < 1 {
		maxVisible = 1
	}
	start := fp.scroll
	end := minInt(len(fp.entries), start+maxVisible)

	for i := start; i < end; i++ {
		ent := fp.entries[i]
		prefix := "  "
		style := overlayItemStyle()
		if i == fp.cursor {
			prefix = "> "
			style = overlaySelectedStyle()
		}

		name := ent.Name
		size := ""
		if ent.IsDir {
			if name != ".." {
				name += "/"
			}
		} else {
			size = humanSize(ent.Size)
		}

		gap := fp.width - 8 - len(prefix) - lipWidth(name) - lipWidth(size)
		if gap < 1 {
			gap = 1
		}
		line := prefix + name + strings.Repeat(" ", gap) + size
		sb.WriteString(style.Width(fp.width - 6).Render(line))
		sb.WriteString("\n")
	}

	for i := end - start; i < maxVisible; i++ {
		sb.WriteString(strings.Repeat(" ", fp.width-6))
		sb.WriteString("\n")
	}

	sb.WriteString(overlayDimStyle().Render(strings.Repeat("─", fp.width-6)))
	sb.WriteString("\n")
	sb.WriteString(overlayDimStyle().Render("↑↓ navigate  ↵ open  esc close"))

	return overlayBoxStyle(fp.width, fp.height).Render(sb.String())
}

func (fp *FilePicker) readDir() {
	fp.err = ""
	entries, err := os.ReadDir(fp.cwd)
	if err != nil {
		fp.entries = []fileEntry{{Name: "..", IsDir: true}}
		fp.err = err.Error()
		return
	}

	dirs := make([]fileEntry, 0, len(entries))
	files := make([]fileEntry, 0, len(entries))
	for _, e := range entries {
		info, infoErr := e.Info()
		sz := int64(0)
		if infoErr == nil {
			sz = info.Size()
		}
		item := fileEntry{Name: e.Name(), IsDir: e.IsDir(), Size: sz}
		if item.IsDir {
			dirs = append(dirs, item)
		} else {
			files = append(files, item)
		}
	}

	sort.SliceStable(dirs, func(i, j int) bool { return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name) })
	sort.SliceStable(files, func(i, j int) bool { return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name) })

	fp.entries = make([]fileEntry, 0, 1+len(dirs)+len(files))
	fp.entries = append(fp.entries, fileEntry{Name: "..", IsDir: true})
	fp.entries = append(fp.entries, dirs...)
	fp.entries = append(fp.entries, files...)
}

func (fp *FilePicker) ensureCursorVisible() {
	maxVisible := fp.height - 6
	if maxVisible < 1 {
		maxVisible = 1
	}
	if fp.cursor < fp.scroll {
		fp.scroll = fp.cursor
	}
	if fp.cursor >= fp.scroll+maxVisible {
		fp.scroll = fp.cursor - maxVisible + 1
	}
	if fp.scroll < 0 {
		fp.scroll = 0
	}
}

func displayPath(p string) string {
	return filepath.ToSlash(p)
}

func humanSize(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%dB", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1fK", float64(n)/1024)
	}
	return fmt.Sprintf("%.1fM", float64(n)/(1024*1024))
}

func lipWidth(s string) int {
	return len([]rune(s))
}

var _ Overlay = (*FilePicker)(nil)

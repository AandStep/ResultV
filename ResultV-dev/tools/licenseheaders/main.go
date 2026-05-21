package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	oldName   = "ResultProxy"
	newName   = "ResultV"
	gnuPhrase = "This program is free software: you can redistribute it and/or modify"
)

type headerStyle struct {
	linePrefix string
	block      bool
}

var supportedStyles = map[string]headerStyle{
	".go":  {linePrefix: "// ", block: false},
	".js":  {linePrefix: " * ", block: true},
	".jsx": {linePrefix: " * ", block: true},
	".ts":  {linePrefix: " * ", block: true},
	".tsx": {linePrefix: " * ", block: true},
	".css": {linePrefix: " * ", block: true},
}

var gnuLines = []string{
	"Copyright (C) 2026 %s",
	"",
	"This program is free software: you can redistribute it and/or modify",
	"it under the terms of the GNU General Public License as published by",
	"the Free Software Foundation, either version 3 of the License, or",
	"(at your option) any later version.",
	"",
	"This program is distributed in the hope that it will be useful,",
	"but WITHOUT ANY WARRANTY; without even the implied warranty of",
	"MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the",
	"GNU General Public License for more details.",
	"",
	"You should have received a copy of the GNU General Public License",
	"along with this program.  If not, see <https://www.gnu.org/licenses/>.",
}

func main() {
	files, err := trackedFiles()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot list tracked files: %v\n", err)
		os.Exit(1)
	}

	var added, updated, unchanged int

	for _, file := range files {
		if shouldSkip(file) {
			continue
		}
		ext := strings.ToLower(filepath.Ext(file))
		style, ok := supportedStyles[ext]
		if !ok {
			continue
		}

		raw, err := os.ReadFile(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip %s: %v\n", file, err)
			continue
		}
		if bytes.IndexByte(raw, 0) != -1 {
			continue
		}

		content := string(raw)
		newline := detectNewline(content)

		nextContent, changed, action := rewriteFile(content, style, newline)
		if !changed {
			unchanged++
			continue
		}
		if err := os.WriteFile(file, []byte(nextContent), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "cannot write %s: %v\n", file, err)
			continue
		}
		if action == "added" {
			added++
		} else {
			updated++
		}
	}

	fmt.Printf("done: added=%d updated=%d unchanged=%d\n", added, updated, unchanged)
}

func trackedFiles() ([]string, error) {
	cmd := exec.Command("git", "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	parts := strings.Split(string(out), "\x00")
	files := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		files = append(files, filepath.FromSlash(p))
	}
	return files, nil
}

func shouldSkip(path string) bool {
	p := strings.ReplaceAll(path, "\\", "/")

	// Hard-coded dirs that never get our GPL header.
	skipped := []string{
		"build/",
		"frontend/dist/",
		"frontend/node_modules/",
		"frontend/wailsjs/",
		"node_modules/",
		"vendor/",
		// Vendored third-party code with its own license — do not overwrite.
		"internal/getlantern_systray/",
	}
	for _, prefix := range skipped {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}

	// Any directory (or its parent) that contains a .no-licenseheader sentinel
	// file is also skipped. Drop a .no-licenseheader file next to any future
	// vendored package to protect it automatically.
	return hasNoLicenseHeaderMarker(p)
}

// hasNoLicenseHeaderMarker walks up the path looking for a .no-licenseheader
// file in any ancestor directory up to the repo root (we stop at 8 levels to
// avoid walking past the repo).
func hasNoLicenseHeaderMarker(filePath string) bool {
	dir := filepath.Dir(filePath)
	for range 8 {
		if dir == "." || dir == "" {
			break
		}
		if _, err := os.Stat(filepath.Join(dir, ".no-licenseheader")); err == nil {
			return true
		}
		dir = filepath.Dir(dir)
	}
	return false
}

func rewriteFile(content string, style headerStyle, nl string) (string, bool, string) {
	if hasGNUHeader(content) {
		updatedTop := replaceNameInTop(content, nl)
		if updatedTop != content {
			return updatedTop, true, "updated"
		}
		return content, false, ""
	}

	header := buildHeader(style, nl)
	if strings.HasPrefix(content, "\uFEFF") {
		body := strings.TrimPrefix(content, "\uFEFF")
		return "\uFEFF" + header + body, true, "added"
	}
	return header + content, true, "added"
}

func hasGNUHeader(content string) bool {
	top := firstNLines(content, 40)
	return strings.Contains(top, gnuPhrase)
}

func replaceNameInTop(content string, nl string) string {
	lines := strings.Split(content, nl)
	limit := len(lines)
	if limit > 40 {
		limit = 40
	}
	for i := 0; i < limit; i++ {
		if strings.Contains(lines[i], "Copyright") {
			lines[i] = strings.ReplaceAll(lines[i], oldName, newName)
		}
	}
	return strings.Join(lines, nl)
}

func buildHeader(style headerStyle, nl string) string {
	if style.block {
		var b strings.Builder
		b.WriteString("/*")
		b.WriteString(nl)
		for _, line := range gnuLines {
			text := line
			if strings.Contains(text, "%s") {
				text = fmt.Sprintf(text, newName)
			}
			b.WriteString(style.linePrefix)
			b.WriteString(text)
			b.WriteString(nl)
		}
		b.WriteString(" */")
		b.WriteString(nl)
		b.WriteString(nl)
		return b.String()
	}

	var b strings.Builder
	for _, line := range gnuLines {
		text := line
		if strings.Contains(text, "%s") {
			text = fmt.Sprintf(text, newName)
		}
		b.WriteString(style.linePrefix)
		b.WriteString(text)
		b.WriteString(nl)
	}
	b.WriteString(nl)
	return b.String()
}

func detectNewline(s string) string {
	if strings.Contains(s, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

func firstNLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[:n], "\n")
}

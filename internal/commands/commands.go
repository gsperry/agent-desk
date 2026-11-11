// Package commands lists the slash commands a Claude Code session accepts,
// for the composer's autocomplete: Claude Code's built-in commands, the
// user's and the project's custom commands and skills, and those of enabled
// plugins. It only reads files; nothing under the config directory is
// written.
package commands

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Kind string

const (
	KindBuiltin Kind = "builtin"
	KindCommand Kind = "command"
	KindSkill   Kind = "skill"
)

// Command is one slash command.
type Command struct {
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	ArgumentHint string   `json:"argumentHint,omitempty"`
	Aliases      []string `json:"aliases,omitempty"`
	Kind         Kind     `json:"kind"`
	// Source says where a custom command or skill comes from: user,
	// project, or plugin:<name>.
	Source string `json:"source,omitempty"`
}

// Builtin returns Claude Code's own slash commands.
func Builtin() []Command {
	return append([]Command(nil), builtin...)
}

var validName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,80}$`)

// Lister finds custom commands and skills under a Claude config directory
// and a session's project directory.
type Lister struct {
	// Root is the Claude config directory, such as ~/.claude.
	Root string
}

// List returns the built-in commands and the custom ones visible to a
// session working in dir, sorted by name. Later sources do not replace an
// earlier name: built-ins first, then project, user and plugins.
func (l Lister) List(dir string) []Command {
	seen := map[string]bool{}
	var out []Command
	add := func(cs []Command) {
		for _, c := range cs {
			if !validName.MatchString(c.Name) || seen[c.Name] {
				continue
			}
			seen[c.Name] = true
			out = append(out, c)
		}
	}
	add(Builtin())
	for _, root := range projectRoots(dir) {
		add(scanCommands(filepath.Join(root, ".claude", "commands"), "", "project"))
		add(scanSkills(filepath.Join(root, ".claude", "skills"), "", "project"))
	}
	if l.Root != "" {
		add(scanCommands(filepath.Join(l.Root, "commands"), "", "user"))
		add(scanSkills(filepath.Join(l.Root, "skills"), "", "user"))
		for _, p := range l.plugins(dir) {
			add(scanCommands(filepath.Join(p.path, "commands"), p.name+":", "plugin:"+p.name))
			add(scanSkills(filepath.Join(p.path, "skills"), p.name+":", "plugin:"+p.name))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// projectRoots is dir and its parents up to the nearest git root, where
// Claude Code looks for .claude/.
func projectRoots(dir string) []string {
	if dir == "" {
		return nil
	}
	var roots []string
	for d := filepath.Clean(dir); ; d = filepath.Dir(d) {
		roots = append(roots, d)
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil || filepath.Dir(d) == d || len(roots) > 20 {
			return roots
		}
	}
}

// scanCommands reads <dir>/**/*.md. A file in a subdirectory is named
// sub:name, as Claude Code namespaces them.
func scanCommands(dir, prefix, source string) []Command {
	var out []Command
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return nil
		}
		name := strings.TrimSuffix(filepath.ToSlash(rel), ".md")
		name = strings.ReplaceAll(name, "/", ":")
		fm := frontMatter(path)
		desc := fm["description"]
		if desc == "" {
			desc = fm["_first"]
		}
		out = append(out, Command{Name: prefix + name, Description: clip(desc), ArgumentHint: fm["argument-hint"], Kind: KindCommand, Source: source})
		return nil
	})
	return out
}

// scanSkills reads <dir>/<skill>/SKILL.md.
func scanSkills(dir, prefix, source string) []Command {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Command
	for _, e := range entries {
		if !e.IsDir() && e.Type()&os.ModeSymlink == 0 {
			continue
		}
		path := filepath.Join(dir, e.Name(), "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			continue
		}
		fm := frontMatter(path)
		name := fm["name"]
		if name == "" {
			name = e.Name()
		}
		out = append(out, Command{Name: prefix + name, Description: clip(fm["description"]), Kind: KindSkill, Source: source})
	}
	return out
}

// frontMatter reads the simple key: value lines of a Markdown file's YAML
// front matter, and the first body line as _first. Only the first 8 KB
// are read.
func frontMatter(path string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(&limited{f: f, left: 8 << 10})
	inFM, first := false, true
	for sc.Scan() {
		line := sc.Text()
		if first {
			first = false
			if strings.TrimSpace(line) == "---" {
				inFM = true
				continue
			}
		}
		if inFM {
			if strings.TrimSpace(line) == "---" {
				inFM = false
				continue
			}
			k, v, ok := strings.Cut(line, ":")
			if ok && !strings.HasPrefix(line, " ") {
				out[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
			}
			continue
		}
		if t := strings.TrimSpace(strings.TrimLeft(line, "#")); t != "" && out["_first"] == "" {
			out["_first"] = t
		}
	}
	return out
}

type limited struct {
	f    *os.File
	left int
}

func (l *limited) Read(p []byte) (int, error) {
	if l.left <= 0 {
		return 0, os.ErrClosed
	}
	if len(p) > l.left {
		p = p[:l.left]
	}
	n, err := l.f.Read(p)
	l.left -= n
	return n, err
}

func clip(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 160 {
		return string(r[:160]) + "…"
	}
	return s
}

type plugin struct{ name, path string }

// plugins are the installed plugins that are enabled in the user's or the
// project's settings and apply to dir.
func (l Lister) plugins(dir string) []plugin {
	var installed struct {
		Plugins map[string][]struct {
			Scope       string `json:"scope"`
			ProjectPath string `json:"projectPath"`
			InstallPath string `json:"installPath"`
		} `json:"plugins"`
	}
	data, err := os.ReadFile(filepath.Join(l.Root, "plugins", "installed_plugins.json"))
	if err != nil || json.Unmarshal(data, &installed) != nil {
		return nil
	}
	enabled := enabledPlugins(filepath.Join(l.Root, "settings.json"))
	for _, root := range projectRoots(dir) {
		for k, v := range enabledPlugins(filepath.Join(root, ".claude", "settings.json")) {
			enabled[k] = v
		}
	}
	var out []plugin
	for id, installs := range installed.Plugins {
		if !enabled[id] {
			continue
		}
		name, _, _ := strings.Cut(id, "@")
		for _, in := range installs {
			if in.ProjectPath != "" && !within(dir, in.ProjectPath) {
				continue
			}
			if in.InstallPath != "" {
				out = append(out, plugin{name: name, path: in.InstallPath})
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

func enabledPlugins(path string) map[string]bool {
	var s struct {
		EnabledPlugins map[string]bool `json:"enabledPlugins"`
	}
	out := map[string]bool{}
	if data, err := os.ReadFile(path); err == nil && json.Unmarshal(data, &s) == nil {
		for k, v := range s.EnabledPlugins {
			out[k] = v
		}
	}
	return out
}

func within(dir, root string) bool {
	rel, err := filepath.Rel(root, dir)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}

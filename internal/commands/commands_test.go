package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestList(t *testing.T) {
	root := t.TempDir()
	proj := t.TempDir()
	sub := filepath.Join(proj, "web")
	if err := os.MkdirAll(filepath.Join(proj, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	// User command with front matter, and one namespaced by a subdirectory.
	write(t, filepath.Join(root, "commands", "ship.md"), "---\ndescription: Ship the branch\nargument-hint: <version>\n---\nDo it.\n")
	write(t, filepath.Join(root, "commands", "ops", "deploy.md"), "# Deploy to staging\nSteps...\n")
	// User skill.
	write(t, filepath.Join(root, "skills", "triage", "SKILL.md"), "---\nname: triage\ndescription: \"Sort incoming issues\"\n---\nbody\n")
	// A directory without SKILL.md is not a skill.
	write(t, filepath.Join(root, "skills", "notes", "README.md"), "x")
	// Project command at the git root, found from a subdirectory; it wins over
	// a user command of the same name.
	write(t, filepath.Join(proj, ".claude", "commands", "ship.md"), "---\ndescription: Project ship\n---\n")
	// A command that would shadow a built-in is ignored.
	write(t, filepath.Join(proj, ".claude", "commands", "compact.md"), "---\ndescription: Not the real compact\n---\n")
	// Plugins: one enabled, one installed but disabled, one scoped to another project.
	enabledDir := filepath.Join(root, "plugins", "cache", "figma")
	write(t, filepath.Join(enabledDir, "commands", "inspect.md"), "---\ndescription: Inspect a frame\n---\n")
	write(t, filepath.Join(enabledDir, "skills", "design", "SKILL.md"), "---\nname: design\ndescription: Design help\n---\n")
	disabledDir := filepath.Join(root, "plugins", "cache", "off")
	write(t, filepath.Join(disabledDir, "commands", "nope.md"), "x")
	otherDir := filepath.Join(root, "plugins", "cache", "other")
	write(t, filepath.Join(otherDir, "commands", "elsewhere.md"), "x")
	write(t, filepath.Join(root, "plugins", "installed_plugins.json"), `{"version":2,"plugins":{
		"figma@official":[{"scope":"user","installPath":"`+enabledDir+`"}],
		"off@official":[{"scope":"user","installPath":"`+disabledDir+`"}],
		"other@market":[{"scope":"project","projectPath":"/somewhere/else","installPath":"`+otherDir+`"}]}}`)
	write(t, filepath.Join(root, "settings.json"), `{"enabledPlugins":{"figma@official":true,"off@official":false,"other@market":true}}`)

	got := map[string]Command{}
	for _, c := range (Lister{Root: root}).List(sub) {
		got[c.Name] = c
	}
	tests := []struct {
		name, desc, hint string
		kind             Kind
		source           string
	}{
		{"compact", "Free up context by summarizing the conversation so far", "<optional custom summarization instructions>", KindBuiltin, ""},
		{"ship", "Project ship", "", KindCommand, "project"},
		{"ops:deploy", "Deploy to staging", "", KindCommand, "user"},
		{"triage", "Sort incoming issues", "", KindSkill, "user"},
		{"figma:inspect", "Inspect a frame", "", KindCommand, "plugin:figma"},
		{"figma:design", "Design help", "", KindSkill, "plugin:figma"},
	}
	for _, tt := range tests {
		c, ok := got[tt.name]
		if !ok {
			t.Errorf("missing /%s", tt.name)
			continue
		}
		if c.Description != tt.desc || c.ArgumentHint != tt.hint || c.Kind != tt.kind || c.Source != tt.source {
			t.Errorf("/%s = %+v", tt.name, c)
		}
	}
	for _, absent := range []string{"notes", "off:nope", "other:elsewhere"} {
		if _, ok := got[absent]; ok {
			t.Errorf("/%s should not be listed", absent)
		}
	}
}

func TestBuiltinList(t *testing.T) {
	b := Builtin()
	if len(b) < 40 || BuiltinVersion == "" {
		t.Fatalf("%d built-ins, version %q", len(b), BuiltinVersion)
	}
	names := map[string]bool{}
	for _, c := range b {
		if names[c.Name] || c.Description == "" || c.Kind != KindBuiltin {
			t.Errorf("bad built-in %+v", c)
		}
		names[c.Name] = true
	}
	for _, want := range []string{"compact", "clear", "model", "init", "help", "context", "rewind"} {
		if !names[want] {
			t.Errorf("built-ins lack /%s", want)
		}
	}
}

func TestFrontMatterToleratesJunk(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.md"), "---\nnot yaml at all\n  indented: ignored\ndescription: ok\n")
	write(t, filepath.Join(dir, "b.md"), "")
	got := scanCommands(dir, "", "user")
	if len(got) != 2 || got[0].Description != "ok" {
		t.Fatalf("got %+v", got)
	}
}

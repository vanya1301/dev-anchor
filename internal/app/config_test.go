package app

import "testing"

func TestDefaultPaths(t *testing.T) {
	p := DefaultPaths("/repo")
	if p.ToolsFile != "/repo/tools.toml" {
		t.Fatalf("tools=%s", p.ToolsFile)
	}
	if p.StateFile != "/repo/.da/state.json" {
		t.Fatalf("state=%s", p.StateFile)
	}
	if p.Configs != "/repo/configs" {
		t.Fatalf("configs=%s", p.Configs)
	}
}

func TestLoadSymlinks(t *testing.T) {
	toml := `
[[link]]
source = "zsh/.zshrc"
target = "~/.zshrc"

[[link]]
source = "vim/.vimrc"
target = "~/.vimrc"
`
	links, err := LoadSymlinks([]byte(toml))
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 || links[0].Source != "zsh/.zshrc" || links[0].Target != "~/.zshrc" {
		t.Fatalf("links=%+v", links)
	}
}

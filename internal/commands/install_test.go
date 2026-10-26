package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"weeco/internal/assets"
	"weeco/internal/store"
	"weeco/internal/sysutil"
)

func TestInstallUpdatesOwnedTemplate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WEECO_HOME", home)
	bin := t.TempDir()
	skills := t.TempDir()
	commands := t.TempDir()

	templateDir := store.TemplateDir()
	resultPath := filepath.Join(templateDir, "result.md")
	contractPath := filepath.Join(templateDir, "contract.md")
	writeTestFile(t, resultPath, "# old result template\n")
	writeTestFile(t, contractPath, "# custom contract\n")

	manifest := installManifest{
		Version:   "0.6.1",
		Templates: []string{resultPath},
	}
	if err := sysutil.WriteJSON(store.ManifestPath(), manifest); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"install", "--bin-dir", bin, "--skill-dir", skills, "--command-dir", commands}); code != 0 {
		t.Fatalf("install failed: %d", code)
	}

	want, err := assets.Read(assets.ResultTemplate)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("owned template was not updated:\n%s", got)
	}

	// The contract template was not in the previous manifest, so it is treated
	// as a user file and must not be overwritten.
	contract, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(contract) != "# custom contract\n" {
		t.Fatalf("unowned template was overwritten:\n%s", contract)
	}
}

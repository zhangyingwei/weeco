package commands

import (
	"bytes"
	iofs "io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"weeco/internal/assets"
	"weeco/internal/store"
	"weeco/internal/sysutil"
)

type installFixture struct {
	home     string
	bin      string
	skills   string
	commands string
}

func installTempDir(t *testing.T) string {
	t.Helper()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func newInstallFixture(t *testing.T) installFixture {
	t.Helper()
	fixture := installFixture{installTempDir(t), installTempDir(t), installTempDir(t), installTempDir(t)}
	t.Setenv("WEECO_HOME", fixture.home)
	t.Setenv("WEECO_BIN_DIR", fixture.bin)
	t.Setenv("WEECO_SKILL_DIR", fixture.skills)
	t.Setenv("WEECO_COMMAND_DIR", fixture.commands)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// Reinstall tests reuse the current executable instead of executing a copied
	// Go test binary to identify an existing weeco executable.
	if err := os.Link(self, filepath.Join(fixture.bin, "weeco")); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func (f installFixture) run(t *testing.T, extra ...string) (int, string) {
	t.Helper()
	output, err := os.CreateTemp(t.TempDir(), "install-output-")
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = output, output
	defer func() { os.Stdout, os.Stderr = stdout, stderr }()
	code := cmdInstall(extra)
	data, err := os.ReadFile(output.Name())
	if err != nil {
		t.Fatal(err)
	}
	return code, string(data)
}

func (f installFixture) install(t *testing.T, extra ...string) string {
	t.Helper()
	code, output := f.run(t, extra...)
	if code != 0 {
		t.Fatalf("install failed: %d\n%s", code, output)
	}
	return output
}

func writeInstallFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readInstallManifest(t *testing.T) installManifest {
	t.Helper()
	var manifest installManifest
	if err := sysutil.ReadJSON(store.ManifestPath(), &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func assertInstallFileContent(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("unexpected content in %s:\n%s", path, got)
	}
}

func assertInstallPathMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("expected missing path %s, got %v", path, err)
	}
}

func TestInstallCopiesOnlyWeecoSkillAndMemberPrompt(t *testing.T) {
	fixture := newInstallFixture(t)
	fixture.install(t)
	entries, err := os.ReadDir(fixture.skills)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "weeco" {
		t.Fatalf("unexpected installed skill directories: %v", entries)
	}
	entries, err = os.ReadDir(filepath.Join(fixture.skills, "weeco"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	if !slices.Equal(names, []string{"SKILL.md", "member-prompt.md"}) {
		t.Fatalf("unexpected skill files: %v", names)
	}
	manifest := readInstallManifest(t)
	if len(manifest.Skills) != 1 || manifest.Skills["weeco"] != filepath.Join(fixture.skills, "weeco", "SKILL.md") {
		t.Fatalf("unexpected manifest skills: %v", manifest.Skills)
	}
	if !slices.Equal(manifest.SkillFiles, []string{filepath.Join(fixture.skills, "weeco", "member-prompt.md")}) {
		t.Fatalf("unexpected manifest skill files: %v", manifest.SkillFiles)
	}
	if manifest.Command != "" || len(manifest.Templates) != 0 {
		t.Fatalf("legacy resources remain owned: %+v", manifest)
	}
	for _, name := range names {
		want, err := assets.Read("files/skills/weeco/" + name)
		if err != nil {
			t.Fatal(err)
		}
		assertInstallFileContent(t, filepath.Join(fixture.skills, "weeco", name), want)
	}
	for _, path := range []string{filepath.Join(fixture.home, "config.json"), filepath.Join(fixture.home, "projects"), filepath.Join(fixture.home, "templates"), filepath.Join(fixture.commands, "weeco.md")} {
		assertInstallPathMissing(t, path)
	}
}

func TestInstallUpdatesOwnedSkillFiles(t *testing.T) {
	fixture := newInstallFixture(t)
	fixture.install(t)
	for _, name := range []string{"SKILL.md", "member-prompt.md"} {
		writeInstallFile(t, filepath.Join(fixture.skills, "weeco", name), "old installed content\n")
	}
	fixture.install(t)
	for _, name := range []string{"SKILL.md", "member-prompt.md"} {
		want, err := assets.Read("files/skills/weeco/" + name)
		if err != nil {
			t.Fatal(err)
		}
		assertInstallFileContent(t, filepath.Join(fixture.skills, "weeco", name), want)
	}
}

func TestInstallPreservesCustomFilesAcrossReinstall(t *testing.T) {
	fixture := newInstallFixture(t)
	custom := map[string]string{
		filepath.Join(fixture.skills, "weeco", "SKILL.md"):         "# custom weeco skill\n",
		filepath.Join(fixture.skills, "weeco", "member-prompt.md"): "# custom member prompt\n",
		filepath.Join(fixture.commands, "weeco.md"):                "# custom slash command\n",
		filepath.Join(fixture.home, "templates", "contract.md"):    "# custom template\n",
	}
	for path, content := range custom {
		writeInstallFile(t, path, content)
	}
	for range 2 {
		code, output := fixture.run(t)
		if code == 0 || !strings.Contains(output, "conflict") {
			t.Fatalf("unresolved custom files were not reported as conflicts: %d\n%s", code, output)
		}
		manifest := readInstallManifest(t)
		if len(manifest.Skills) != 0 || len(manifest.SkillFiles) != 0 || manifest.Command != "" || len(manifest.Templates) != 0 {
			t.Fatalf("custom resources were claimed: %+v", manifest)
		}
		for path, content := range custom {
			assertInstallFileContent(t, path, []byte(content))
		}
	}
}

func TestInstallDoesNotClaimIdenticalCustomFile(t *testing.T) {
	fixture := newInstallFixture(t)
	target := filepath.Join(fixture.skills, "weeco", "SKILL.md")
	data, err := assets.Read("files/skills/weeco/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	writeInstallFile(t, target, string(data))
	code, _ := fixture.run(t)
	if code == 0 || readInstallManifest(t).Skills["weeco"] != "" {
		t.Fatal("identical custom skill was claimed")
	}
	writeInstallFile(t, target, "customized later\n")
	code, _ = fixture.run(t)
	if code == 0 {
		t.Fatal("custom skill conflict was not reported")
	}
	assertInstallFileContent(t, target, []byte("customized later\n"))
}

func TestInstallForceClaimsCustomSkill(t *testing.T) {
	fixture := newInstallFixture(t)
	target := filepath.Join(fixture.skills, "weeco", "SKILL.md")
	writeInstallFile(t, target, "custom\n")
	fixture.install(t, "--force")
	if readInstallManifest(t).Skills["weeco"] != target {
		t.Fatal("forced skill was not claimed")
	}
	writeInstallFile(t, target, "old installed content\n")
	fixture.install(t)
	want, err := assets.Read("files/skills/weeco/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	assertInstallFileContent(t, target, want)
}

func TestInstallArchivesObsoleteOwnedFilesAndSlashCommand(t *testing.T) {
	fixture := newInstallFixture(t)
	oldSkill := filepath.Join(fixture.skills, "weeco-core", "SKILL.md")
	oldReference := filepath.Join(fixture.skills, "weeco-core", "references", "playbooks", "general.md")
	oldTemplate := filepath.Join(fixture.home, "templates", "context.md")
	oldCommand := filepath.Join(fixture.commands, "weeco.md")
	custom := filepath.Join(fixture.skills, "weeco-core", "notes.md")
	oldFiles := map[string]string{oldSkill: "old skill\n", oldReference: "old reference\n", oldTemplate: "old template\n", oldCommand: "old command\n"}
	for path, content := range oldFiles {
		writeInstallFile(t, path, content)
	}
	writeInstallFile(t, custom, "user notes\n")
	previous := installManifest{Version: "0.10.0", Skills: map[string]string{"weeco-core": oldSkill}, SkillFiles: []string{oldReference}, Templates: []string{oldTemplate}, Command: oldCommand}
	if err := sysutil.WriteJSON(store.ManifestPath(), previous); err != nil {
		t.Fatal(err)
	}
	fixture.install(t)
	for path := range oldFiles {
		assertInstallPathMissing(t, path)
	}
	assertInstallFileContent(t, custom, []byte("user notes\n"))
	archived := map[string]bool{}
	err := filepath.WalkDir(filepath.Join(fixture.home, "archives"), func(path string, entry iofs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.Contains(path, "0.10.0-to-"+Version) {
			t.Fatalf("backup lacks version archive: %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		archived[string(data)] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, content := range oldFiles {
		if !archived[content] {
			t.Fatalf("obsolete content was not backed up: %q", content)
		}
	}
	manifest := readInstallManifest(t)
	if manifest.Command != "" || len(manifest.Templates) != 0 || manifest.Skills["weeco-core"] != "" || slices.Contains(manifest.SkillFiles, oldReference) {
		t.Fatalf("obsolete paths remain owned: %+v", manifest)
	}
	fixture.install(t)
}

func TestInstallRemovesEmptyObsoleteSkillDirectories(t *testing.T) {
	fixture := newInstallFixture(t)
	oldSkill := filepath.Join(fixture.skills, "weeco-core", "SKILL.md")
	oldReference := filepath.Join(fixture.skills, "weeco-core", "references", "general.md")
	writeInstallFile(t, oldSkill, "old skill\n")
	writeInstallFile(t, oldReference, "old reference\n")
	previous := installManifest{Version: "0.10.0", Skills: map[string]string{"weeco-core": oldSkill}, SkillFiles: []string{oldReference}}
	if err := sysutil.WriteJSON(store.ManifestPath(), previous); err != nil {
		t.Fatal(err)
	}
	fixture.install(t)
	assertInstallPathMissing(t, filepath.Join(fixture.skills, "weeco-core"))
	if info, err := os.Stat(fixture.skills); err != nil || !info.IsDir() {
		t.Fatalf("install root must survive cleanup: %v", err)
	}
}

func TestInstallPreservesUnownedSlashCommandEvenWithForce(t *testing.T) {
	fixture := newInstallFixture(t)
	target := filepath.Join(fixture.commands, "weeco.md")
	writeInstallFile(t, target, "custom slash command\n")
	code, output := fixture.run(t, "--force")
	if code == 0 || !strings.Contains(output, "conflict") || !strings.Contains(output, target) {
		t.Fatalf("shadowing custom slash command was not reported: %d\n%s", code, output)
	}
	assertInstallFileContent(t, target, []byte("custom slash command\n"))
	if readInstallManifest(t).Command != "" {
		t.Fatal("custom command was claimed")
	}
}

func TestInstallCleanupStaysWithinInstallDirectories(t *testing.T) {
	fixture := newInstallFixture(t)
	outside := filepath.Join(installTempDir(t), "keep.md")
	writeInstallFile(t, outside, "outside\n")
	link := filepath.Join(fixture.skills, "escape")
	if err := os.Symlink(filepath.Dir(outside), link); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(fixture.skills, "custom-directory")
	custom := filepath.Join(directory, "keep.md")
	writeInstallFile(t, custom, "custom directory\n")
	previous := installManifest{Version: "0.10.0", Skills: map[string]string{"outside": outside}, SkillFiles: []string{filepath.Join(link, "keep.md"), directory}, Templates: []string{outside}, Command: outside}
	if err := sysutil.WriteJSON(store.ManifestPath(), previous); err != nil {
		t.Fatal(err)
	}
	fixture.install(t)
	assertInstallFileContent(t, outside, []byte("outside\n"))
	assertInstallFileContent(t, custom, []byte("custom directory\n"))
}

func TestInstallDoesNotDeleteBeforeBackupSucceeds(t *testing.T) {
	fixture := newInstallFixture(t)
	oldSkill := filepath.Join(fixture.skills, "weeco-core", "SKILL.md")
	writeInstallFile(t, oldSkill, "old skill\n")
	writeInstallFile(t, filepath.Join(fixture.home, "archives"), "blocks archive directory\n")
	previous := installManifest{Version: "0.10.0", Skills: map[string]string{"weeco-core": oldSkill}}
	if err := sysutil.WriteJSON(store.ManifestPath(), previous); err != nil {
		t.Fatal(err)
	}
	if code, _ := fixture.run(t); code == 0 {
		t.Fatal("install succeeded despite failed backup")
	}
	assertInstallFileContent(t, oldSkill, []byte("old skill\n"))
	if got := readInstallManifest(t); got.Skills["weeco-core"] != oldSkill {
		t.Fatalf("failed cleanup replaced previous manifest: %+v", got)
	}
}

func TestInstallDryRunDoesNotChangeFiles(t *testing.T) {
	fixture := newInstallFixture(t)
	oldSkill := filepath.Join(fixture.skills, "weeco-core", "SKILL.md")
	oldCommand := filepath.Join(fixture.commands, "weeco.md")
	writeInstallFile(t, oldSkill, "old skill\n")
	writeInstallFile(t, oldCommand, "old command\n")
	previous := installManifest{Version: "0.10.0", Skills: map[string]string{"weeco-core": oldSkill}, Command: oldCommand}
	if err := sysutil.WriteJSON(store.ManifestPath(), previous); err != nil {
		t.Fatal(err)
	}
	manifestBefore, err := os.ReadFile(store.ManifestPath())
	if err != nil {
		t.Fatal(err)
	}
	fixture.install(t, "--dry-run")
	assertInstallFileContent(t, oldSkill, []byte("old skill\n"))
	assertInstallFileContent(t, oldCommand, []byte("old command\n"))
	assertInstallFileContent(t, store.ManifestPath(), manifestBefore)
	for _, path := range []string{filepath.Join(fixture.home, "archives"), filepath.Join(fixture.home, "config.json"), filepath.Join(fixture.home, "templates"), filepath.Join(fixture.skills, "weeco")} {
		assertInstallPathMissing(t, path)
	}
}

func TestInstallFileDoesNotFollowSymlinkedParent(t *testing.T) {
	for _, owned := range []bool{false, true} {
		for _, force := range []bool{false, true} {
			root := installTempDir(t)
			outside := installTempDir(t)
			link := filepath.Join(root, "weeco")
			if err := os.Symlink(outside, link); err != nil {
				t.Fatal(err)
			}
			result, err := installFile(filepath.Join(link, "SKILL.md"), []byte("bundled skill\n"), 0o644, force, false, owned)
			if err != nil {
				t.Fatal(err)
			}
			if installResultKind(result) != "skipped" || !strings.Contains(result, "conflict") {
				t.Fatalf("symlinked parent was not skipped: force=%v owned=%v %s", force, owned, result)
			}
			assertInstallPathMissing(t, filepath.Join(outside, "SKILL.md"))
		}
	}
}

func TestInstallFileSkipsSymlinkAndDirectoryEvenWithForce(t *testing.T) {
	for _, kind := range []string{"directory", "symlink", "dangling symlink"} {
		t.Run(kind, func(t *testing.T) {
			target := filepath.Join(installTempDir(t), "SKILL.md")
			linkTarget := filepath.Join(installTempDir(t), "keep.md")
			if kind == "directory" {
				if err := os.Mkdir(target, 0o755); err != nil {
					t.Fatal(err)
				}
			} else {
				if kind == "symlink" {
					writeInstallFile(t, linkTarget, "user target\n")
				}
				if err := os.Symlink(linkTarget, target); err != nil {
					t.Fatal(err)
				}
			}
			result, err := installFile(target, []byte("bundled skill\n"), 0o644, true, false, true)
			if err != nil {
				t.Fatal(err)
			}
			if installResultKind(result) != "skipped" {
				t.Fatalf("non-regular path was not skipped: %s", result)
			}
			if kind == "symlink" {
				assertInstallFileContent(t, linkTarget, []byte("user target\n"))
			} else {
				assertInstallPathMissing(t, linkTarget)
			}
		})
	}
}

func TestInstallDoesNotWriteThroughSymlinkedSkillDirectory(t *testing.T) {
	fixture := newInstallFixture(t)
	outside := installTempDir(t)
	if err := os.Symlink(outside, filepath.Join(fixture.skills, "weeco")); err != nil {
		t.Fatal(err)
	}
	code, output := fixture.run(t, "--force")
	if code == 0 || !strings.Contains(output, "conflict") {
		t.Fatalf("symlinked skill directory was not reported: %d\n%s", code, output)
	}
	assertInstallPathMissing(t, filepath.Join(outside, "SKILL.md"))
	assertInstallPathMissing(t, filepath.Join(outside, "member-prompt.md"))
	manifest := readInstallManifest(t)
	if len(manifest.Skills) != 0 || len(manifest.SkillFiles) != 0 {
		t.Fatalf("symlink targets were claimed: %+v", manifest)
	}
}

func TestInstallDoesNotWriteThroughSymlinkedBinaryDirectory(t *testing.T) {
	fixture := newInstallFixture(t)
	outside := installTempDir(t)
	link := filepath.Join(installTempDir(t), "bin")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WEECO_BIN_DIR", link)
	if code, _ := fixture.run(t, "--force"); code == 0 {
		t.Fatal("install accepted a symlinked binary directory")
	}
	assertInstallPathMissing(t, filepath.Join(outside, "weeco"))
}

func TestInstallDoesNotWriteThroughSymlinkedManifest(t *testing.T) {
	fixture := newInstallFixture(t)
	outside := filepath.Join(installTempDir(t), "manifest.json")
	writeInstallFile(t, outside, "{}\n")
	if err := os.Symlink(outside, store.ManifestPath()); err != nil {
		t.Fatal(err)
	}
	if code, _ := fixture.run(t); code == 0 {
		t.Fatal("install accepted a symlinked manifest")
	}
	assertInstallFileContent(t, outside, []byte("{}\n"))
	assertInstallPathMissing(t, filepath.Join(fixture.skills, "weeco"))
}

func TestEmbeddedFilesContainOnlyWeecoSkillAndMemberPrompt(t *testing.T) {
	var files []string
	if err := iofs.WalkDir(assets.FS, "files", func(path string, entry iofs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(files, []string{"files/skills/weeco/SKILL.md", "files/skills/weeco/member-prompt.md"}) {
		t.Fatalf("unexpected embedded resources: %v", files)
	}
}

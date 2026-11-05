package commands

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"weeco/internal/store"
	"weeco/internal/sysutil"
)

type installManifest struct {
	Version     string            `json:"version"`
	Binary      string            `json:"binary"`
	Skills      map[string]string `json:"skills"`
	SkillFiles  []string          `json:"skill_files,omitempty"`
	Command     string            `json:"command,omitempty"`
	Templates   []string          `json:"templates,omitempty"`
	InstalledAt string            `json:"installed_at"`
}

func cmdInstall(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"bin-dir": true, "skill-dir": true})
	if err != nil {
		return fail(err)
	}
	for name := range fs.bools {
		if name != "help" && name != "force" && name != "dry-run" {
			return fail(fmt.Errorf("unknown install flag --%s", name))
		}
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco install [--bin-dir PATH] [--skill-dir PATH] [--force] [--dry-run]")
		return 0
	}
	if len(fs.positionals) != 0 {
		return fail(fmt.Errorf("install does not accept positional arguments"))
	}
	bin, skills := store.BinDir(), store.SkillDir()
	if value := fs.value("bin-dir"); value != "" {
		bin = sysutil.ExpandHome(value)
	}
	if value := fs.value("skill-dir"); value != "" {
		skills = sysutil.ExpandHome(value)
	}
	if bin, err = filepath.Abs(bin); err != nil {
		return fail(err)
	}
	if skills, err = filepath.Abs(skills); err != nil {
		return fail(err)
	}
	dryRun, force := fs.bool("dry-run"), fs.bool("force")
	action := func(format string, args ...any) {
		if dryRun {
			format = "(dry-run) " + format
		}
		fmt.Printf(format+"\n", args...)
	}
	manifestPath := store.ManifestPath()
	if err := checkInstallPath(manifestPath); err != nil {
		return fail(err)
	}
	var previous installManifest
	if _, err := sysutil.ReadJSONOr(manifestPath, &previous); err != nil {
		return fail(err)
	}
	owned := map[string]bool{}
	for _, path := range previous.Skills {
		owned[filepath.Clean(path)] = true
	}
	for _, path := range previous.SkillFiles {
		owned[filepath.Clean(path)] = true
	}
	files, err := skillFiles()
	if err != nil {
		return fail(err)
	}
	target := filepath.Join(bin, binaryFileName())
	if err := checkInstallPath(target); err != nil {
		return fail(err)
	}
	self, err := os.Executable()
	if err != nil {
		return fail(err)
	}
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	var sameBinary bool
	if targetInfo, err := os.Stat(target); err == nil {
		if selfInfo, err := os.Stat(self); err == nil {
			sameBinary = os.SameFile(targetInfo, selfInfo)
		}
		if !sameBinary && !force && previous.Binary != target && !isWeecoBinary(target) {
			return fail(fmt.Errorf("conflict: existing non-weeco binary %s; move it away or use --force", target))
		}
	} else if !os.IsNotExist(err) {
		return fail(err)
	}
	if sameBinary {
		action("binary already in place: %s", target)
	} else {
		if !dryRun {
			if err := copyFile(self, target); err != nil {
				return fail(err)
			}
		}
		action("install binary: %s", target)
	}
	manifest := installManifest{Version: Version, Binary: target, Skills: map[string]string{}, InstalledAt: sysutil.NowISO()}
	wanted := map[string]bool{}
	conflicts := false
	for _, file := range files {
		path := filepath.Join(skills, "weeco", file.name)
		wanted[filepath.Clean(path)] = true
		result, err := installFile(path, file.data, 0o644, force, dryRun, owned[filepath.Clean(path)])
		if err != nil {
			return fail(err)
		}
		action("%s: %s", path, result)
		if installResultKind(result) == "skipped" {
			conflicts = true
			continue
		}
		if file.name == "SKILL.md" {
			manifest.Skills["weeco"] = path
		} else {
			manifest.SkillFiles = append(manifest.SkillFiles, path)
		}
	}
	if err := archiveObsoleteInstallFiles(previous, wanted, skills, store.CommandDir(), dryRun, action); err != nil {
		return fail(err)
	}
	commandPath := filepath.Join(store.CommandDir(), "weeco.md")
	if _, err := os.Lstat(commandPath); err == nil {
		// A dry run leaves the manifest-owned command in place until a real install.
		if !dryRun || filepath.Clean(previous.Command) != filepath.Clean(commandPath) {
			action("conflict: legacy slash command %s shadows the weeco Skill; move this user file before using /weeco", commandPath)
			conflicts = true
		}
	} else if !os.IsNotExist(err) {
		return fail(err)
	}
	if !dryRun {
		if err := sysutil.WriteJSON(manifestPath, manifest); err != nil {
			return fail(err)
		}
	}
	if !pathContains(bin) {
		fmt.Printf("warning: %s is not on PATH; add it to your shell profile\n", bin)
	}
	if dryRun {
		fmt.Println("dry-run: no files changed")
	}
	if conflicts {
		return fail(fmt.Errorf("installation has unresolved file conflicts"))
	}
	if !dryRun {
		fmt.Println("installed: binary and weeco Skill")
		fmt.Println("next: invoke /weeco from the session that will lead the work")
	}
	return 0
}

func archiveObsoleteInstallFiles(previous installManifest, wanted map[string]bool, skills, commands string, dryRun bool, action func(string, ...any)) error {
	type obsoleteFile struct {
		path string
		root string
		kind string
	}
	files := map[string]obsoleteFile{}
	add := func(path, root, kind string) {
		if path != "" && !wanted[filepath.Clean(path)] {
			files[filepath.Clean(path)] = obsoleteFile{filepath.Clean(path), root, kind}
		}
	}
	for _, path := range previous.Skills {
		add(path, skills, "skills")
	}
	for _, path := range previous.SkillFiles {
		add(path, skills, "skills")
	}
	for _, path := range previous.Templates {
		add(path, filepath.Join(store.Home(), "templates"), "templates")
	}
	add(previous.Command, commands, "commands")
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	archiveDir := ""
	for _, path := range paths {
		file := files[path]
		rel, err := filepath.Rel(file.root, file.path)
		if err != nil || rel == "." || !filepath.IsLocal(rel) {
			action("keep obsolete file outside install directory: %s", file.path)
			continue
		}
		if err := checkInstallPath(file.path); err != nil {
			action("keep obsolete file: %v", err)
			continue
		}
		info, err := os.Lstat(file.path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		data, err := os.ReadFile(file.path)
		if err != nil {
			return fmt.Errorf("read obsolete file %s: %w", file.path, err)
		}
		if dryRun {
			action("archive and remove obsolete file: %s", file.path)
			continue
		}
		if archiveDir == "" {
			archiveRoot := filepath.Join(store.Home(), "archives")
			if err := checkInstallParents(filepath.Join(archiveRoot, "archive")); err != nil {
				return err
			}
			if err := os.MkdirAll(archiveRoot, 0o755); err != nil {
				return err
			}
			archiveDir, err = os.MkdirTemp(archiveRoot, archiveVersion(previous.Version)+"-to-"+archiveVersion(Version)+"-")
			if err != nil {
				return err
			}
		}
		backup := filepath.Join(archiveDir, file.kind, rel)
		if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(backup, data, info.Mode().Perm()); err != nil {
			return fmt.Errorf("archive obsolete file %s: %w", file.path, err)
		}
		if err := os.Remove(file.path); err != nil {
			return fmt.Errorf("remove archived file %s: %w", file.path, err)
		}
		action("archived obsolete file: %s -> %s", file.path, backup)
		removeEmptyParentDirs(filepath.Dir(file.path), file.root)
	}
	return nil
}

// removeEmptyParentDirs removes directories left empty after archiving an
// obsolete file. It stops at root, which is an install directory and may hold
// other user content, and never follows a path outside it.
func removeEmptyParentDirs(dir, root string) {
	root = filepath.Clean(root)
	for dir = filepath.Clean(dir); ; dir = filepath.Dir(dir) {
		rel, err := filepath.Rel(root, dir)
		if err != nil || rel == "." || !filepath.IsLocal(rel) {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
	}
}

func checkInstallParents(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	for parent := filepath.Dir(abs); ; parent = filepath.Dir(parent) {
		info, err := os.Lstat(parent)
		if err == nil && !info.IsDir() {
			return fmt.Errorf("conflict: install parent is not a real directory: %s", parent)
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if parent == filepath.Dir(parent) {
			return nil
		}
	}
}

func checkInstallPath(path string) error {
	if err := checkInstallParents(path); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("conflict: install path is not a regular file: %s", path)
	}
	return nil
}

func archiveVersion(version string) string {
	version = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, version)
	if version == "" || version == "." || version == ".." {
		return "unknown"
	}
	return version
}

func installResultKind(result string) string {
	switch {
	case strings.HasPrefix(result, "created"), strings.HasPrefix(result, "would create"):
		return "created"
	case strings.HasPrefix(result, "updated"), strings.HasPrefix(result, "would update"):
		return "updated"
	case result == "unchanged":
		return "unchanged"
	default:
		return "skipped"
	}
}

func installFile(targetPath string, data []byte, mode os.FileMode, force, dryRun, manifestOwned bool) (string, error) {
	if err := checkInstallPath(targetPath); err != nil {
		return "skipped (" + err.Error() + ")", nil
	}
	if !manifestOwned && !force {
		if _, err := os.Lstat(targetPath); err == nil {
			return "skipped (conflict: file exists and is not ours; use --force)", nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
	}
	existing, err := os.ReadFile(targetPath)
	if err == nil {
		if bytes.Equal(existing, data) {
			return "unchanged", nil
		}
		if dryRun {
			return "would update", nil
		}
		return "updated", os.WriteFile(targetPath, data, mode)
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	if dryRun {
		return "would create", nil
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return "", err
	}
	return "created", os.WriteFile(targetPath, data, mode)
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".weeco-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return err
	}
	return os.Rename(tmpName, dst)
}

func isWeecoBinary(path string) bool {
	res, err := sysutil.Run([]string{path, "version"}, "", 5*time.Second, nil)
	return err == nil && res.ExitCode == 0 && strings.HasPrefix(strings.TrimSpace(res.Stdout), "weeco ")
}

func binaryFileName() string {
	if runtime.GOOS == "windows" {
		return "weeco.exe"
	}
	return "weeco"
}

func pathContains(dir string) bool {
	target := filepath.Clean(dir)
	for _, part := range filepath.SplitList(os.Getenv("PATH")) {
		if filepath.Clean(part) == target {
			return true
		}
	}
	return false
}

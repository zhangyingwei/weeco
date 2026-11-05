package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"weeco/internal/sysutil"
)

type guardInput struct {
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		Command string `json:"command"`
	} `json:"tool_input"`
	Command string `json:"command"`
}

func cmdGuard(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"run-dir": true})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco guard --run-dir DIR")
		return 0
	}
	runDir := fs.value("run-dir")
	if runDir == "" {
		return fail(fmt.Errorf("--run-dir is required"))
	}
	runDir, err = sysutil.AbsPath(runDir)
	if err != nil {
		return fail(err)
	}
	var in guardInput
	if err := json.NewDecoder(os.Stdin).Decode(&in); err != nil {
		fmt.Fprintln(os.Stderr, "weeco guard: cannot read tool input: "+err.Error())
		return 2
	}
	command := in.ToolInput.Command
	if command == "" {
		command = in.Command
	}
	if command == "" {
		return 0
	}
	if reason := checkBash(runDir, command); reason != "" {
		fmt.Fprintln(os.Stderr, "weeco guard: "+reason)
		return 2
	}
	return 0
}

// checkBash rejects destructive shell operations outside the run directory.
// It is a guardrail, not a sandbox: shell obfuscation and scripts can bypass it.
func checkBash(runDir, command string) string {
	for _, raw := range shellSegments(command) {
		segment := strings.TrimSpace(raw)
		if segment == "" {
			continue
		}
		fields := shellFields(segment)
		for len(fields) > 0 {
			if isEnvAssignment(fields[0]) {
				fields = fields[1:]
				continue
			}
			switch fields[0] {
			case "env", "nohup", "command", "builtin":
				fields = fields[1:]
				for len(fields) > 0 && strings.HasPrefix(fields[0], "-") {
					fields = fields[1:]
				}
				continue
			}
			break
		}
		if len(fields) == 0 {
			continue
		}
		cmd := filepath.Base(fields[0])
		if cmd == "sudo" {
			return "sudo is blocked"
		}
		if reason := checkShellWrapper(runDir, cmd, fields[1:]); reason != "" {
			return reason
		}
		switch cmd {
		case "rm":
			if reason := checkRM(runDir, fields[1:]); reason != "" {
				return reason
			}
		case "find":
			if hasArg(fields[1:], "-delete") {
				if reason := checkRoot(runDir, fields[1:], "-delete"); reason != "" {
					return reason
				}
			}
		case "git":
			if reason := checkGit(runDir, fields[1:]); reason != "" {
				return reason
			}
		case "dd":
			if reason := checkDD(runDir, fields[1:]); reason != "" {
				return reason
			}
		case "chmod", "chown":
			if reason := checkTargets(runDir, fields[1:]); reason != "" {
				return reason
			}
		case "mv":
			if reason := checkTargets(runDir, fields[1:]); reason != "" {
				return reason
			}
		case "cp":
			if reason := checkCopyDestination(runDir, fields[1:]); reason != "" {
				return reason
			}
		case "mkfs", "shred", "truncate":
			if reason := checkTargets(runDir, fields[1:]); reason != "" {
				return reason
			}
		}
		if reason := checkRedirection(runDir, segment); reason != "" {
			return reason
		}
		if cmd == "cd" && len(fields) > 1 && containsDestructive(command) {
			if !resolvesInside(runDir, fields[1]) {
				return "cd outside the run directory combined with a destructive command"
			}
		}
	}
	return ""
}

// checkShellWrapper inspects commands passed to a nested shell or eval, which
// would otherwise hide a destructive command from segment parsing.
func checkShellWrapper(runDir, cmd string, args []string) string {
	switch cmd {
	case "bash", "sh", "zsh", "dash", "ksh", "fish":
		for i := 0; i < len(args); i++ {
			flag := args[i]
			if flag == "--" {
				break
			}
			if strings.HasPrefix(flag, "--") || !strings.HasPrefix(flag, "-") {
				continue
			}
			if strings.Contains(flag, "c") && i+1 < len(args) {
				return checkBash(runDir, args[i+1])
			}
		}
	case "eval":
		if len(args) > 0 {
			return checkBash(runDir, strings.Join(args, " "))
		}
	}
	return ""
}

func checkRM(runDir string, args []string) string {
	return checkTargets(runDir, args)
}

func checkTargets(runDir string, args []string) string {
	endFlags := false
	for _, arg := range args {
		if !endFlags && arg == "--" {
			endFlags = true
			continue
		}
		if !endFlags && strings.HasPrefix(arg, "-") && arg != "-" {
			continue
		}
		if reason := checkTarget(runDir, arg); reason != "" {
			return reason
		}
	}
	return ""
}

func checkTarget(runDir, target string) string {
	t := strings.TrimSpace(strings.Trim(target, "'\""))
	if t == "" || t == "-" {
		return ""
	}
	if strings.Contains(t, "$") && !strings.HasPrefix(t, "$HOME") && !strings.HasPrefix(t, "${HOME}") {
		return "unknown variable in destructive target: " + target
	}
	t = strings.ReplaceAll(t, "${HOME}", sysutil.HomeDir())
	t = strings.ReplaceAll(t, "$HOME", sysutil.HomeDir())
	if t == "~" || strings.HasPrefix(t, "~/") {
		t = filepath.Join(sysutil.HomeDir(), strings.TrimPrefix(t, "~"))
	}
	t = strings.TrimSuffix(t, "/")
	if t == "" {
		t = "/"
	}
	if strings.HasPrefix(t, "../") || strings.Contains(t, "/../") {
		return "destructive target escapes the run directory: " + target
	}
	if t == "*" || t == ".*" || t == "." || t == ".." {
		return "unsafe destructive target: " + target
	}
	if targetsRunRoot(runDir, t) {
		return "refusing to destroy the run directory itself: " + target
	}
	if filepath.IsAbs(t) {
		if resolvesInside(runDir, t) {
			return ""
		}
		return "destructive target outside the run directory: " + target
	}
	return ""
}

// targetsRunRoot reports whether a destructive target is the run directory
// itself or every entry directly under it (for example ".", "/*" or "./*").
func targetsRunRoot(runDir, target string) bool {
	root := runDir
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	path := target
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path = filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(filepath.Dir(path)); err == nil {
		path = filepath.Join(resolved, filepath.Base(path))
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || rel == "*" || rel == ".*"
}

func checkRoot(runDir string, args []string, flag string) string {
	for _, arg := range args {
		if arg == flag || strings.HasPrefix(arg, "-") {
			continue
		}
		if arg == "." {
			continue
		}
		if arg == ".." {
			return "destructive root escapes the run directory: " + arg
		}
		if reason := checkTarget(runDir, arg); reason != "" {
			return reason
		}
	}
	return ""
}

func checkCopyDestination(runDir string, args []string) string {
	dest := ""
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			dest = arg
		}
	}
	if dest == "" {
		return ""
	}
	return checkTarget(runDir, dest)
}

func checkGit(runDir string, args []string) string {
	path := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "-C" && i+1 < len(args) {
			path = args[i+1]
		}
	}
	if path == "" {
		return ""
	}
	destructive := false
	for _, arg := range args {
		if arg == "clean" || arg == "reset" || arg == "checkout" {
			destructive = true
		}
	}
	if destructive && !resolvesInside(runDir, path) {
		return "git destructive operation outside the run directory: " + path
	}
	return ""
}

func checkDD(runDir string, args []string) string {
	for _, arg := range args {
		if strings.HasPrefix(arg, "of=") {
			if reason := checkTarget(runDir, strings.TrimPrefix(arg, "of=")); reason != "" {
				return reason
			}
			if strings.HasPrefix(strings.TrimPrefix(arg, "of="), "/dev/") {
				return "blocked write to a device: " + arg
			}
		}
	}
	return ""
}

func checkRedirection(runDir, segment string) string {
	for i := 0; i < len(segment); i++ {
		if segment[i] != '>' {
			continue
		}
		j := i + 1
		if j < len(segment) && segment[j] == '>' {
			j++
		}
		for j < len(segment) && (segment[j] == ' ' || segment[j] == '\t') {
			j++
		}
		start := j
		for j < len(segment) && !unicode.IsSpace(rune(segment[j])) && !strings.ContainsRune(";|&", rune(segment[j])) {
			j++
		}
		target := strings.Trim(segment[start:j], "'\"")
		if target == "" || strings.HasPrefix(target, "&") || target == "/dev/null" || target == "/dev/stdout" || target == "/dev/stderr" {
			continue
		}
		if reason := checkTarget(runDir, target); reason != "" {
			return "blocked redirection outside the run directory: " + target
		}
	}
	return ""
}

func hasArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func containsDestructive(command string) bool {
	for _, token := range []string{"rm ", "chmod ", "chown ", "find ", "dd ", "mkfs", "shred", "truncate", ">"} {
		if strings.Contains(command, token) {
			return true
		}
	}
	return false
}

func resolvesInside(runDir, target string) bool {
	root := runDir
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	path := target
	if path == "~" || strings.HasPrefix(path, "~/") {
		path = filepath.Join(sysutil.HomeDir(), strings.TrimPrefix(path, "~"))
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path = filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	} else if resolved, err := filepath.EvalSymlinks(filepath.Dir(path)); err == nil {
		path = filepath.Join(resolved, filepath.Base(path))
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func isEnvAssignment(token string) bool {
	idx := strings.IndexByte(token, '=')
	if idx <= 0 {
		return false
	}
	for _, r := range token[:idx] {
		if !(r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}

func shellSegments(command string) []string {
	var out []string
	var b strings.Builder
	var quote byte
	escaped := false
	for i := 0; i < len(command); i++ {
		c := command[i]
		if escaped {
			b.WriteByte(c)
			escaped = false
			continue
		}
		if c == '\\' && quote != '\'' {
			b.WriteByte(c)
			escaped = true
			continue
		}
		if quote != 0 {
			b.WriteByte(c)
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			b.WriteByte(c)
			continue
		}
		if c == ';' || c == '\n' {
			out = append(out, b.String())
			b.Reset()
			continue
		}
		if c == '&' && i+1 < len(command) && command[i+1] == '&' {
			out = append(out, b.String())
			b.Reset()
			i++
			continue
		}
		if c == '|' {
			if i+1 < len(command) && command[i+1] == '|' {
				i++
			}
			out = append(out, b.String())
			b.Reset()
			continue
		}
		b.WriteByte(c)
	}
	out = append(out, b.String())
	return out
}

func shellFields(s string) []string {
	var out []string
	var b strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
	}
	for _, r := range s {
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				b.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if unicode.IsSpace(r) {
			flush()
			continue
		}
		b.WriteRune(r)
	}
	flush()
	return out
}

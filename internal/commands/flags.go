package commands

import (
	"fmt"
	"strings"
)

// flagSet is a small parser that accepts flags before or after positional
// arguments. Value flags are declared per command; every other --name is a
// boolean flag.
type flagSet struct {
	valueFlags  map[string]bool
	valuesMap   map[string][]string
	bools       map[string]bool
	positionals []string
}

func parseFlags(args []string, valueFlags map[string]bool) (*flagSet, error) {
	fs := &flagSet{
		valueFlags: valueFlags,
		valuesMap:  map[string][]string{},
		bools:      map[string]bool{},
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			fs.positionals = append(fs.positionals, args[i+1:]...)
			break
		}
		if arg == "-h" || arg == "--help" {
			fs.bools["help"] = true
			continue
		}
		if strings.HasPrefix(arg, "--") {
			name := strings.TrimPrefix(arg, "--")
			value := ""
			hasValue := false
			if idx := strings.Index(name, "="); idx >= 0 {
				value = name[idx+1:]
				name = name[:idx]
				hasValue = true
			}
			if name == "" {
				return nil, fmt.Errorf("invalid flag: %s", arg)
			}
			if fs.valueFlags[name] {
				if !hasValue {
					if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
						return nil, fmt.Errorf("flag --%s needs a value", name)
					}
					i++
					value = args[i]
				}
				fs.valuesMap[name] = append(fs.valuesMap[name], value)
				continue
			}
			if hasValue {
				switch strings.ToLower(value) {
				case "1", "true", "yes", "on":
					fs.bools[name] = true
				case "0", "false", "no", "off":
					fs.bools[name] = false
				default:
					return nil, fmt.Errorf("flag --%s does not take a value", name)
				}
				continue
			}
			fs.bools[name] = true
			continue
		}
		fs.positionals = append(fs.positionals, arg)
	}
	return fs, nil
}

func (f *flagSet) value(name string) string {
	if vs := f.valuesMap[name]; len(vs) > 0 {
		return vs[len(vs)-1]
	}
	return ""
}

func (f *flagSet) values(name string) []string { return f.valuesMap[name] }

func (f *flagSet) bool(name string) bool { return f.bools[name] }

func (f *flagSet) has(name string) bool {
	_, ok := f.bools[name]
	return ok
}

func (f *flagSet) positional(i int, name string) (string, error) {
	if i >= len(f.positionals) {
		return "", fmt.Errorf("missing %s", name)
	}
	return f.positionals[i], nil
}

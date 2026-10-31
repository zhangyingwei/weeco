package commands

import "weeco/internal/assets"

type skillFile struct {
	name string
	data []byte
}

func skillFiles() ([]skillFile, error) {
	var files []skillFile
	for _, file := range []struct{ name, asset string }{
		{"SKILL.md", assets.WeecoSkill},
		{"member-prompt.md", assets.MemberPrompt},
	} {
		data, err := assets.Read(file.asset)
		if err != nil {
			return nil, err
		}
		files = append(files, skillFile{file.name, data})
	}
	return files, nil
}

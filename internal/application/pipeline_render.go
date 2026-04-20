package application

import "strings"

func renderMasterBody(title, normalizedText string, additions []string) string {
	sections := []string{"# " + title}
	if normalizedText != "" {
		sections = append(sections, normalizedText)
	}
	body := strings.Join(sections, "\n\n")
	if len(additions) == 0 {
		return body
	}
	var additionsBlock strings.Builder
	for i, addition := range additions {
		if i > 0 {
			additionsBlock.WriteByte('\n')
		}
		additionsBlock.WriteString(renderAdditionBullet(addition))
	}
	return body + "\n\n## Additions\n\n" + additionsBlock.String()
}

func renderAdditionBullet(addition string) string {
	var b strings.Builder
	firstLine := true
	for line := range strings.SplitSeq(addition, "\n") {
		if firstLine {
			b.WriteString("- ")
			b.WriteString(line)
			firstLine = false
			continue
		}
		b.WriteString("\n  ")
		b.WriteString(line)
	}
	return b.String()
}

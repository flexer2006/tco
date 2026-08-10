package pipeline

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
	var builder strings.Builder

	firstLine := true
	for line := range strings.SplitSeq(addition, "\n") {
		if firstLine {
			builder.WriteString("- ")
			builder.WriteString(line)

			firstLine = false

			continue
		}

		builder.WriteString("\n  ")
		builder.WriteString(line)
	}

	return builder.String()
}

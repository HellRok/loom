package store

import (
	"regexp"
	"strings"
)

var (
	headingRE = regexp.MustCompile(`^ {0,3}(#{1,6})[ \t]+(.*?)(?:[ \t]+#+)?[ \t]*$`)
	fenceRE   = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})")
)

type heading struct {
	line  int
	level int
	text  string
}

// headings finds the ATX headings in body's lines, skipping fenced code.
func headings(lines []string) []heading {
	var hs []heading
	fence := ""
	for k, l := range lines {
		if m := fenceRE.FindStringSubmatch(l); m != nil {
			// A fence closes on the same character, at least as long.
			switch {
			case fence == "":
				fence = m[1]
			case m[1][0] == fence[0] && len(m[1]) >= len(fence):
				fence = ""
			}
			continue
		}
		if fence != "" {
			continue
		}
		if m := headingRE.FindStringSubmatch(l); m != nil {
			hs = append(hs, heading{line: k, level: len(m[1]), text: m[2]})
		}
	}
	return hs
}

// appendToSection adds text as the last lines of the section under the
// first heading whose text is name (ignoring case and any leading #s on
// name). The section runs to the next heading of the same or a higher
// level; trailing blank lines stay after the appended text.
func appendToSection(body, name, text string) (string, error) {
	name = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(name), "#"))
	lines := strings.Split(body, "\n")
	hs := headings(lines)
	for k, h := range hs {
		if !strings.EqualFold(h.text, name) {
			continue
		}
		end := len(lines)
		for _, next := range hs[k+1:] {
			if next.level <= h.level {
				end = next.line
				break
			}
		}
		at := end
		for at > h.line+1 && strings.TrimSpace(lines[at-1]) == "" {
			at--
		}
		add := strings.Split(strings.TrimRight(text, "\n"), "\n")
		out := append(append(append([]string{}, lines[:at]...), add...), lines[at:]...)
		return strings.Join(out, "\n"), nil
	}
	names := make([]string, len(hs))
	for k, h := range hs {
		names[k] = h.text
	}
	return "", &NoSectionError{Headings: names}
}

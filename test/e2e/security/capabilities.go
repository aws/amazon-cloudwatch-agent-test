package security

import (
	"github.com/syndtr/gocapability/capability"
	"regexp"
	"strings"
)

// extractCapabilities extracts all capabilities from the given text,
// ignoring those prefixed with "!" and removing the "cap_" prefix.
func extractCapabilities(text string) []string {
	var caps []string
	// Regex to match capabilities, capturing the part after "cap_"
	re := regexp.MustCompile(`!?cap_([a-zA-Z0-9_]+)`)
	matches := re.FindAllStringSubmatch(text, -1)

	for _, match := range matches {
		if len(match) > 1 {
			caps = append(caps, match[1]) // Append the captured capability name without "cap_"
		}
	}
	return caps
}

// separateCurrentsCapabilities extracts capabilities from "Current:" and "Current IAB:" sections.
func separateCurrentsCapabilities(block string) (current, currentIAB []string) {
	lines := strings.Split(block, "\n")

	for _, line := range lines {
		if strings.HasPrefix(line, "Current:") {
			current = extractCapabilities(line)
		} else if strings.HasPrefix(line, "Current IAB:") {
			currentIAB = extractCapabilities(line)
		}
	}
	return current, currentIAB
}

func getListOfAllLinuxCapabilities() []capability.Cap {
	return capability.List()
}

package carfile

import (
	"fmt"
	"path"
	"strings"
)

type renditionMatcher struct {
	patterns []string
}

func newRenditionMatcher(patterns []string) (renditionMatcher, error) {
	matcher := renditionMatcher{patterns: make([]string, 0, len(patterns))}
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			return renditionMatcher{}, fmt.Errorf("include pattern cannot be empty")
		}
		if _, err := path.Match(pattern, ""); err != nil {
			return renditionMatcher{}, fmt.Errorf("invalid include pattern %q: %w", pattern, err)
		}
		matcher.patterns = append(matcher.patterns, pattern)
	}
	return matcher, nil
}

func (m renditionMatcher) matches(r Rendition) bool {
	if len(m.patterns) == 0 {
		return true
	}
	candidates := []string{r.AssetName, r.CSI.Name}
	if r.AssetName != "" && r.CSI.Name != "" {
		candidates = append(candidates, r.AssetName+"/"+r.CSI.Name)
	}
	for _, pattern := range m.patterns {
		for _, candidate := range candidates {
			if matched, _ := path.Match(pattern, candidate); matched {
				return true
			}
		}
	}
	return false
}

func (m renditionMatcher) matchesCatalog(c *Catalog) bool {
	for _, rendition := range c.Renditions {
		if m.matches(rendition) {
			return true
		}
	}
	return false
}

package carfile

import "testing"

func TestRenditionMatcher(t *testing.T) {
	r := Rendition{AssetName: "AppIcon", CSI: CSI{Name: "Icon-iPhone-60@2x.png"}}
	tests := []struct {
		patterns []string
		want     bool
	}{
		{nil, true},
		{[]string{"AppIcon"}, true},
		{[]string{"Icon-*@2x.png"}, true},
		{[]string{"AppIcon/Icon-iPhone-60@2x.png"}, true},
		{[]string{"Other", "*@3x.png"}, false},
	}
	for _, test := range tests {
		matcher, err := newRenditionMatcher(test.patterns)
		if err != nil {
			t.Fatal(err)
		}
		if got := matcher.matches(r); got != test.want {
			t.Fatalf("patterns %q: got %v, want %v", test.patterns, got, test.want)
		}
	}
}

func TestRenditionMatcherRejectsInvalidGlob(t *testing.T) {
	if _, err := newRenditionMatcher([]string{"["}); err == nil {
		t.Fatal("expected invalid glob error")
	}
}

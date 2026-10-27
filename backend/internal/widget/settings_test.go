package widget

import (
	"strings"
	"testing"
)

type sample struct {
	Title    string     `json:"title,omitempty"`
	Limit    *int       `json:"limit,omitempty"`
	Enabled  bool       `json:"enabled,omitempty"`
	Ratio    float64    `json:"ratio,omitempty"`
	Projects StringList `json:"projects,omitempty"`
}

func TestDecodeSettings(t *testing.T) {
	var s sample
	err := DecodeSettings(map[string]any{
		"title": "Hallo", "limit": 5, "enabled": true, "ratio": 0.5,
		"projects": []any{"Inbox", 7, 2.5},
	}, &s)
	if err != nil {
		t.Fatal(err)
	}
	if s.Title != "Hallo" || s.Limit == nil || *s.Limit != 5 || !s.Enabled || s.Ratio != 0.5 {
		t.Errorf("decoded: %+v", s)
	}
	if strings.Join(s.Projects, "|") != "Inbox|7|2.5" {
		t.Errorf("mixed list: %v", s.Projects)
	}
}

func TestEmptySettingsAreFine(t *testing.T) {
	var s sample
	if err := DecodeSettings(nil, &s); err != nil {
		t.Fatal(err)
	}
	if err := DecodeSettings(map[string]any{}, &s); err != nil {
		t.Fatal(err)
	}
	if s.Limit != nil {
		t.Error("an unset pointer setting must stay nil (so defaults can be applied)")
	}
}

func TestStringListAcceptsASingleValue(t *testing.T) {
	var s sample
	if err := DecodeSettings(map[string]any{"projects": "Inbox"}, &s); err != nil || len(s.Projects) != 1 || s.Projects[0] != "Inbox" {
		t.Errorf("%v %v", s.Projects, err)
	}
	if err := DecodeSettings(map[string]any{"projects": 5}, &s); err != nil || s.Projects[0] != "5" {
		t.Errorf("%v %v", s.Projects, err)
	}
}

func TestDecodeErrorsAreHelpful(t *testing.T) {
	for name, c := range map[string]struct {
		in   map[string]any
		want []string
	}{
		"unknown key names the valid ones": {map[string]any{"titel": "x"}, []string{`unknown setting "titel"`, "enabled, limit, projects, ratio, title"}},
		"text instead of number":           {map[string]any{"limit": "ten"}, []string{`setting "limit"`, "a whole number", "string"}},
		"number instead of text":           {map[string]any{"title": 5}, []string{`setting "title"`, "text", "number"}},
		"text instead of bool":             {map[string]any{"enabled": "yes"}, []string{`setting "enabled"`, "true or false"}},
		"object in a list":                 {map[string]any{"projects": []any{map[string]any{"a": 1}}}, []string{"list item 1", "text or a number"}},
		"list instead of text":             {map[string]any{"title": []any{"a"}}, []string{`setting "title"`, "text"}},
	} {
		t.Run(name, func(t *testing.T) {
			var s sample
			err := DecodeSettings(c.in, &s)
			if err == nil {
				t.Fatal("expected an error")
			}
			for _, w := range c.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q should contain %q", err, w)
				}
			}
		})
	}
}

func TestValidateSettingsForUnknownTypeIsNoError(t *testing.T) {
	if err := ValidateSettings("no.such.type", map[string]any{"x": 1}, nil); err != nil {
		t.Errorf("unknown types are reported by the config loader, not here: %v", err)
	}
}

func TestFlexString(t *testing.T) {
	var s struct {
		Panel FlexString `json:"panel,omitempty"`
	}
	for in, want := range map[any]string{34: "34", "Verbrauch": "Verbrauch", 2.5: "2.5", nil: ""} {
		s.Panel = "stale"
		if err := DecodeSettings(map[string]any{"panel": in}, &s); err != nil || string(s.Panel) != want {
			t.Errorf("%v -> %q (%v), want %q", in, s.Panel, err, want)
		}
	}
	if err := DecodeSettings(map[string]any{"panel": []any{1}}, &s); err == nil {
		t.Error("a list is not a valid panel reference")
	}
}

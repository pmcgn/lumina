package widget

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// Validator is implemented by providers whose settings can be checked without
// starting them. The config loader calls it so a typo in the YAML is reported
// when the file is loaded (and the previous config stays active), not later as a
// broken widget.
type Validator interface {
	ValidateSettings(settings map[string]any, connections map[string]map[string]any) error
}

// ValidateSettings checks the settings of a widget of the given type. Types
// that do not implement Validator are accepted.
func ValidateSettings(typeID string, settings map[string]any, connections map[string]map[string]any) error {
	p, ok := New(typeID)
	if !ok {
		return nil
	}
	if v, ok := p.(Validator); ok {
		return v.ValidateSettings(settings, connections)
	}
	return nil
}

// DecodeSettings strictly decodes the loosely typed settings map (from YAML)
// into a settings struct that has json tags. Unknown keys are an error that
// names the valid ones; wrong value types are reported with the setting name.
func DecodeSettings(m map[string]any, out any) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return settingsError(err, out)
	}
	return nil
}

func settingsError(err error, target any) error {
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		name := typeErr.Field
		if name == "" {
			name = "value"
		}
		return fmt.Errorf("setting %q: expected %s, got %s", name, friendlyType(typeErr.Type), typeErr.Value)
	}
	const prefix = "json: unknown field "
	if msg := err.Error(); strings.HasPrefix(msg, prefix) {
		return fmt.Errorf("unknown setting %s (valid settings: %s)", strings.TrimPrefix(msg, prefix), strings.Join(settingNames(target), ", "))
	}
	// errors from custom UnmarshalJSON methods are already worded for users
	return fmt.Errorf("settings: %w", err)
}

func friendlyType(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "text"
	case reflect.Bool:
		return "true or false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "a whole number"
	case reflect.Float32, reflect.Float64:
		return "a number"
	case reflect.Slice:
		return "a list"
	case reflect.Map, reflect.Struct:
		return "a mapping"
	}
	return t.String()
}

// settingNames lists the json names of a settings struct, sorted.
func settingNames(target any) []string {
	t := reflect.TypeOf(target)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return nil
	}
	var names []string
	for i := 0; i < t.NumField(); i++ {
		name := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
		if name != "" && name != "-" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// StringList is a setting that is a list of text values, but may be written as
// a single value, and whose items may be numbers (e.g. project ids and names mixed).
type StringList []string

func (l *StringList) UnmarshalJSON(b []byte) error {
	var one any
	if err := json.Unmarshal(b, &one); err != nil {
		return err
	}
	switch v := one.(type) {
	case nil:
		*l = nil
	case []any:
		out := make([]string, 0, len(v))
		for i, item := range v {
			s, ok := scalarString(item)
			if !ok {
				return fmt.Errorf("list item %d must be text or a number", i+1)
			}
			out = append(out, s)
		}
		*l = out
	default:
		s, ok := scalarString(v)
		if !ok {
			return errors.New("must be a text value or a list of text values")
		}
		*l = []string{s}
	}
	return nil
}

func scalarString(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%f", x), "0"), "."), true
	}
	return "", false
}

// FlexString is a setting that is text but may be written as a number in the
// YAML (e.g. an id: `panel: 34` or a title: `panel: Verbrauch`).
type FlexString string

func (f *FlexString) UnmarshalJSON(b []byte) error {
	var one any
	if err := json.Unmarshal(b, &one); err != nil {
		return err
	}
	if one == nil {
		*f = ""
		return nil
	}
	s, ok := scalarString(one)
	if !ok {
		return errors.New("must be text or a number")
	}
	*f = FlexString(s)
	return nil
}

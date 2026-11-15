package lookup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// Field returns the value of a field in the JSON properties that the Cloud
// Control API returns for a resource. The field is a path of property names
// and array indexes separated by slashes, for example Endpoint/Address, with
// JSON Pointer escaping (~1 for / and ~0 for ~).
func Field(properties, field string) (string, error) {
	path, err := fieldPath(field)
	if err != nil {
		return "", err
	}

	d := json.NewDecoder(bytes.NewReader([]byte(properties)))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return "", fmt.Errorf("cannot parse resource properties: %w", err)
	}

	for i, p := range path {
		at := strings.Join(path[:i], "/")
		switch t := v.(type) {
		case map[string]any:
			next, ok := t[p]
			if !ok {
				return "", fmt.Errorf("resource has no property %q, available properties%s are %s", p, under(at), strings.Join(slices.Sorted(maps.Keys(t)), ", "))
			}
			v = next
		case []any:
			n, err := strconv.Atoi(p)
			if err != nil || n < 0 || n >= len(t) {
				return "", fmt.Errorf("property %s is a list of %d items, %q is not a valid index", at, len(t), p)
			}
			v = t[n]
		default:
			return "", fmt.Errorf("property %s has no property %q", at, p)
		}
	}

	switch t := v.(type) {
	case string:
		if t == "" {
			return "", fmt.Errorf("property %s is empty", field)
		}
		return t, nil
	case json.Number:
		return t.String(), nil
	default:
		return "", fmt.Errorf("property %s is not a string or number", field)
	}
}

func under(path string) string {
	if path == "" {
		return ""
	}
	return " of " + path
}

func fieldPath(field string) ([]string, error) {
	path := strings.Split(strings.TrimPrefix(field, "/"), "/")
	for i, p := range path {
		if p == "" {
			return nil, errors.New("field must not have empty path segments")
		}
		path[i] = strings.ReplaceAll(strings.ReplaceAll(p, "~1", "/"), "~0", "~")
	}
	return path, nil
}

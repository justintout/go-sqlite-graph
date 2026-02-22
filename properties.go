package graph

import "encoding/json"

// MarshalProperties converts a property map to a JSON string for storage.
// A nil or empty map returns "{}".
func MarshalProperties(props map[string]any) (string, error) {
	if len(props) == 0 {
		return "{}", nil
	}
	b, err := json.Marshal(props)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// UnmarshalProperties parses a JSON string into a property map.
// An empty string returns an empty map.
func UnmarshalProperties(raw string) (map[string]any, error) {
	if raw == "" || raw == "{}" {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, err
	}
	return m, nil
}

package security

import (
	"encoding/json"
	"strings"
)

// RedactedValue replaces a secret in redacted output. Writers that receive it
// back must treat it as "keep the stored value" instead of persisting it
// literally — see RestoreRedacted.
const RedactedValue = "***"

// secretKeys are JSON field names whose value is always a credential.
var secretKeys = map[string]bool{
	"bearer_token":         true,
	"api_key":              true,
	"oauth2_client_secret": true,
	"client_secret":        true,
	"password":             true,
	"token":                true,
	"secret":               true,
}

// secretContainers are JSON field names holding a map whose *values* are all
// potentially credentials (admin-configured outbound headers can carry
// "Authorization: Bearer ...").
var secretContainers = map[string]bool{
	"headers": true,
}

// RedactJSON parses raw JSON and returns it with every credential replaced by
// RedactedValue. Non-JSON input (or a parse failure) is returned unchanged: the
// caller is dealing with an error page or plain text, not a resource payload.
func RedactJSON(raw []byte) []byte {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return raw
	}
	out, err := json.Marshal(redactValue(v, false))
	if err != nil {
		return raw
	}
	return out
}

// redactValue walks decoded JSON. inContainer marks that every string reached
// from here is a credential (values of a "headers" map).
func redactValue(v interface{}, inContainer bool) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, val := range t {
			switch {
			case secretKeys[strings.ToLower(k)]:
				out[k] = RedactedValue
			case secretContainers[strings.ToLower(k)]:
				out[k] = redactValue(val, true)
			default:
				out[k] = redactValue(val, inContainer)
			}
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, val := range t {
			out[i] = redactValue(val, inContainer)
		}
		return out
	case string:
		if inContainer {
			return RedactedValue
		}
		return t
	default:
		return v
	}
}

// RestoreRedacted returns body with every RedactedValue replaced by the value at
// the same location in current. A client that reads a resource, edits one field
// and writes it back sends the redaction sentinel for untouched secrets;
// forwarding that literally would wipe the stored credential.
//
// Locations present only in body keep the sentinel — there is nothing to restore
// and the write handler validates it like any other value.
func RestoreRedacted(body, current []byte) []byte {
	var b, c interface{}
	if err := json.Unmarshal(body, &b); err != nil {
		return body
	}
	if err := json.Unmarshal(current, &c); err != nil {
		return body
	}
	out, err := json.Marshal(restoreValue(b, c))
	if err != nil {
		return body
	}
	return out
}

func restoreValue(body, current interface{}) interface{} {
	if s, ok := body.(string); ok && s == RedactedValue {
		if cur, ok := current.(string); ok {
			return cur
		}
		return body
	}

	switch t := body.(type) {
	case map[string]interface{}:
		curMap, _ := current.(map[string]interface{})
		out := make(map[string]interface{}, len(t))
		for k, val := range t {
			out[k] = restoreValue(val, curMap[k])
		}
		return out
	case []interface{}:
		// Arrays are matched positionally: api_key_rules[i] in the write body is
		// assumed to be the same rule as api_key_rules[i] in the stored object.
		// A reordered array restores the wrong key, so a reordering client must
		// send real values rather than the sentinel.
		curList, _ := current.([]interface{})
		out := make([]interface{}, len(t))
		for i, val := range t {
			var cur interface{}
			if i < len(curList) {
				cur = curList[i]
			}
			out[i] = restoreValue(val, cur)
		}
		return out
	default:
		return body
	}
}

// HasRedacted reports whether raw contains the redaction sentinel anywhere, so
// callers can skip the extra read when there is nothing to restore.
func HasRedacted(raw []byte) bool {
	return strings.Contains(string(raw), `"`+RedactedValue+`"`)
}

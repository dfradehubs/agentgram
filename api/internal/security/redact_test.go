package security

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactJSON(t *testing.T) {
	tests := []struct {
		name       string
		in         string
		mustHave   []string
		mustNotHav []string
	}{
		{
			name:       "top level bearer token",
			in:         `{"id":"a1","bearer_token":"ghp_real"}`,
			mustHave:   []string{`"id":"a1"`, `"bearer_token":"***"`},
			mustNotHav: []string{"ghp_real"},
		},
		{
			name:       "api keys inside array of rules",
			in:         `{"api_key_rules":[{"subject":"user@example.com","api_key":"sk-1"},{"subject":"g","api_key":"sk-2"}]}`,
			mustHave:   []string{`"api_key":"***"`, `"subject":"user@example.com"`},
			mustNotHav: []string{"sk-1", "sk-2"},
		},
		{
			name:       "header values are all redacted",
			in:         `{"headers":{"Authorization":"Bearer real","X-Trace":"on"}}`,
			mustHave:   []string{`"Authorization":"***"`, `"X-Trace":"***"`},
			mustNotHav: []string{"Bearer real", `"on"`},
		},
		{
			name:       "oauth2 client secret",
			in:         `{"oauth2_client_id":"cid","oauth2_client_secret":"csecret"}`,
			mustHave:   []string{`"oauth2_client_id":"cid"`, `"oauth2_client_secret":"***"`},
			mustNotHav: []string{"csecret"},
		},
		{
			name:       "list payload",
			in:         `{"agents":[{"id":"a","bearer_token":"t1"},{"id":"b","bearer_token":"t2"}]}`,
			mustHave:   []string{`"id":"a"`, `"bearer_token":"***"`},
			mustNotHav: []string{"t1", "t2"},
		},
		{
			name:       "key casing is ignored",
			in:         `{"Bearer_Token":"t1"}`,
			mustHave:   []string{`"Bearer_Token":"***"`},
			mustNotHav: []string{"t1"},
		},
		{
			name:       "non-string secret is still replaced",
			in:         `{"api_key":123}`,
			mustHave:   []string{`"api_key":"***"`},
			mustNotHav: []string{"123"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(RedactJSON([]byte(tt.in)))
			for _, want := range tt.mustHave {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q\ngot: %s", want, got)
				}
			}
			for _, bad := range tt.mustNotHav {
				if strings.Contains(got, bad) {
					t.Errorf("output leaked %q\ngot: %s", bad, got)
				}
			}
		})
	}
}

// Use case: the tool receives an error page or plain text instead of a resource.
func TestRedactJSONPassesThroughNonJSON(t *testing.T) {
	in := []byte("forbidden")
	if got := string(RedactJSON(in)); got != "forbidden" {
		t.Errorf("expected passthrough, got %q", got)
	}
}

// Use case: an LLM reads an agent (secrets redacted), edits the name and writes
// it back. The stored bearer token must survive.
func TestRestoreRedactedKeepsStoredSecrets(t *testing.T) {
	current := []byte(`{"id":"a1","name":"Old","bearer_token":"ghp_real","headers":{"X-Env":"prod"},"api_key_rules":[{"subject":"user@example.com","api_key":"sk-real"}]}`)
	body := []byte(`{"id":"a1","name":"New","bearer_token":"***","headers":{"X-Env":"***"},"api_key_rules":[{"subject":"user@example.com","api_key":"***"}]}`)

	var got map[string]interface{}
	if err := json.Unmarshal(RestoreRedacted(body, current), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got["name"] != "New" {
		t.Errorf("name not applied: %v", got["name"])
	}
	if got["bearer_token"] != "ghp_real" {
		t.Errorf("bearer_token not restored: %v", got["bearer_token"])
	}
	if hdr := got["headers"].(map[string]interface{}); hdr["X-Env"] != "prod" {
		t.Errorf("header not restored: %v", hdr["X-Env"])
	}
	rules := got["api_key_rules"].([]interface{})
	if rule := rules[0].(map[string]interface{}); rule["api_key"] != "sk-real" {
		t.Errorf("api_key not restored: %v", rule["api_key"])
	}
}

// Use case: an explicit new secret must overwrite the stored one.
func TestRestoreRedactedKeepsExplicitValues(t *testing.T) {
	current := []byte(`{"bearer_token":"old"}`)
	body := []byte(`{"bearer_token":"brand-new"}`)

	got := string(RestoreRedacted(body, current))
	if !strings.Contains(got, "brand-new") {
		t.Errorf("explicit value overwritten: %s", got)
	}
}

// Use case: creating a resource — there is no stored object to restore from.
func TestRestoreRedactedWithNoCurrentKeepsSentinel(t *testing.T) {
	body := []byte(`{"bearer_token":"***"}`)
	got := string(RestoreRedacted(body, []byte("not json")))
	if !strings.Contains(got, RedactedValue) {
		t.Errorf("expected sentinel to survive, got %s", got)
	}
}

func TestHasRedacted(t *testing.T) {
	if !HasRedacted([]byte(`{"a":"***"}`)) {
		t.Error("expected true")
	}
	if HasRedacted([]byte(`{"a":"real"}`)) {
		t.Error("expected false")
	}
}

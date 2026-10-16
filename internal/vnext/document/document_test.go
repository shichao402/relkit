package document

import (
	"strings"
	"testing"
)

func TestDecodeRejectsDuplicateKeysAtAnyDepth(t *testing.T) {
	var destination map[string]any
	err := Decode([]byte(`{"outer":{"value":1,"value":2}}`), &destination)
	if err == nil || !strings.Contains(err.Error(), `duplicate JSON object key "value"`) {
		t.Fatalf("unexpected result: %v", err)
	}
}

func TestDecodeRejectsTrailingValue(t *testing.T) {
	var destination map[string]any
	if err := Decode([]byte(`{"ok":true} {"other":true}`), &destination); err == nil {
		t.Fatal("trailing JSON value was accepted")
	}
}

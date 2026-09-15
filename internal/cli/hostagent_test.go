package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestHostAgentUsageErrorsPreserveJSONProtocol(t *testing.T) {
	for _, args := range [][]string{{"hostagent"}, {"hostagent", "--invalid-flag"}} {
		var output, stderr bytes.Buffer
		code := Execute(context.Background(), args, IO{In: strings.NewReader(""), Out: &output, Err: &stderr}, Dependencies{})
		if code != 1 {
			t.Fatalf("internal command returned %d: %s", code, stderr.String())
		}
		var event struct {
			Level string `json:"level"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal(stderr.Bytes(), &event); err != nil || event.Level != "error" || event.Error == "" {
			t.Fatalf("hostagent error is not a JSON event: %s (%v)", stderr.String(), err)
		}
	}
}

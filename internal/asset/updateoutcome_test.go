package asset

import (
	"bytes"
	"testing"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dashcolor "github.com/dash0hq/dash0-cli/internal/color"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrintUpdateOutcome(t *testing.T) {
	dashcolor.NoColor = true
	defer func() { dashcolor.NoColor = false }()

	deletedAt := time.Date(2026, 10, 13, 12, 50, 57, 0, time.UTC)
	errorLogs := testView("Error Logs")
	failedRequests := testView("Failed requests")
	diff := `--- View (before)
+++ View (after)
@@ -3,6 +3,6 @@
   name: Error Logs
 spec:
   display:
-    name: Error Logs
+    name: Failed requests
   type: ""
 
`

	tests := []struct {
		name    string
		outcome UpdateOutcome
		want    string
		wantErr string
	}{
		{
			name:    "active without changes",
			outcome: UpdateOutcome{Before: errorLogs, After: errorLogs},
			want:    "View \"Error Logs\": no changes\n",
		},
		{
			name:    "active with changes",
			outcome: UpdateOutcome{Before: errorLogs, After: failedRequests},
			want:    diff,
		},
		{
			name:    "restored without changes",
			outcome: UpdateOutcome{Before: errorLogs, After: errorLogs, BeforeDeletedAt: &deletedAt},
			want:    "View \"Error Logs\" restored\n",
		},
		{
			name:    "restored with changes",
			outcome: UpdateOutcome{Before: errorLogs, After: failedRequests, BeforeDeletedAt: &deletedAt},
			want:    "View \"Error Logs\" restored\n" + diff,
		},
		{
			name:    "dry run of a restore",
			outcome: UpdateOutcome{Before: errorLogs, After: failedRequests, BeforeDeletedAt: &deletedAt, DryRun: true},
			want:    "View \"Error Logs\" would be restored\n" + diff,
		},
		{
			name:    "still deleted",
			outcome: UpdateOutcome{Before: errorLogs, After: errorLogs, BeforeDeletedAt: &deletedAt, AfterDeletedAt: &deletedAt},
			wantErr: `View "Error Logs" was saved, but the API still shows it as deleted`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.outcome.DisplayKind = "View"
			tt.outcome.Name = "Error Logs"
			tt.outcome.ID = "view-id"
			var buf bytes.Buffer
			err := PrintUpdateOutcome(&buf, tt.outcome)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				assert.Empty(t, buf.String())
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, buf.String())
		})
	}
}

func testView(displayName string) *dash0api.ViewDefinition {
	return &dash0api.ViewDefinition{
		Kind:     "View",
		Metadata: dash0api.ViewMetadata{Name: "Error Logs"},
		Spec:     dash0api.ViewSpec{Display: dash0api.ViewDisplay{Name: displayName}},
	}
}

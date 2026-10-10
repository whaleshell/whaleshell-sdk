package cautem

import (
	"context"
	"fmt"
	"time"

	controlv1 "github.com/cautem/cautem-gateway/api/gen/cautem/control/v1"
)

// PostLogs appends bounded observation lines through the authenticated Control
// API. User tokens need sandbox log write access; sandbox tokens are restricted
// by the gateway to their own sandbox.
func (c *Client) PostLogs(ctx context.Context, sandbox string, lines []LogLine) error {
	if len(lines) == 0 {
		return nil
	}
	conn, err := c.controlConn()
	if err != nil {
		return err
	}
	items := make([]*controlv1.ClientLogLine, 0, len(lines))
	for _, line := range lines {
		ts := line.TS
		if ts.IsZero() {
			ts = time.Now()
		}
		items = append(items, &controlv1.ClientLogLine{
			TimestampUnixMs: ts.UnixMilli(), Source: line.Source, Level: line.Level, Text: line.Text,
		})
	}
	_, err = controlv1.NewSandboxServiceClient(conn).AppendSandboxLogs(ctx, &controlv1.AppendSandboxLogsRequest{
		Workspace: c.workspace(), SandboxName: sandbox, Lines: items,
	})
	if err != nil {
		return fmt.Errorf("append sandbox logs: %w", err)
	}
	return nil
}

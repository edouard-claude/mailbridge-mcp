package tools

import (
	"context"
	"fmt"
	"time"

	imappool "github.com/edouard-claude/mailbridge-mcp/internal/imap"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// TimeoutMiddleware bounds every tool call. On timeout it returns an error
// result to the client and drops the account's IMAP connection, which fails
// the stuck handler's pending commands and forces a reconnect on the next call.
func TimeoutMiddleware(pool *imappool.Pool, d time.Duration) server.ToolHandlerMiddleware {
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, cancel := context.WithTimeout(ctx, d)
			defer cancel()

			type result struct {
				res *mcp.CallToolResult
				err error
			}
			done := make(chan result, 1)
			go func() {
				res, err := next(ctx, req)
				done <- result{res, err}
			}()

			select {
			case r := <-done:
				return r.res, r.err
			case <-ctx.Done():
				if accountID := req.GetString("account_id", ""); accountID != "" {
					pool.Drop(accountID)
				}
				return mcp.NewToolResultError(fmt.Sprintf(
					"%s timed out after %s (IMAP/SMTP server not responding); the connection was reset, retry the call",
					req.Params.Name, d,
				)), nil
			}
		}
	}
}

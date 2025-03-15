package tools

import (
	"context"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func addHelloWorldTool(server *server.MCPServer) {
	helloWorldTool := mcp.NewTool("hello_world",
		mcp.WithDescription("A tool that responds with hello world"),
	)

	server.AddTool(helloWorldTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("Hello, World!"), nil
	})
}

package tools

import "github.com/mark3labs/mcp-go/server"

func AddTools(server *server.MCPServer) {
	addHelloWorldTool(server)
	addQueryInstantMetricTool(server)
}

package tools

import "github.com/mark3labs/mcp-go/server"

func AddTools(server *server.MCPServer) {
	addHelloWorldTool(server)
	addQueryInstantMetricTool(server)
	addGetFailedChecksTool(server)
	addGetMetricCatalogTool(server)
	addGetServiceCatalogTool(server)
	addGetOperationsTool(server)
}

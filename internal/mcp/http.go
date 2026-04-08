package mcp

import (
	"net/http"

	mcpserver "github.com/mark3labs/mcp-go/server"
)

const DefaultSSEBasePath = "/mcp"

// SSEHandler exposes the MCP server over HTTP SSE using a static base path.
func (s *Server) SSEHandler(basePath string) http.Handler {
	if basePath == "" {
		basePath = DefaultSSEBasePath
	}

	return mcpserver.NewSSEServer(
		s.mcpServer,
		mcpserver.WithStaticBasePath(basePath),
	)
}

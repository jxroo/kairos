package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jxroo/kairos/internal/config"
	"github.com/jxroo/kairos/internal/tools"
	mcpclient "github.com/mark3labs/mcp-go/client"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"go.uber.org/zap"
)

// ExternalClient manages connections to external MCP servers and registers
// their tools as proxy handlers in the Kairos tool registry.
type ExternalClient struct {
	clients  []*mcpclient.Client
	registry *tools.Registry
	logger   *zap.Logger
}

// NewExternalClient creates an ExternalClient.
func NewExternalClient(registry *tools.Registry, logger *zap.Logger) *ExternalClient {
	return &ExternalClient{
		registry: registry,
		logger:   logger,
	}
}

// ConnectAll connects to each configured external MCP server, discovers its
// tools, and registers proxy handlers. Connection failures are logged but
// non-fatal.
func (ec *ExternalClient) ConnectAll(ctx context.Context, servers []config.ExternalMCPServer) error {
	for _, srv := range servers {
		if err := ec.connectOne(ctx, srv); err != nil {
			ec.logger.Warn("external MCP server connection failed",
				zap.String("name", srv.Name),
				zap.Error(err),
			)
		}
	}
	return nil
}

func (ec *ExternalClient) connectOne(ctx context.Context, srv config.ExternalMCPServer) error {
	client, err := mcpclient.NewStdioMCPClient(srv.Command, srv.Env, srv.Args...)
	if err != nil {
		return fmt.Errorf("creating stdio client for %s: %w", srv.Name, err)
	}

	initReq := mcpgo.InitializeRequest{}
	initReq.Params.ClientInfo = mcpgo.Implementation{
		Name:    "kairos",
		Version: "0.4.0",
	}
	initReq.Params.ProtocolVersion = mcpgo.LATEST_PROTOCOL_VERSION

	if _, err := client.Initialize(ctx, initReq); err != nil {
		_ = client.Close()
		return fmt.Errorf("initializing %s: %w", srv.Name, err)
	}

	toolsList, err := client.ListTools(ctx, mcpgo.ListToolsRequest{})
	if err != nil {
		_ = client.Close()
		return fmt.Errorf("listing tools from %s: %w", srv.Name, err)
	}

	ec.clients = append(ec.clients, client)

	for _, tool := range toolsList.Tools {
		name := fmt.Sprintf("external:%s:%s", srv.Name, tool.Name)
		def := tools.ToolDefinition{
			Name:        name,
			Description: fmt.Sprintf("[%s] %s", srv.Name, tool.Description),
		}

		// Capture for closure.
		c := client
		toolName := tool.Name
		handler := func(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
			req := mcpgo.CallToolRequest{}
			req.Params.Name = toolName
			req.Params.Arguments = args

			result, err := c.CallTool(ctx, req)
			if err != nil {
				return &tools.ToolResult{Content: fmt.Sprintf("external tool error: %v", err), IsError: true}, nil
			}

			// Serialize result content.
			b, _ := json.Marshal(result.Content)
			return &tools.ToolResult{Content: string(b), IsError: result.IsError}, nil
		}

		if regErr := ec.registry.Register(def, handler); regErr != nil {
			ec.logger.Warn("failed to register external tool",
				zap.String("tool", name),
				zap.Error(regErr),
			)
		} else {
			ec.logger.Info("registered external tool", zap.String("tool", name))
		}
	}

	ec.logger.Info("connected external MCP server",
		zap.String("name", srv.Name),
		zap.Int("tools", len(toolsList.Tools)),
	)
	return nil
}

// Close disconnects all external MCP clients.
func (ec *ExternalClient) Close() error {
	for _, c := range ec.clients {
		if err := c.Close(); err != nil {
			ec.logger.Warn("error closing external MCP client", zap.Error(err))
		}
	}
	return nil
}

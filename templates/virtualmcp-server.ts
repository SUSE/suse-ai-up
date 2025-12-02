#!/usr/bin/env node

/**
 * VirtualMCP Server Template
 *
 * This template creates an MCP server from virtualMCP tool specifications
 * using the official MCP TypeScript SDK. It runs as an HTTP server with
 * streamable HTTP transport and authentication.
 */

import { Request, Response } from 'express';
import { randomUUID } from 'node:crypto';
import * as z from 'zod/v4';
import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { StreamableHTTPServerTransport } from '@modelcontextprotocol/sdk/server/streamableHttp.js';
import { requireBearerAuth } from '@modelcontextprotocol/sdk/server/auth/middleware/bearerAuth.js';
import { createMcpExpressApp } from '@modelcontextprotocol/sdk/server/index.js';
import {
    CallToolResult,
    isInitializeRequest,
} from '@modelcontextprotocol/sdk/types.js';

// Configuration from environment variables
const TOOLS_CONFIG = process.env.TOOLS_CONFIG || '[]';
const SERVER_NAME = process.env.SERVER_NAME || 'virtualmcp-server';
const MCP_PORT = process.env.MCP_PORT ? parseInt(process.env.MCP_PORT, 10) : 3000;
const BEARER_TOKEN = process.env.BEARER_TOKEN;

// Parse tool configuration - simplified format
interface VirtualMCPTool {
  name: string;
  description: string;
  inputSchema: any; // JSON Schema for tool input
}

let tools: VirtualMCPTool[] = [];

try {
  console.log('Parsing TOOLS_CONFIG...');
  const rawTools = JSON.parse(TOOLS_CONFIG);
  console.log('Raw tools count:', Array.isArray(rawTools) ? rawTools.length : 'not array');

  if (Array.isArray(rawTools)) {
    tools = rawTools.map((tool: any) => ({
      name: tool.name,
      description: tool.description,
      inputSchema: tool.inputSchema || {
        type: 'object',
        properties: {},
        required: []
      }
    }));
  }

  console.log('Final tools count:', tools.length);
  tools.forEach((tool, i) => {
    console.log(`  Tool ${i}: ${tool.name} - ${tool.description}`);
  });
} catch (error) {
  console.error('Failed to parse TOOLS_CONFIG:', error);
  process.exit(1);
}

// Create MCP server
const server = new McpServer(
  {
    name: SERVER_NAME,
    version: '1.0.0',
  },
  {
    capabilities: {
      tools: {},
    },
  }
);

// Register tools dynamically
tools.forEach((tool: VirtualMCPTool) => {
  server.registerTool(
    tool.name,
    {
      description: tool.description,
      inputSchema: tool.inputSchema,
    },
    async (args: any): Promise<CallToolResult> => {
      // For now, return a simple response
      // In a real implementation, this would execute the actual tool logic
      console.log(`Executing tool: ${tool.name}`, args);

      return {
        content: [
          {
            type: 'text',
            text: `Tool ${tool.name} executed successfully with args: ${JSON.stringify(args)}`
          }
        ]
      };
    }
  );
});

// Create Express app with MCP support
const app = createMcpExpressApp();

// Set up authentication if token is provided
let authMiddleware = null;
if (BEARER_TOKEN) {
  authMiddleware = requireBearerAuth({
    token: BEARER_TOKEN,
    requiredScopes: [],
  });
}

// Map to store transports by session ID
const transports: { [sessionId: string]: StreamableHTTPServerTransport } = {};

// MCP POST endpoint with optional auth
const mcpPostHandler = async (req: Request, res: Response) => {
  const sessionId = req.headers['mcp-session-id'] as string | undefined;
  if (sessionId) {
    console.log(`Received MCP request for session: ${sessionId}`);
  } else {
    console.log('Request body:', req.body);
  }

  try {
    let transport: StreamableHTTPServerTransport;
    if (sessionId && transports[sessionId]) {
      // Reuse existing transport
      transport = transports[sessionId];
    } else if (!sessionId && isInitializeRequest(req.body)) {
      // New initialization request
      transport = new StreamableHTTPServerTransport({
        sessionIdGenerator: () => randomUUID(),
        onsessioninitialized: sessionId => {
          console.log(`Session initialized with ID: ${sessionId}`);
          transports[sessionId] = transport;
        }
      });

      // Set up onclose handler to clean up transport when closed
      transport.onclose = () => {
        const sid = transport.sessionId;
        if (sid && transports[sid]) {
          console.log(`Transport closed for session ${sid}, removing from transports map`);
          delete transports[sid];
        }
      };

      // Connect the transport to the MCP server BEFORE handling the request
      await server.connect(transport);
    } else {
      // Invalid request - no session ID or not initialization request
      res.status(400).json({
        jsonrpc: '2.0',
        error: {
          code: -32000,
          message: 'Bad Request: No valid session ID provided'
        },
        id: null
      });
      return;
    }

    // Handle the request with existing transport
    await transport.handleRequest(req, res, req.body);
  } catch (error) {
    console.error('Error handling MCP request:', error);
    if (!res.headersSent) {
      res.status(500).json({
        jsonrpc: '2.0',
        error: {
          code: -32603,
          message: 'Internal server error'
        },
        id: null
      });
    }
  }
};

// MCP GET endpoint for SSE streams
const mcpGetHandler = async (req: Request, res: Response) => {
  const sessionId = req.headers['mcp-session-id'] as string | undefined;
  if (!sessionId || !transports[sessionId]) {
    res.status(400).send('Invalid or missing session ID');
    return;
  }

  console.log(`Establishing SSE stream for session ${sessionId}`);

  const transport = transports[sessionId];
  await transport.handleRequest(req, res);
};

// MCP DELETE endpoint for session termination
const mcpDeleteHandler = async (req: Request, res: Response) => {
  const sessionId = req.headers['mcp-session-id'] as string | undefined;
  if (!sessionId || !transports[sessionId]) {
    res.status(400).send('Invalid or missing session ID');
    return;
  }

  console.log(`Received session termination request for session ${sessionId}`);

  try {
    const transport = transports[sessionId];
    await transport.handleRequest(req, res);
  } catch (error) {
    console.error('Error handling session termination:', error);
    if (!res.headersSent) {
      res.status(500).send('Error processing session termination');
    }
  }
};

// Set up routes with conditional auth middleware
if (authMiddleware) {
  app.post('/mcp', authMiddleware, mcpPostHandler);
  app.get('/mcp', authMiddleware, mcpGetHandler);
  app.delete('/mcp', authMiddleware, mcpDeleteHandler);
} else {
  app.post('/mcp', mcpPostHandler);
  app.get('/mcp', mcpGetHandler);
  app.delete('/mcp', mcpDeleteHandler);
}

// Health check endpoint
app.get('/health', (req, res) => {
  res.json({
    status: 'healthy',
    server: SERVER_NAME,
    tools: tools.length,
    transport: 'streamable-http'
  });
});

app.listen(MCP_PORT, error => {
  if (error) {
    console.error('Failed to start server:', error);
    process.exit(1);
  }
  console.log(`VirtualMCP Streamable HTTP Server '${SERVER_NAME}' listening on port ${MCP_PORT}`);
  console.log(`Loaded ${tools.length} tools:`, tools.map(t => t.name));
});

// Handle server shutdown
process.on('SIGINT', async () => {
  console.log('Shutting down server...');

  // Close all active transports to properly clean up resources
  for (const sessionId in transports) {
    try {
      console.log(`Closing transport for session ${sessionId}`);
      await transports[sessionId].close();
      delete transports[sid];
    } catch (error) {
      console.error(`Error closing transport for session ${sessionId}:`, error);
    }
  }
  console.log('Server shutdown complete');
  process.exit(0);
});
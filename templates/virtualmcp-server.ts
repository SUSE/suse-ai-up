#!/usr/bin/env node

/**
 * VirtualMCP Server Template
 *
 * This template creates an MCP server from virtualMCP tool specifications.
 * It runs as an HTTP server with streamable HTTP transport and authentication.
 */

import { Server } from '@modelcontextprotocol/sdk/server/index.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import { SSEServerTransport } from '@modelcontextprotocol/sdk/server/sse.js';
import {
  CallToolRequestSchema,
  ErrorCode,
  ListToolsRequestSchema,
  McpError,
} from '@modelcontextprotocol/sdk/types.js';

// Configuration from environment variables
const PORT = process.env.PORT || '3000';
const AUTH_TOKEN = process.env.AUTH_TOKEN;
const TOOLS_CONFIG = process.env.TOOLS_CONFIG || '[]';

// Parse tool configuration
interface VirtualMCPTool {
  name: string;
  description: string;
  input_schema: {
    type: string;
    properties: Record<string, any>;
    required?: string[];
  };
}

let tools: VirtualMCPTool[] = [];

try {
  tools = JSON.parse(TOOLS_CONFIG);
} catch (error) {
  console.error('Failed to parse TOOLS_CONFIG:', error);
  process.exit(1);
}

// Authentication middleware
function authenticateRequest(req: any): boolean {
  if (!AUTH_TOKEN) {
    return true; // No auth required
  }

  const authHeader = req.headers.authorization || req.headers['Authorization'];
  if (!authHeader) {
    return false;
  }

  const token = authHeader.replace('Bearer ', '');
  return token === AUTH_TOKEN;
}

// Tool execution function
async function executeTool(name: string, args: any): Promise<any> {
  console.log(`Executing tool: ${name}`, args);

  // Find the tool definition
  const tool = tools.find(t => t.name === name);
  if (!tool) {
    throw new McpError(ErrorCode.MethodNotFound, `Tool '${name}' not found`);
  }

  // Validate required parameters
  if (tool.input_schema.required) {
    for (const required of tool.input_schema.required) {
      if (!(required in args)) {
        throw new McpError(ErrorCode.InvalidParams, `Missing required parameter: ${required}`);
      }
    }
  }

  // Execute tool based on its specification
  // This is a generic implementation that can be extended for specific tools
  try {
    const result = await executeVirtualMCPTool(tool, args);
    return result;
  } catch (error) {
    console.error(`Tool execution failed for ${name}:`, error);
    throw new McpError(ErrorCode.InternalError, `Tool execution failed: ${error}`);
  }
}

// Execute a virtualMCP tool based on its specification
async function executeVirtualMCPTool(tool: VirtualMCPTool, args: any): Promise<any> {
  // This is where you would implement the actual tool execution logic
  // For now, we provide mock implementations for common tool types

  switch (tool.name) {
    case 'chat_completion':
    case 'completion':
      return await executeChatCompletion(args);

    case 'read_file':
    case 'readFile':
      return await executeReadFile(args);

    case 'list_directory':
    case 'listDirectory':
    case 'list_dir':
      return await executeListDirectory(args);

    case 'write_file':
    case 'writeFile':
      return await executeWriteFile(args);

    case 'run_command':
    case 'execute_command':
      return await executeCommand(args);

    default:
      // Generic tool execution - return a structured response
      return {
        content: [{
          type: 'text',
          text: `Executed tool '${tool.name}' with parameters: ${JSON.stringify(args, null, 2)}`
        }]
      };
  }
}

// Mock implementations for common tool types
async function executeChatCompletion(args: any): Promise<any> {
  const messages = args.messages || [];
  const maxTokens = args.max_tokens || 100;

  // Mock response - in real implementation, call actual AI API
  return {
    content: [{
      type: 'text',
      text: `Mock chat completion response for ${messages.length} messages (max ${maxTokens} tokens)`
    }],
    usage: {
      prompt_tokens: messages.length * 10,
      completion_tokens: 50,
      total_tokens: messages.length * 10 + 50
    }
  };
}

async function executeReadFile(args: any): Promise<any> {
  const path = args.path;
  if (!path) {
    throw new Error('Path parameter is required');
  }

  // Mock response - in real implementation, read actual file
  return {
    content: [{
      type: 'text',
      text: `Mock file content for: ${path}\n\nThis is mock content. In a real implementation, this would read the actual file.`
    }]
  };
}

async function executeListDirectory(args: any): Promise<any> {
  const path = args.path || '.';

  // Mock response - in real implementation, list actual directory
  return {
    content: [{
      type: 'text',
      text: `Mock directory listing for: ${path}\n\n- file1.txt\n- file2.js\n- subdirectory/\n\nThis is mock content. In a real implementation, this would list the actual directory contents.`
    }]
  };
}

async function executeWriteFile(args: any): Promise<any> {
  const path = args.path;
  const content = args.content;

  if (!path || content === undefined) {
    throw new Error('Path and content parameters are required');
  }

  // Mock response - in real implementation, write actual file
  return {
    content: [{
      type: 'text',
      text: `Mock file write successful: ${path}\n\nWrote ${content.length} characters. In a real implementation, this would write to the actual file.`
    }]
  };
}

async function executeCommand(args: any): Promise<any> {
  const command = args.command;
  if (!command) {
    throw new Error('Command parameter is required');
  }

  // Mock response - in real implementation, execute actual command
  return {
    content: [{
      type: 'text',
      text: `Mock command execution: ${command}\n\nExit code: 0\n\nThis is mock output. In a real implementation, this would execute the actual command.`
    }]
  };
}

// Create MCP server
const server = new Server(
  {
    name: 'virtualmcp-server',
    version: '1.0.0',
  },
  {
    capabilities: {
      tools: {},
    },
  }
);

// List tools handler
server.setRequestHandler(ListToolsRequestSchema, async () => {
  return {
    tools: tools.map(tool => ({
      name: tool.name,
      description: tool.description,
      inputSchema: tool.input_schema,
    })),
  };
});

// Call tool handler
server.setRequestHandler(CallToolRequestSchema, async (request) => {
  const { name, arguments: args = {} } = request.params;

  try {
    const result = await executeTool(name, args);
    return result;
  } catch (error) {
    if (error instanceof McpError) {
      throw error;
    }
    throw new McpError(ErrorCode.InternalError, `Tool execution failed: ${error}`);
  }
});

// Start the server with HTTP transport
async function main() {
  console.log(`Starting VirtualMCP server on port ${PORT}`);
  console.log(`Loaded ${tools.length} tools:`, tools.map(t => t.name));

  // Use SSE transport for HTTP streaming
  const transport = new SSEServerTransport('/sse', {
    authenticate: authenticateRequest,
  });

  await server.connect(transport);

  // Start HTTP server
  const httpServer = transport.createServer();
  httpServer.listen(parseInt(PORT), '0.0.0.0', () => {
    console.log(`VirtualMCP server listening on http://0.0.0.0:${PORT}`);
    console.log(`SSE endpoint: http://0.0.0.0:${PORT}/sse`);
  });

  // Graceful shutdown
  process.on('SIGINT', async () => {
    console.log('Shutting down VirtualMCP server...');
    await server.close();
    process.exit(0);
  });
}

main().catch((error) => {
  console.error('Failed to start VirtualMCP server:', error);
  process.exit(1);
});
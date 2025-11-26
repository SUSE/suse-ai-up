#!/usr/bin/env node

/**
 * VirtualMCP Server Template
 *
 * This template creates an MCP server from virtualMCP tool specifications.
 * It runs as an HTTP server with streamable HTTP transport and authentication.
 */

import express from 'express';
import cors from 'cors';
import { Server } from '@modelcontextprotocol/sdk/server/index.js';
import {
  CallToolRequestSchema,
  ErrorCode,
  ListToolsRequestSchema,
  McpError,
  JSONRPCMessage,
} from '@modelcontextprotocol/sdk/types.js';
import axios, { AxiosRequestConfig } from 'axios';
import { Client as PostgresClient } from 'pg';
import mysql from 'mysql2/promise';
import { GraphQLClient, gql } from 'graphql-request';
import { v4 as uuidv4 } from 'uuid';

// Configuration from environment variables
const TOOLS_CONFIG = process.env.TOOLS_CONFIG || '[]';
const SERVER_NAME = process.env.SERVER_NAME || 'virtualmcp-server';
const PORT = parseInt(process.env.PORT || '3000');
const BEARER_TOKEN = process.env.BEARER_TOKEN;
const API_BASE_URL = process.env.API_BASE_URL; // No default - only set if explicitly provided

console.log('VirtualMCP Server Configuration:');
console.log('  SERVER_NAME:', SERVER_NAME);
console.log('  PORT:', PORT);
console.log('  API_BASE_URL:', API_BASE_URL || 'not set');
console.log('  TOOLS_CONFIG length:', TOOLS_CONFIG.length);
console.log('  TOOLS_CONFIG preview:', TOOLS_CONFIG.substring(0, 200) + (TOOLS_CONFIG.length > 200 ? '...' : ''));

// Transport mode
const TRANSPORT_MODE = process.argv.includes('--transport') ?
  process.argv[process.argv.indexOf('--transport') + 1] : 'stdio';

// Parse tool configuration
interface VirtualMCPTool {
  name: string;
  description: string;
  source_type: 'api' | 'database' | 'graphql';
  input_schema: {
    type: string;
    properties: Record<string, any>;
    required?: string[];
  };
  config: {
    // API config
    api_url?: string;
    api_method?: string;
    api_headers?: Record<string, string>;
    request_mapping?: Record<string, string>;
    response_mapping?: Record<string, string>;
    // Database config
    db_type?: 'postgres' | 'mysql';
    db_connection?: string;
    db_query?: string;
    db_params?: Record<string, string>;
    // GraphQL config
    graphql_url?: string;
    graphql_query?: string;
    graphql_variables?: Record<string, string>;
  };
}

// OpenAPI-style tool definition
interface OpenAPITool {
  name: string;
  title?: string;
  description: string;
  inputSchema?: {
    type: string;
    properties: Record<string, any>;
    required?: string[];
  };
  annotations?: {
    openapi?: {
      method: string;
      path: string;
    };
  };
}

let tools: VirtualMCPTool[] = [];

try {
  console.log('Parsing TOOLS_CONFIG...');
  const rawTools = JSON.parse(TOOLS_CONFIG);
  console.log('Raw tools count:', Array.isArray(rawTools) ? rawTools.length : 'not array');

  // Check if tools are in OpenAPI format or legacy format
  if (Array.isArray(rawTools) && rawTools.length > 0) {
    console.log('Processing tools array...');
    const firstTool = rawTools[0];
    console.log('First tool keys:', Object.keys(firstTool));

    // Check if it's OpenAPI format (has annotations.openapi)
    if (firstTool.annotations?.openapi) {
      console.log('Detected OpenAPI format, converting...');
      // Convert OpenAPI format to VirtualMCPTool format
      tools = rawTools.map((openApiTool: OpenAPITool) => {
        const annotations = openApiTool.annotations?.openapi;
        if (!annotations) {
          throw new Error(`Tool ${openApiTool.name} missing OpenAPI annotations`);
        }

        // Build API URL from base URL + path
        if (!API_BASE_URL) {
          throw new Error(`API_BASE_URL environment variable must be set for OpenAPI tools`);
        }
        const apiUrl = API_BASE_URL + annotations.path;
        console.log(`Converting tool ${openApiTool.name}: ${annotations.method} ${apiUrl}`);

        return {
          name: openApiTool.name,
          description: openApiTool.title ? `${openApiTool.title}: ${openApiTool.description}` : openApiTool.description,
          source_type: 'api' as const,
          input_schema: openApiTool.inputSchema || {
            type: 'object',
            properties: {},
            required: []
          },
          config: {
            api_url: apiUrl,
            api_method: annotations.method,
            api_headers: {
              'Content-Type': 'application/json',
              'Accept': 'application/json'
            }
          }
        };
      });
    } else {
      console.log('Using legacy VirtualMCPTool format...');
      // Assume it's already in VirtualMCPTool format
      tools = rawTools as VirtualMCPTool[];
    }
  } else {
    console.log('No tools found in TOOLS_CONFIG');
  }

  console.log('Final tools count:', tools.length);
  tools.forEach((tool, i) => {
    console.log(`  Tool ${i}: ${tool.name} - ${tool.description}`);
  });
} catch (error) {
  console.error('Failed to parse TOOLS_CONFIG:', error);
  process.exit(1);
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

  // Execute tool based on its source type
  try {
    const result = await executeVirtualMCPTool(tool, args);
    return result;
  } catch (error) {
    console.error(`Tool execution failed for ${name}:`, error);
    throw new McpError(ErrorCode.InternalError, `Tool execution failed: ${error}`);
  }
}

// Execute a virtualMCP tool based on its source type
async function executeVirtualMCPTool(tool: VirtualMCPTool, args: any): Promise<any> {
  switch (tool.source_type) {
    case 'api':
      return await executeApiTool(tool, args);
    case 'database':
      return await executeDatabaseTool(tool, args);
    case 'graphql':
      return await executeGraphQLTool(tool, args);
    default:
      throw new Error(`Unsupported source type: ${tool.source_type}`);
  }
}

// API Tool Execution
async function executeApiTool(tool: VirtualMCPTool, args: any): Promise<any> {
  const config = tool.config;
  if (!config.api_url) {
    throw new Error('API URL is required for API tools');
  }

  // Build request configuration
  const requestConfig: AxiosRequestConfig = {
    method: config.api_method || 'GET',
    url: config.api_url,
    headers: config.api_headers || {},
  };

  // Apply request mapping
  if (config.request_mapping) {
    if (requestConfig.method === 'GET') {
      requestConfig.params = {};
      for (const [paramKey, argKey] of Object.entries(config.request_mapping)) {
        if (args[argKey]) {
          requestConfig.params[paramKey] = args[argKey];
        }
      }
    } else {
      requestConfig.data = {};
      for (const [paramKey, argKey] of Object.entries(config.request_mapping)) {
        if (args[argKey] !== undefined) {
          requestConfig.data[paramKey] = args[argKey];
        }
      }
    }
  } else {
    // Default: pass all args as request data
    if (requestConfig.method !== 'GET') {
      requestConfig.data = args;
    } else {
      requestConfig.params = args;
    }
  }

  try {
    const response = await axios.request(requestConfig);

    // Apply response mapping
    let result = response.data;
    if (config.response_mapping) {
      result = {};
      for (const [resultKey, responsePath] of Object.entries(config.response_mapping)) {
        // Simple dot notation support
        const value = responsePath.split('.').reduce((obj, key) => obj?.[key], response.data);
        result[resultKey] = value;
      }
    }

    return {
      content: [{
        type: 'text',
        text: JSON.stringify(result, null, 2)
      }]
    };
  } catch (error: any) {
    throw new Error(`API request failed: ${error.message}`);
  }
}

// Database Tool Execution
async function executeDatabaseTool(tool: VirtualMCPTool, args: any): Promise<any> {
  const config = tool.config;
  if (!config.db_type || !config.db_connection || !config.db_query) {
    throw new Error('Database type, connection, and query are required for database tools');
  }

  try {
    let result: any;

    if (config.db_type === 'postgres') {
      const client = new PostgresClient(config.db_connection);
      await client.connect();

      // Prepare parameters
      const params: any[] = [];
      if (config.db_params) {
        for (const [paramName, argKey] of Object.entries(config.db_params)) {
          const paramIndex = parseInt(paramName.replace('$', ''));
          params[paramIndex - 1] = args[argKey];
        }
      }

      const queryResult = await client.query(config.db_query, params);
      result = queryResult.rows;

      await client.end();
    } else if (config.db_type === 'mysql') {
      const connection = await mysql.createConnection(config.db_connection);

      // Prepare parameters
      const params: any[] = [];
      if (config.db_params) {
        for (const [paramName, argKey] of Object.entries(config.db_params)) {
          params.push(args[argKey]);
        }
      }

      const [rows] = await connection.execute(config.db_query, params);
      result = rows;

      await connection.end();
    } else {
      throw new Error(`Unsupported database type: ${config.db_type}`);
    }

    return {
      content: [{
        type: 'text',
        text: JSON.stringify(result, null, 2)
      }]
    };
  } catch (error: any) {
    throw new Error(`Database query failed: ${error.message}`);
  }
}

// GraphQL Tool Execution
async function executeGraphQLTool(tool: VirtualMCPTool, args: any): Promise<any> {
  const config = tool.config;
  if (!config.graphql_url || !config.graphql_query) {
    throw new Error('GraphQL URL and query are required for GraphQL tools');
  }

  try {
    const client = new GraphQLClient(config.graphql_url);

    // Prepare variables
    const variables: Record<string, any> = {};
    if (config.graphql_variables) {
      for (const [varName, argKey] of Object.entries(config.graphql_variables)) {
        variables[varName] = args[argKey];
      }
    }

    const data = await client.request(config.graphql_query, variables);

    return {
      content: [{
        type: 'text',
        text: JSON.stringify(data, null, 2)
      }]
    };
  } catch (error: any) {
    throw new Error(`GraphQL request failed: ${error.message}`);
  }
}

// Create MCP server
const server = new Server(
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

// List tools handler
server.setRequestHandler(ListToolsRequestSchema, async () => {
  console.log('Handling tools/list request, returning', tools.length, 'tools');
  const result = {
    tools: tools.map(tool => ({
      name: tool.name,
      description: tool.description,
      inputSchema: tool.input_schema,
    })),
  };
  console.log('Tools result:', JSON.stringify(result, null, 2));
  return result;
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

// Authentication middleware
function authenticate(req: express.Request, res: express.Response, next: express.NextFunction) {
  if (!BEARER_TOKEN) {
    return next(); // No auth required if no token set
  }

  const authHeader = req.headers.authorization;
  if (!authHeader || !authHeader.startsWith('Bearer ')) {
    return res.status(401).json({ error: 'Missing or invalid authorization header' });
  }

  const token = authHeader.substring(7); // Remove 'Bearer ' prefix
  if (token !== BEARER_TOKEN) {
    return res.status(401).json({ error: 'Invalid token' });
  }

  next();
}

// Streamable HTTP transport implementation
class StreamableHTTPTransport {
  private sessions: Map<string, { messages: JSONRPCMessage[] }> = new Map();

  async handleRequest(req: express.Request, res: express.Response) {
    const sessionId = req.headers['mcp-session-id'] as string || uuidv4();

    // Set response headers
    res.setHeader('Content-Type', 'application/json');
    res.setHeader('MCP-Protocol-Version', '2024-11-05');
    res.setHeader('Cache-Control', 'no-cache');
    res.setHeader('Connection', 'keep-alive');

    if (req.method === 'POST') {
      await this.handlePost(req, res, sessionId);
    } else if (req.method === 'GET') {
      await this.handleGet(req, res, sessionId);
    } else {
      res.status(405).json({ error: 'Method not allowed' });
    }
  }

  private async handlePost(req: express.Request, res: express.Response, sessionId: string) {
    try {
      const message: any = req.body;
      let response: any;

      // Handle MCP methods directly
      if (message.method === 'tools/list') {
        response = {
          jsonrpc: '2.0',
          id: message.id,
          result: {
            tools: tools.map(tool => ({
              name: tool.name,
              description: tool.description,
              inputSchema: tool.input_schema,
            }))
          }
        };
      } else if (message.method === 'tools/call') {
        const { name, arguments: args = {} } = message.params;
        try {
          const result = await executeTool(name, args);
          response = {
            jsonrpc: '2.0',
            id: message.id,
            result: result
          };
        } catch (error: any) {
          response = {
            jsonrpc: '2.0',
            id: message.id,
            error: {
              code: error.code || -32603,
              message: error.message || 'Tool execution failed'
            }
          };
        }
      } else if (message.method === 'initialize') {
        response = {
          jsonrpc: '2.0',
          id: message.id,
          result: {
            protocolVersion: '2024-11-05',
            capabilities: {
              tools: {}
            },
            serverInfo: {
              name: SERVER_NAME,
              version: '1.0.0'
            }
          }
        };
      } else {
        response = {
          jsonrpc: '2.0',
          id: message.id,
          error: { code: -32601, message: 'Method not found' }
        };
      }

      // Store response for GET polling
      const session = this.sessions.get(sessionId) || { messages: [] };
      session.messages.push(response);
      this.sessions.set(sessionId, session);

      res.json(response);
    } catch (error) {
      console.error('Error processing request:', error);
      res.status(500).json({
        jsonrpc: '2.0',
        error: { code: -32603, message: 'Internal error' },
        id: req.body?.id || null
      });
    }
  }

  private async handleGet(req: express.Request, res: express.Response, sessionId: string) {
    const session = this.sessions.get(sessionId);
    const messages = session ? session.messages.splice(0) : [];

    res.json({ messages });
  }
}

const transport = new StreamableHTTPTransport();

// HTTP Server mode
async function startHTTPServer() {
  const app = express();

  app.use(cors({
    origin: true,
    credentials: true,
  }));

  app.use(express.json());

  // Health check endpoint
  app.get('/health', (req, res) => {
    res.json({
      status: 'healthy',
      server: SERVER_NAME,
      tools: tools.length,
      transport: 'streamable-http'
    });
  });

  // MCP endpoint with authentication
  app.use('/mcp', authenticate);

  app.all('/mcp', (req, res) => {
    transport.handleRequest(req, res);
  });

  app.listen(PORT, () => {
    console.log(`VirtualMCP HTTP server '${SERVER_NAME}' listening on port ${PORT}`);
    console.log(`Loaded ${tools.length} tools:`, tools.map(t => t.name));
    console.log(`Source types:`, Array.from(new Set(tools.map(t => t.source_type))));
  });

  // Graceful shutdown
  process.on('SIGINT', () => {
    console.log('Shutting down VirtualMCP HTTP server...');
    process.exit(0);
  });
}

// Stdio mode (legacy)
async function startStdioServer() {
  console.log(`Starting VirtualMCP server '${SERVER_NAME}' with stdio transport`);
  console.log(`Loaded ${tools.length} tools:`, tools.map(t => t.name));

  // Import stdio transport dynamically to avoid issues when not used
  const { StdioServerTransport } = await import('@modelcontextprotocol/sdk/server/stdio.js');
  const stdioTransport = new StdioServerTransport();

  await server.connect(stdioTransport);
  console.log('VirtualMCP server connected via stdio');
}

// Main entry point
async function main() {
  if (TRANSPORT_MODE === 'http') {
    await startHTTPServer();
  } else {
    await startStdioServer();
  }
}

main().catch((error) => {
  console.error('Failed to start VirtualMCP server:', error);
  process.exit(1);
});
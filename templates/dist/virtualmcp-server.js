#!/usr/bin/env node
"use strict";
/**
 * VirtualMCP Server Template
 *
 * This template creates an MCP server from virtualMCP tool specifications.
 * It runs as an HTTP server with streamable HTTP transport and authentication.
 */
Object.defineProperty(exports, "__esModule", { value: true });
const express_1 = require("express");
const cors_1 = require("cors");
const index_js_1 = require("@modelcontextprotocol/sdk/server/index.js");
const types_js_1 = require("@modelcontextprotocol/sdk/types.js");
const axios_1 = require("axios");
const pg_1 = require("pg");
const promise_1 = require("mysql2/promise");
const graphql_request_1 = require("graphql-request");
const uuid_1 = require("uuid");
// Configuration from environment variables
const TOOLS_CONFIG = process.env.TOOLS_CONFIG || '[]';
const SERVER_NAME = process.env.SERVER_NAME || 'virtualmcp-server';
const PORT = parseInt(process.env.PORT || '3000');
const BEARER_TOKEN = process.env.BEARER_TOKEN;
// Transport mode
const TRANSPORT_MODE = process.argv.includes('--transport') ?
    process.argv[process.argv.indexOf('--transport') + 1] : 'stdio';
let tools = [];
try {
    tools = JSON.parse(TOOLS_CONFIG);
}
catch (error) {
    console.error('Failed to parse TOOLS_CONFIG:', error);
    process.exit(1);
}
// Tool execution function
async function executeTool(name, args) {
    console.log(`Executing tool: ${name}`, args);
    // Find the tool definition
    const tool = tools.find(t => t.name === name);
    if (!tool) {
        throw new types_js_1.McpError(types_js_1.ErrorCode.MethodNotFound, `Tool '${name}' not found`);
    }
    // Validate required parameters
    if (tool.input_schema.required) {
        for (const required of tool.input_schema.required) {
            if (!(required in args)) {
                throw new types_js_1.McpError(types_js_1.ErrorCode.InvalidParams, `Missing required parameter: ${required}`);
            }
        }
    }
    // Execute tool based on its source type
    try {
        const result = await executeVirtualMCPTool(tool, args);
        return result;
    }
    catch (error) {
        console.error(`Tool execution failed for ${name}:`, error);
        throw new types_js_1.McpError(types_js_1.ErrorCode.InternalError, `Tool execution failed: ${error}`);
    }
}
// Execute a virtualMCP tool based on its source type
async function executeVirtualMCPTool(tool, args) {
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
async function executeApiTool(tool, args) {
    const config = tool.config;
    if (!config.api_url) {
        throw new Error('API URL is required for API tools');
    }
    // Build request configuration
    const requestConfig = {
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
        }
        else {
            requestConfig.data = {};
            for (const [paramKey, argKey] of Object.entries(config.request_mapping)) {
                if (args[argKey] !== undefined) {
                    requestConfig.data[paramKey] = args[argKey];
                }
            }
        }
    }
    else {
        // Default: pass all args as request data
        if (requestConfig.method !== 'GET') {
            requestConfig.data = args;
        }
        else {
            requestConfig.params = args;
        }
    }
    try {
        const response = await axios_1.default.request(requestConfig);
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
    }
    catch (error) {
        throw new Error(`API request failed: ${error.message}`);
    }
}
// Database Tool Execution
async function executeDatabaseTool(tool, args) {
    const config = tool.config;
    if (!config.db_type || !config.db_connection || !config.db_query) {
        throw new Error('Database type, connection, and query are required for database tools');
    }
    try {
        let result;
        if (config.db_type === 'postgres') {
            const client = new pg_1.Client(config.db_connection);
            await client.connect();
            // Prepare parameters
            const params = [];
            if (config.db_params) {
                for (const [paramName, argKey] of Object.entries(config.db_params)) {
                    const paramIndex = parseInt(paramName.replace('$', ''));
                    params[paramIndex - 1] = args[argKey];
                }
            }
            const queryResult = await client.query(config.db_query, params);
            result = queryResult.rows;
            await client.end();
        }
        else if (config.db_type === 'mysql') {
            const connection = await promise_1.default.createConnection(config.db_connection);
            // Prepare parameters
            const params = [];
            if (config.db_params) {
                for (const [paramName, argKey] of Object.entries(config.db_params)) {
                    params.push(args[argKey]);
                }
            }
            const [rows] = await connection.execute(config.db_query, params);
            result = rows;
            await connection.end();
        }
        else {
            throw new Error(`Unsupported database type: ${config.db_type}`);
        }
        return {
            content: [{
                    type: 'text',
                    text: JSON.stringify(result, null, 2)
                }]
        };
    }
    catch (error) {
        throw new Error(`Database query failed: ${error.message}`);
    }
}
// GraphQL Tool Execution
async function executeGraphQLTool(tool, args) {
    const config = tool.config;
    if (!config.graphql_url || !config.graphql_query) {
        throw new Error('GraphQL URL and query are required for GraphQL tools');
    }
    try {
        const client = new graphql_request_1.GraphQLClient(config.graphql_url);
        // Prepare variables
        const variables = {};
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
    }
    catch (error) {
        throw new Error(`GraphQL request failed: ${error.message}`);
    }
}
// Create MCP server
const server = new index_js_1.Server({
    name: SERVER_NAME,
    version: '1.0.0',
}, {
    capabilities: {
        tools: {},
    },
});
// List tools handler
server.setRequestHandler(types_js_1.ListToolsRequestSchema, async () => {
    return {
        tools: tools.map(tool => ({
            name: tool.name,
            description: tool.description,
            inputSchema: tool.input_schema,
        })),
    };
});
// Call tool handler
server.setRequestHandler(types_js_1.CallToolRequestSchema, async (request) => {
    const { name, arguments: args = {} } = request.params;
    try {
        const result = await executeTool(name, args);
        return result;
    }
    catch (error) {
        if (error instanceof types_js_1.McpError) {
            throw error;
        }
        throw new types_js_1.McpError(types_js_1.ErrorCode.InternalError, `Tool execution failed: ${error}`);
    }
});
// Authentication middleware
function authenticate(req, res, next) {
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
    constructor() {
        this.sessions = new Map();
    }
    async handleRequest(req, res) {
        const sessionId = req.headers['mcp-session-id'] || (0, uuid_1.v4)();
        // Set response headers
        res.setHeader('Content-Type', 'application/json');
        res.setHeader('MCP-Protocol-Version', '2024-11-05');
        res.setHeader('Cache-Control', 'no-cache');
        res.setHeader('Connection', 'keep-alive');
        if (req.method === 'POST') {
            await this.handlePost(req, res, sessionId);
        }
        else if (req.method === 'GET') {
            await this.handleGet(req, res, sessionId);
        }
        else {
            res.status(405).json({ error: 'Method not allowed' });
        }
    }
    async handlePost(req, res, sessionId) {
        try {
            const message = req.body;
            let response;
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
            }
            else if (message.method === 'tools/call') {
                const { name, arguments: args = {} } = message.params;
                try {
                    const result = await executeTool(name, args);
                    response = {
                        jsonrpc: '2.0',
                        id: message.id,
                        result: result
                    };
                }
                catch (error) {
                    response = {
                        jsonrpc: '2.0',
                        id: message.id,
                        error: {
                            code: error.code || -32603,
                            message: error.message || 'Tool execution failed'
                        }
                    };
                }
            }
            else if (message.method === 'initialize') {
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
            }
            else {
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
        }
        catch (error) {
            console.error('Error processing request:', error);
            res.status(500).json({
                jsonrpc: '2.0',
                error: { code: -32603, message: 'Internal error' },
                id: req.body?.id || null
            });
        }
    }
    async handleGet(req, res, sessionId) {
        const session = this.sessions.get(sessionId);
        const messages = session ? session.messages.splice(0) : [];
        res.json({ messages });
    }
}
const transport = new StreamableHTTPTransport();
// HTTP Server mode
async function startHTTPServer() {
    const app = (0, express_1.default)();
    app.use((0, cors_1.default)({
        origin: true,
        credentials: true,
    }));
    app.use(express_1.default.json());
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
    const { StdioServerTransport } = await Promise.resolve().then(() => require('@modelcontextprotocol/sdk/server/stdio.js'));
    const stdioTransport = new StdioServerTransport();
    await server.connect(stdioTransport);
    console.log('VirtualMCP server connected via stdio');
}
// Main entry point
async function main() {
    if (TRANSPORT_MODE === 'http') {
        await startHTTPServer();
    }
    else {
        await startStdioServer();
    }
}
main().catch((error) => {
    console.error('Failed to start VirtualMCP server:', error);
    process.exit(1);
});

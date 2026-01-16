package proxy

import (
	"fmt"
	"strings"
)

// MultiServerProxy handles proxying to multiple MCP servers with prefixing
type MultiServerProxy struct {
	*MCPProxyServer
	servers map[string]*MCPProxyServer
}

// NewMultiServerProxy creates a new multi-server proxy
func NewMultiServerProxy(name string) *MultiServerProxy {
	baseServer := &MCPProxyServer{
		name:        name,
		middlewares: []Middleware{},
	}

	return &MultiServerProxy{
		MCPProxyServer: baseServer,
		servers:        make(map[string]*MCPProxyServer),
	}
}

// AddServer adds a server with a prefix
func (msp *MultiServerProxy) AddServer(prefix string, server *MCPProxyServer) {
	msp.servers[prefix] = server
}

// GetTool gets a tool from the appropriate server based on prefix
func (msp *MultiServerProxy) GetTool(name string) (*Tool, error) {
	prefix, toolName := msp.splitPrefixedName(name)
	if server, exists := msp.servers[prefix]; exists {
		return server.GetTool(toolName)
	}
	return nil, fmt.Errorf("server not found for prefix: %s", prefix)
}

// GetResource gets a resource from the appropriate server based on prefix
func (msp *MultiServerProxy) GetResource(uri string) (*Resource, error) {
	prefix, resourceURI := msp.splitPrefixedURI(uri)
	if server, exists := msp.servers[prefix]; exists {
		return server.GetResource(resourceURI)
	}
	return nil, fmt.Errorf("server not found for prefix: %s", prefix)
}

// GetPrompt gets a prompt from the appropriate server based on prefix
func (msp *MultiServerProxy) GetPrompt(name string) (*Prompt, error) {
	prefix, promptName := msp.splitPrefixedName(name)
	if server, exists := msp.servers[prefix]; exists {
		return server.GetPrompt(promptName)
	}
	return nil, fmt.Errorf("server not found for prefix: %s", prefix)
}

// ListTools lists all tools from all servers with prefixes
func (msp *MultiServerProxy) ListTools() ([]*Tool, error) {
	var allTools []*Tool
	for prefix, server := range msp.servers {
		tools, err := server.ListTools()
		if err != nil {
			continue // Skip servers that fail
		}
		for _, tool := range tools {
			// Create prefixed copy
			prefixedTool := &Tool{
				Name:        msp.addPrefix(prefix, tool.Name),
				Description: tool.Description,
				InputSchema: tool.InputSchema,
				Tags:        tool.Tags,
			}
			// Mark as mirrored
			if prefixedTool.Tags == nil {
				prefixedTool.Tags = make(map[string]interface{})
			}
			prefixedTool.Tags["mirrored"] = true
			prefixedTool.Tags["server_prefix"] = prefix
			allTools = append(allTools, prefixedTool)
		}
	}
	return allTools, nil
}

// ListResources lists all resources from all servers with prefixes
func (msp *MultiServerProxy) ListResources() ([]*Resource, error) {
	var allResources []*Resource
	for prefix, server := range msp.servers {
		resources, err := server.ListResources()
		if err != nil {
			continue // Skip servers that fail
		}
		for _, resource := range resources {
			// Create prefixed copy
			prefixedResource := &Resource{
				URI:         msp.addPrefixToURI(prefix, resource.URI),
				Name:        resource.Name,
				Description: resource.Description,
				MimeType:    resource.MimeType,
				Tags:        resource.Tags,
			}
			// Mark as mirrored
			if prefixedResource.Tags == nil {
				prefixedResource.Tags = make(map[string]interface{})
			}
			prefixedResource.Tags["mirrored"] = true
			prefixedResource.Tags["server_prefix"] = prefix
			allResources = append(allResources, prefixedResource)
		}
	}
	return allResources, nil
}

// ListPrompts lists all prompts from all servers with prefixes
func (msp *MultiServerProxy) ListPrompts() ([]*Prompt, error) {
	var allPrompts []*Prompt
	for prefix, server := range msp.servers {
		prompts, err := server.ListPrompts()
		if err != nil {
			continue // Skip servers that fail
		}
		for _, prompt := range prompts {
			// Create prefixed copy
			prefixedPrompt := &Prompt{
				Name:        msp.addPrefix(prefix, prompt.Name),
				Description: prompt.Description,
				Arguments:   prompt.Arguments,
				Tags:        prompt.Tags,
			}
			// Mark as mirrored
			if prefixedPrompt.Tags == nil {
				prefixedPrompt.Tags = make(map[string]interface{})
			}
			prefixedPrompt.Tags["mirrored"] = true
			prefixedPrompt.Tags["server_prefix"] = prefix
			allPrompts = append(allPrompts, prefixedPrompt)
		}
	}
	return allPrompts, nil
}

// CopyTool creates a local copy of a mirrored tool
func (msp *MultiServerProxy) CopyTool(name string) (*Tool, error) {
	tool, err := msp.GetTool(name)
	if err != nil {
		return nil, err
	}

	// Create local copy without mirroring tags
	localTool := &Tool{
		Name:        strings.TrimPrefix(tool.Name, name[:strings.LastIndex(name, "__")+2]+"__"),
		Description: tool.Description,
		InputSchema: tool.InputSchema,
		Tags:        make(map[string]interface{}),
	}

	// Copy other tags but remove mirroring info
	for k, v := range tool.Tags {
		if k != "mirrored" && k != "server_prefix" {
			localTool.Tags[k] = v
		}
	}

	return localTool, nil
}

// CopyResource creates a local copy of a mirrored resource
func (msp *MultiServerProxy) CopyResource(uri string) (*Resource, error) {
	resource, err := msp.GetResource(uri)
	if err != nil {
		return nil, err
	}

	// Create local copy without mirroring tags
	localResource := &Resource{
		URI:         strings.TrimPrefix(resource.URI, uri[:strings.Index(uri, "://")+3]+"/"),
		Name:        resource.Name,
		Description: resource.Description,
		MimeType:    resource.MimeType,
		Tags:        make(map[string]interface{}),
	}

	// Copy other tags but remove mirroring info
	for k, v := range resource.Tags {
		if k != "mirrored" && k != "server_prefix" {
			localResource.Tags[k] = v
		}
	}

	return localResource, nil
}

// CopyPrompt creates a local copy of a mirrored prompt
func (msp *MultiServerProxy) CopyPrompt(name string) (*Prompt, error) {
	prompt, err := msp.GetPrompt(name)
	if err != nil {
		return nil, err
	}

	// Create local copy without mirroring tags
	localPrompt := &Prompt{
		Name:        strings.TrimPrefix(prompt.Name, name[:strings.LastIndex(name, "__")+2]+"__"),
		Description: prompt.Description,
		Arguments:   prompt.Arguments,
		Tags:        make(map[string]interface{}),
	}

	// Copy other tags but remove mirroring info
	for k, v := range prompt.Tags {
		if k != "mirrored" && k != "server_prefix" {
			localPrompt.Tags[k] = v
		}
	}

	return localPrompt, nil
}

// Helper methods for prefixing
func (msp *MultiServerProxy) splitPrefixedName(name string) (string, string) {
	parts := strings.SplitN(name, "__", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", name
}

func (msp *MultiServerProxy) splitPrefixedURI(uri string) (string, string) {
	// URI format: protocol://prefix/path
	if idx := strings.Index(uri, "://"); idx != -1 {
		protocolAndPrefix := uri[:idx]
		path := uri[idx+3:]

		if prefixIdx := strings.Index(path, "/"); prefixIdx != -1 {
			prefix := path[:prefixIdx]
			remainingPath := path[prefixIdx+1:]
			return prefix, protocolAndPrefix + "://" + remainingPath
		}
	}
	return "", uri
}

func (msp *MultiServerProxy) addPrefix(prefix, name string) string {
	return prefix + "__" + name
}

func (msp *MultiServerProxy) addPrefixToURI(prefix, uri string) string {
	// URI format: protocol://path -> protocol://prefix/path
	if idx := strings.Index(uri, "://"); idx != -1 {
		protocol := uri[:idx]
		path := uri[idx+3:]
		return protocol + "://" + prefix + "/" + path
	}
	return uri
}

// Tool, Resource, and Prompt are placeholder structs - these would be defined elsewhere
type Tool struct {
	Name        string
	Description string
	InputSchema interface{}
	Tags        map[string]interface{}
}

type Resource struct {
	URI         string
	Name        string
	Description string
	MimeType    string
	Tags        map[string]interface{}
}

type Prompt struct {
	Name        string
	Description string
	Arguments   interface{}
	Tags        map[string]interface{}
}

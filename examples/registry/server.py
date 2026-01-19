from fastmcp import FastMCP

mcp = FastMCP("My Simple MCP Server")

@mcp.tool
def greet(name: str) -> str:
    return f"Hello, {name}!"

if __name__ == "__main__":
    mcp.run(transport="http",port=8000,host="0.0.0.0")
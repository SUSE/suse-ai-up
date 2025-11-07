#!/usr/bin/env python3
import sys
import json

def main():
    print("Test stdio server started", file=sys.stderr)
    
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
            
        try:
            message = json.loads(line)
            print(f"Received: {message}", file=sys.stderr)
            
            if message.get("method") == "initialize":
                response = {
                    "jsonrpc": "2.0",
                    "id": message.get("id"),
                    "result": {
                        "protocolVersion": "2024-11-05",
                        "capabilities": {
                            "tools": {}
                        },
                        "serverInfo": {
                            "name": "test-server",
                            "version": "1.0.0"
                        }
                    }
                }
                print(json.dumps(response))
                sys.stdout.flush()
            elif message.get("method") == "tools/list":
                response = {
                    "jsonrpc": "2.0",
                    "id": message.get("id"),
                    "result": {
                        "tools": []
                    }
                }
                print(json.dumps(response))
                sys.stdout.flush()
                
        except Exception as e:
            print(f"Error: {e}", file=sys.stderr)

if __name__ == "__main__":
    main()
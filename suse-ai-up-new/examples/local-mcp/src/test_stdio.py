#!/usr/bin/env python3
import sys
import json

def main():
    print("Test script started", file=sys.stderr)
    sys.stderr.flush()
    try:
        # Read all input at once
        input_data = sys.stdin.read()
        print(f"Received: {input_data}", file=sys.stderr)
        sys.stderr.flush()
        try:
            request = json.loads(input_data)
            # Echo back the request as response
            response = {
                "jsonrpc": "2.0",
                "id": request.get("id"),
                "result": {"echo": request}
            }
            print(json.dumps(response))
            sys.stdout.flush()
            print(f"Sent response", file=sys.stderr)
            sys.stderr.flush()
        except json.JSONDecodeError as e:
            print(f"JSON error: {e}", file=sys.stderr)
            print(json.dumps({"error": "Invalid JSON"}))
            sys.stdout.flush()
    except Exception as e:
        print(f"Exception: {e}", file=sys.stderr)
        sys.stderr.flush()

if __name__ == "__main__":
    main()
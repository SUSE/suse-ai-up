#!/usr/bin/env python3
"""
ServiceNow MCP Server - Mock + Real Mode

An MCP server that provides ServiceNow incident management tools.
Supports both mock data for demos and real ServiceNow API connections.

Authentication Methods:
    1. OAuth 2.0 Client Credentials (Recommended)
    2. Personal Access Token (PAT) - San Diego release and later
    3. Basic Auth (username/password) - Legacy

Environment Variables:
    SERVICENOW_INSTANCE_URL: ServiceNow instance URL (e.g., https://dev12345.service-now.com)
    SERVICENOW_AUTH_METHOD: Authentication method: "oauth", "pat", or "basic" (default: "oauth")

    # For OAuth 2.0 (recommended):
    SERVICENOW_CLIENT_ID: OAuth client ID
    SERVICENOW_CLIENT_SECRET: OAuth client secret

    # For Personal Access Token:
    SERVICENOW_PAT: Personal Access Token

    # For Basic Auth (legacy):
    SERVICENOW_USERNAME: ServiceNow username
    SERVICENOW_PASSWORD: ServiceNow password

    SERVICENOW_MOCK_MODE: Set to "true" for mock data (default: true)
    MCP_HOST: Host to bind to (default: 0.0.0.0)
    MCP_PORT: Port to bind to (default: 8000)

Usage:
    # Mock mode (default)
    python servicenow_mcp_server.py

    # Real mode with OAuth 2.0 (recommended)
    SERVICENOW_MOCK_MODE=false \
    SERVICENOW_INSTANCE_URL=https://dev12345.service-now.com \
    SERVICENOW_AUTH_METHOD=oauth \
    SERVICENOW_CLIENT_ID=your_client_id \
    SERVICENOW_CLIENT_SECRET=your_client_secret \
    python servicenow_mcp_server.py

    # Real mode with Personal Access Token
    SERVICENOW_MOCK_MODE=false \
    SERVICENOW_INSTANCE_URL=https://dev12345.service-now.com \
    SERVICENOW_AUTH_METHOD=pat \
    SERVICENOW_PAT=your_personal_access_token \
    python servicenow_mcp_server.py

    # Real mode with Basic Auth (legacy)
    SERVICENOW_MOCK_MODE=false \
    SERVICENOW_INSTANCE_URL=https://dev12345.service-now.com \
    SERVICENOW_AUTH_METHOD=basic \
    SERVICENOW_USERNAME=admin \
    SERVICENOW_PASSWORD=password \
    python servicenow_mcp_server.py
"""

import json
import os
import requests
from datetime import datetime, timedelta
from flask import Flask, request, jsonify, Response
from typing import Any, Dict, List, Optional
import random
import threading

app = Flask(__name__)

# Configuration
MOCK_MODE = os.environ.get("SERVICENOW_MOCK_MODE", "true").lower() == "true"
INSTANCE_URL = os.environ.get("SERVICENOW_INSTANCE_URL", "")
AUTH_METHOD = os.environ.get("SERVICENOW_AUTH_METHOD", "oauth").lower()

# OAuth 2.0 credentials
CLIENT_ID = os.environ.get("SERVICENOW_CLIENT_ID", "")
CLIENT_SECRET = os.environ.get("SERVICENOW_CLIENT_SECRET", "")

# Personal Access Token
PAT = os.environ.get("SERVICENOW_PAT", "")

# Basic Auth (legacy)
USERNAME = os.environ.get("SERVICENOW_USERNAME", "")
PASSWORD = os.environ.get("SERVICENOW_PASSWORD", "")
HOST = os.environ.get("MCP_HOST", "0.0.0.0")
PORT = int(os.environ.get("MCP_PORT", "8000"))

# Mock data for demo purposes
MOCK_INCIDENTS = [
    {
        "number": "INC0010001",
        "short_description": "Database connection timeout on production cluster",
        "priority": "1",
        "state": "2",  # In Progress
        "category": "Database",
        "subcategory": "Performance",
        "assigned_to": "John Smith",
        "assignment_group": "Database Operations",
        "sys_created_on": (datetime.now() - timedelta(hours=4)).isoformat(),
        "sys_updated_on": (datetime.now() - timedelta(minutes=30)).isoformat(),
        "caller_id": "Jane Doe",
        "impact": "1",
        "urgency": "1",
        "description": "Production database cluster experiencing intermittent connection timeouts. Error rate spiked to 15% starting at 14:00 UTC. Databricks ETL jobs are failing.",
        "work_notes": "Investigating connection pool settings. Initial analysis shows max connections reached."
    },
    {
        "number": "INC0010002",
        "short_description": "Databricks cluster auto-scaling failure",
        "priority": "2",
        "state": "1",  # New
        "category": "Cloud Infrastructure",
        "subcategory": "Databricks",
        "assigned_to": "",
        "assignment_group": "Cloud Platform Team",
        "sys_created_on": (datetime.now() - timedelta(hours=2)).isoformat(),
        "sys_updated_on": (datetime.now() - timedelta(hours=2)).isoformat(),
        "caller_id": "Mike Johnson",
        "impact": "2",
        "urgency": "2",
        "description": "Databricks cluster failed to auto-scale during peak processing. Jobs queuing up.",
        "work_notes": ""
    },
    {
        "number": "INC0010003",
        "short_description": "ETL pipeline data quality alert",
        "priority": "3",
        "state": "2",  # In Progress
        "category": "Data Platform",
        "subcategory": "ETL",
        "assigned_to": "Sarah Wilson",
        "assignment_group": "Data Engineering",
        "sys_created_on": (datetime.now() - timedelta(days=1)).isoformat(),
        "sys_updated_on": (datetime.now() - timedelta(hours=6)).isoformat(),
        "caller_id": "Data Quality Monitor",
        "impact": "3",
        "urgency": "2",
        "description": "Data quality checks failing on customer_transactions table. Null values detected in required fields.",
        "work_notes": "Root cause identified: upstream system sending incomplete records."
    },
    {
        "number": "INC0010004",
        "short_description": "API gateway latency spike",
        "priority": "2",
        "state": "6",  # Resolved
        "category": "Application",
        "subcategory": "API",
        "assigned_to": "Tom Brown",
        "assignment_group": "API Platform",
        "sys_created_on": (datetime.now() - timedelta(days=2)).isoformat(),
        "sys_updated_on": (datetime.now() - timedelta(hours=12)).isoformat(),
        "caller_id": "Monitoring System",
        "impact": "2",
        "urgency": "2",
        "description": "API gateway P99 latency exceeded 500ms threshold.",
        "work_notes": "Resolved by scaling up API gateway pods and optimizing database queries."
    },
    {
        "number": "INC0010005",
        "short_description": "ML model serving degraded performance",
        "priority": "2",
        "state": "2",  # In Progress
        "category": "Machine Learning",
        "subcategory": "Model Serving",
        "assigned_to": "Lisa Chen",
        "assignment_group": "ML Platform",
        "sys_created_on": (datetime.now() - timedelta(hours=8)).isoformat(),
        "sys_updated_on": (datetime.now() - timedelta(hours=1)).isoformat(),
        "caller_id": "ML Pipeline Monitor",
        "impact": "2",
        "urgency": "2",
        "description": "Recommendation model serving latency increased 3x. Affecting customer experience on e-commerce platform.",
        "work_notes": "Investigating model complexity and feature store performance."
    }
]

MOCK_CMDB_CIS = [
    {
        "sys_id": "ci001",
        "name": "prod-db-cluster-01",
        "sys_class_name": "cmdb_ci_database",
        "operational_status": "1",
        "environment": "Production",
        "location": "US-East-1",
        "assigned_to": "Database Operations",
        "comments": "Primary production database cluster"
    },
    {
        "sys_id": "ci002",
        "name": "databricks-workspace-prod",
        "sys_class_name": "cmdb_ci_cloud_service",
        "operational_status": "1",
        "environment": "Production",
        "location": "US-East-1",
        "assigned_to": "Cloud Platform Team",
        "comments": "Production Databricks workspace for analytics"
    },
    {
        "sys_id": "ci003",
        "name": "etl-pipeline-main",
        "sys_class_name": "cmdb_ci_service",
        "operational_status": "1",
        "environment": "Production",
        "location": "US-East-1",
        "assigned_to": "Data Engineering",
        "comments": "Main ETL data pipeline"
    }
]

# State and priority mappings
STATE_MAP = {
    "1": "New",
    "2": "In Progress",
    "3": "On Hold",
    "6": "Resolved",
    "7": "Closed"
}

PRIORITY_MAP = {
    "1": "Critical",
    "2": "High",
    "3": "Medium",
    "4": "Low",
    "5": "Planning"
}

# Tool definitions
TOOLS = [
    {
        "name": "search_incidents",
        "description": "Search for ServiceNow incidents by various criteria (priority, state, category, assignment group, correlation_id)",
        "inputSchema": {
            "type": "object",
            "properties": {
                "query": {
                    "type": "string",
                    "description": "Free text search query"
                },
                "priority": {
                    "type": "string",
                    "description": "Filter by priority: 1 (Critical), 2 (High), 3 (Medium), 4 (Low)"
                },
                "state": {
                    "type": "string",
                    "description": "Filter by state: 1 (New), 2 (In Progress), 3 (On Hold), 6 (Resolved), 7 (Closed)"
                },
                "category": {
                    "type": "string",
                    "description": "Filter by category (e.g., Database, Cloud Infrastructure, Data Platform)"
                },
                "assignment_group": {
                    "type": "string",
                    "description": "Filter by assignment group"
                },
                "correlation_id": {
                    "type": "string",
                    "description": "Filter by correlation ID (e.g., SUSE-AI-DEMO for demo incidents)"
                },
                "limit": {
                    "type": "integer",
                    "description": "Maximum number of results (default: 10)"
                }
            },
            "required": []
        }
    },
    {
        "name": "get_incident",
        "description": "Get detailed information about a specific incident by its number",
        "inputSchema": {
            "type": "object",
            "properties": {
                "incident_number": {
                    "type": "string",
                    "description": "The incident number (e.g., INC0010001)"
                }
            },
            "required": ["incident_number"]
        }
    },
    {
        "name": "create_incident",
        "description": "Create a new incident in ServiceNow",
        "inputSchema": {
            "type": "object",
            "properties": {
                "short_description": {
                    "type": "string",
                    "description": "Brief description of the incident"
                },
                "description": {
                    "type": "string",
                    "description": "Detailed description of the incident"
                },
                "priority": {
                    "type": "string",
                    "description": "Priority level: 1 (Critical), 2 (High), 3 (Medium), 4 (Low)"
                },
                "category": {
                    "type": "string",
                    "description": "Incident category"
                },
                "assignment_group": {
                    "type": "string",
                    "description": "Group to assign the incident to"
                },
                "caller_id": {
                    "type": "string",
                    "description": "Name of the person reporting the incident"
                }
            },
            "required": ["short_description", "description"]
        }
    },
    {
        "name": "update_incident",
        "description": "Update an existing incident (add work notes, change state, etc.)",
        "inputSchema": {
            "type": "object",
            "properties": {
                "incident_number": {
                    "type": "string",
                    "description": "The incident number to update"
                },
                "state": {
                    "type": "string",
                    "description": "New state: 1 (New), 2 (In Progress), 3 (On Hold), 6 (Resolved), 7 (Closed)"
                },
                "work_notes": {
                    "type": "string",
                    "description": "Work notes to add to the incident"
                },
                "assigned_to": {
                    "type": "string",
                    "description": "Person to assign the incident to"
                }
            },
            "required": ["incident_number"]
        }
    },
    {
        "name": "get_related_cis",
        "description": "Get Configuration Items (CIs) related to an incident",
        "inputSchema": {
            "type": "object",
            "properties": {
                "incident_number": {
                    "type": "string",
                    "description": "The incident number"
                }
            },
            "required": ["incident_number"]
        }
    },
    {
        "name": "search_cmdb",
        "description": "Search the CMDB for configuration items",
        "inputSchema": {
            "type": "object",
            "properties": {
                "query": {
                    "type": "string",
                    "description": "Search query for CI name or description"
                },
                "ci_class": {
                    "type": "string",
                    "description": "CI class to filter by (e.g., cmdb_ci_database, cmdb_ci_server)"
                },
                "environment": {
                    "type": "string",
                    "description": "Environment filter (e.g., Production, Development)"
                }
            },
            "required": []
        }
    },
    {
        "name": "get_incident_metrics",
        "description": "Get aggregate metrics about incidents (counts by priority, state, category)",
        "inputSchema": {
            "type": "object",
            "properties": {
                "time_range": {
                    "type": "string",
                    "description": "Time range: last_hour, last_day, last_week (default: last_day)"
                }
            },
            "required": []
        }
    }
]


class ServiceNowClient:
    """Client for interacting with ServiceNow REST API

    Supports multiple authentication methods:
    - OAuth 2.0 Client Credentials (recommended)
    - Personal Access Token (PAT)
    - Basic Auth (legacy)
    """

    def __init__(self, instance_url: str, auth_method: str = "oauth",
                 client_id: str = None, client_secret: str = None,
                 pat: str = None, username: str = None, password: str = None):
        self.instance_url = instance_url.rstrip('/')
        self.auth_method = auth_method
        self.client_id = client_id
        self.client_secret = client_secret
        self.pat = pat
        self.username = username
        self.password = password

        # OAuth token cache
        self._oauth_token = None
        self._token_expiry = None
        self._token_lock = threading.Lock()

        self.headers = {
            "Content-Type": "application/json",
            "Accept": "application/json"
        }

    def _get_oauth_token(self) -> str:
        """Get OAuth 2.0 access token using client credentials grant"""
        with self._token_lock:
            # Check if we have a valid cached token
            if self._oauth_token and self._token_expiry:
                if datetime.now() < self._token_expiry:
                    return self._oauth_token

            # Request new token
            token_url = f"{self.instance_url}/oauth_token.do"
            data = {
                "grant_type": "client_credentials",
                "client_id": self.client_id,
                "client_secret": self.client_secret
            }

            response = requests.post(token_url, data=data, timeout=30)
            response.raise_for_status()
            token_data = response.json()

            self._oauth_token = token_data["access_token"]
            # Set expiry with 60 second buffer
            expires_in = token_data.get("expires_in", 1800)
            self._token_expiry = datetime.now() + timedelta(seconds=expires_in - 60)

            return self._oauth_token

    def _get_auth_headers(self) -> Dict[str, str]:
        """Get authentication headers based on auth method"""
        headers = self.headers.copy()

        if self.auth_method == "oauth":
            token = self._get_oauth_token()
            headers["Authorization"] = f"Bearer {token}"
        elif self.auth_method == "pat":
            headers["Authorization"] = f"Bearer {self.pat}"
        # For basic auth, we use the auth parameter in requests

        return headers

    def _make_request(self, method: str, endpoint: str, params: Dict = None, data: Dict = None) -> Dict:
        url = f"{self.instance_url}/api/now/{endpoint}"
        headers = self._get_auth_headers()

        # Use basic auth tuple only for basic auth method
        auth = None
        if self.auth_method == "basic":
            auth = (self.username, self.password)

        response = requests.request(
            method=method,
            url=url,
            auth=auth,
            headers=headers,
            params=params,
            json=data,
            timeout=30
        )
        response.raise_for_status()
        return response.json()

    def search_incidents(self, query: str = None, priority: str = None, state: str = None,
                        category: str = None, assignment_group: str = None,
                        correlation_id: str = None, limit: int = 10) -> List[Dict]:
        sysparm_query_parts = []

        if query:
            sysparm_query_parts.append(f"short_descriptionLIKE{query}^ORdescriptionLIKE{query}")
        if priority:
            sysparm_query_parts.append(f"priority={priority}")
        if state:
            sysparm_query_parts.append(f"state={state}")
        if category:
            sysparm_query_parts.append(f"category={category}")
        if assignment_group:
            sysparm_query_parts.append(f"assignment_group.nameLIKE{assignment_group}")
        if correlation_id:
            sysparm_query_parts.append(f"correlation_id={correlation_id}")

        params = {
            "sysparm_limit": limit,
            "sysparm_query": "^".join(sysparm_query_parts) if sysparm_query_parts else ""
        }

        result = self._make_request("GET", "table/incident", params=params)
        return result.get("result", [])

    def get_incident(self, incident_number: str) -> Optional[Dict]:
        params = {"sysparm_query": f"number={incident_number}"}
        result = self._make_request("GET", "table/incident", params=params)
        incidents = result.get("result", [])
        return incidents[0] if incidents else None

    def create_incident(self, data: Dict) -> Dict:
        result = self._make_request("POST", "table/incident", data=data)
        return result.get("result", {})

    def update_incident(self, sys_id: str, data: Dict) -> Dict:
        result = self._make_request("PATCH", f"table/incident/{sys_id}", data=data)
        return result.get("result", {})


def execute_tool_mock(name: str, arguments: Dict) -> Dict:
    """Execute a tool using mock data"""

    if name == "search_incidents":
        results = MOCK_INCIDENTS.copy()

        # Apply filters
        if arguments.get("priority"):
            results = [i for i in results if i["priority"] == arguments["priority"]]
        if arguments.get("state"):
            results = [i for i in results if i["state"] == arguments["state"]]
        if arguments.get("category"):
            results = [i for i in results if arguments["category"].lower() in i["category"].lower()]
        if arguments.get("assignment_group"):
            results = [i for i in results if arguments["assignment_group"].lower() in i["assignment_group"].lower()]
        if arguments.get("query"):
            query = arguments["query"].lower()
            results = [i for i in results if query in i["short_description"].lower() or query in i["description"].lower()]

        limit = arguments.get("limit", 10)
        results = results[:limit]

        # Enrich with readable values
        for incident in results:
            incident["state_display"] = STATE_MAP.get(incident["state"], incident["state"])
            incident["priority_display"] = PRIORITY_MAP.get(incident["priority"], incident["priority"])

        return {
            "mode": "mock",
            "count": len(results),
            "incidents": results
        }

    elif name == "get_incident":
        incident_number = arguments.get("incident_number", "")
        for incident in MOCK_INCIDENTS:
            if incident["number"] == incident_number:
                incident["state_display"] = STATE_MAP.get(incident["state"], incident["state"])
                incident["priority_display"] = PRIORITY_MAP.get(incident["priority"], incident["priority"])
                return {"mode": "mock", "incident": incident}
        return {"error": f"Incident {incident_number} not found"}

    elif name == "create_incident":
        new_number = f"INC{random.randint(1000000, 9999999)}"
        new_incident = {
            "number": new_number,
            "short_description": arguments.get("short_description", ""),
            "description": arguments.get("description", ""),
            "priority": arguments.get("priority", "3"),
            "state": "1",
            "category": arguments.get("category", "Inquiry / Help"),
            "assigned_to": "",
            "assignment_group": arguments.get("assignment_group", ""),
            "caller_id": arguments.get("caller_id", "System"),
            "sys_created_on": datetime.now().isoformat(),
            "sys_updated_on": datetime.now().isoformat(),
            "work_notes": ""
        }
        MOCK_INCIDENTS.append(new_incident)
        return {
            "mode": "mock",
            "message": f"Incident {new_number} created successfully",
            "incident": new_incident
        }

    elif name == "update_incident":
        incident_number = arguments.get("incident_number", "")
        for incident in MOCK_INCIDENTS:
            if incident["number"] == incident_number:
                if arguments.get("state"):
                    incident["state"] = arguments["state"]
                if arguments.get("work_notes"):
                    if incident["work_notes"]:
                        incident["work_notes"] += f"\n\n{datetime.now().isoformat()}: {arguments['work_notes']}"
                    else:
                        incident["work_notes"] = f"{datetime.now().isoformat()}: {arguments['work_notes']}"
                if arguments.get("assigned_to"):
                    incident["assigned_to"] = arguments["assigned_to"]
                incident["sys_updated_on"] = datetime.now().isoformat()
                return {"mode": "mock", "message": f"Incident {incident_number} updated", "incident": incident}
        return {"error": f"Incident {incident_number} not found"}

    elif name == "get_related_cis":
        incident_number = arguments.get("incident_number", "")
        # Return mock CIs based on incident category
        incident = None
        for inc in MOCK_INCIDENTS:
            if inc["number"] == incident_number:
                incident = inc
                break

        if not incident:
            return {"error": f"Incident {incident_number} not found"}

        # Simple logic: return relevant CIs based on category
        related_cis = []
        category = incident.get("category", "").lower()
        if "database" in category:
            related_cis = [ci for ci in MOCK_CMDB_CIS if "database" in ci["sys_class_name"]]
        elif "databricks" in category.lower() or "cloud" in category.lower():
            related_cis = [ci for ci in MOCK_CMDB_CIS if "cloud" in ci["sys_class_name"] or "databricks" in ci["name"].lower()]
        elif "data" in category.lower() or "etl" in category.lower():
            related_cis = [ci for ci in MOCK_CMDB_CIS if "pipeline" in ci["name"].lower() or "databricks" in ci["name"].lower()]
        else:
            related_cis = MOCK_CMDB_CIS[:2]  # Return first two CIs

        return {"mode": "mock", "incident": incident_number, "related_cis": related_cis}

    elif name == "search_cmdb":
        results = MOCK_CMDB_CIS.copy()

        if arguments.get("query"):
            query = arguments["query"].lower()
            results = [ci for ci in results if query in ci["name"].lower() or query in ci.get("comments", "").lower()]
        if arguments.get("ci_class"):
            results = [ci for ci in results if arguments["ci_class"] in ci["sys_class_name"]]
        if arguments.get("environment"):
            results = [ci for ci in results if arguments["environment"].lower() == ci["environment"].lower()]

        return {"mode": "mock", "count": len(results), "configuration_items": results}

    elif name == "get_incident_metrics":
        # Generate mock metrics
        open_incidents = [i for i in MOCK_INCIDENTS if i["state"] in ["1", "2", "3"]]

        metrics = {
            "mode": "mock",
            "time_range": arguments.get("time_range", "last_day"),
            "total_incidents": len(MOCK_INCIDENTS),
            "open_incidents": len(open_incidents),
            "by_priority": {},
            "by_state": {},
            "by_category": {}
        }

        for incident in MOCK_INCIDENTS:
            priority = PRIORITY_MAP.get(incident["priority"], "Unknown")
            state = STATE_MAP.get(incident["state"], "Unknown")
            category = incident.get("category", "Unknown")

            metrics["by_priority"][priority] = metrics["by_priority"].get(priority, 0) + 1
            metrics["by_state"][state] = metrics["by_state"].get(state, 0) + 1
            metrics["by_category"][category] = metrics["by_category"].get(category, 0) + 1

        return metrics

    else:
        return {"error": f"Unknown tool: {name}"}


def execute_tool_real(name: str, arguments: Dict, client: ServiceNowClient) -> Dict:
    """Execute a tool using real ServiceNow API"""

    try:
        if name == "search_incidents":
            incidents = client.search_incidents(
                query=arguments.get("query"),
                priority=arguments.get("priority"),
                state=arguments.get("state"),
                category=arguments.get("category"),
                assignment_group=arguments.get("assignment_group"),
                correlation_id=arguments.get("correlation_id"),
                limit=arguments.get("limit", 10)
            )
            return {"mode": "real", "count": len(incidents), "incidents": incidents}

        elif name == "get_incident":
            incident = client.get_incident(arguments["incident_number"])
            if incident:
                return {"mode": "real", "incident": incident}
            return {"error": f"Incident {arguments['incident_number']} not found"}

        elif name == "create_incident":
            data = {
                "short_description": arguments["short_description"],
                "description": arguments.get("description", ""),
                "priority": arguments.get("priority", "3"),
                "category": arguments.get("category", ""),
                "assignment_group": arguments.get("assignment_group", ""),
                "caller_id": arguments.get("caller_id", "")
            }
            incident = client.create_incident(data)
            return {"mode": "real", "message": "Incident created", "incident": incident}

        elif name == "update_incident":
            incident = client.get_incident(arguments["incident_number"])
            if not incident:
                return {"error": f"Incident {arguments['incident_number']} not found"}

            data = {}
            if arguments.get("state"):
                data["state"] = arguments["state"]
            if arguments.get("work_notes"):
                data["work_notes"] = arguments["work_notes"]
            if arguments.get("assigned_to"):
                data["assigned_to"] = arguments["assigned_to"]

            updated = client.update_incident(incident["sys_id"], data)
            return {"mode": "real", "message": "Incident updated", "incident": updated}

        else:
            return execute_tool_mock(name, arguments)

    except requests.exceptions.RequestException as e:
        return {"error": f"ServiceNow API error: {str(e)}"}


def handle_jsonrpc(data: Dict) -> Optional[Dict]:
    """Handle a JSON-RPC request"""
    method = data.get("method", "")
    params = data.get("params", {})
    req_id = data.get("id")

    if method == "initialize":
        return {
            "jsonrpc": "2.0",
            "id": req_id,
            "result": {
                "protocolVersion": "2025-06-18",
                "serverInfo": {
                    "name": "servicenow-mcp-server",
                    "version": "1.0.0"
                },
                "capabilities": {
                    "tools": {"listChanged": False},
                    "resources": {"listChanged": False},
                    "prompts": {"listChanged": False}
                }
            }
        }

    elif method == "initialized":
        return None

    elif method == "tools/list":
        return {
            "jsonrpc": "2.0",
            "id": req_id,
            "result": {"tools": TOOLS}
        }

    elif method == "tools/call":
        tool_name = params.get("name", "")
        arguments = params.get("arguments", {})

        if MOCK_MODE:
            result = execute_tool_mock(tool_name, arguments)
        else:
            client = ServiceNowClient(
                instance_url=INSTANCE_URL,
                auth_method=AUTH_METHOD,
                client_id=CLIENT_ID,
                client_secret=CLIENT_SECRET,
                pat=PAT,
                username=USERNAME,
                password=PASSWORD
            )
            result = execute_tool_real(tool_name, arguments, client)

        if "error" in result:
            content = [{"type": "text", "text": f"Error: {result['error']}"}]
            is_error = True
        else:
            content = [{"type": "text", "text": json.dumps(result, indent=2, default=str)}]
            is_error = False

        return {
            "jsonrpc": "2.0",
            "id": req_id,
            "result": {"content": content, "isError": is_error}
        }

    elif method == "resources/list":
        return {
            "jsonrpc": "2.0",
            "id": req_id,
            "result": {"resources": []}
        }

    elif method == "prompts/list":
        return {
            "jsonrpc": "2.0",
            "id": req_id,
            "result": {"prompts": []}
        }

    else:
        return {
            "jsonrpc": "2.0",
            "id": req_id,
            "error": {"code": -32601, "message": f"Method not found: {method}"}
        }


@app.route('/mcp', methods=['POST', 'GET'])
def mcp_endpoint():
    """Main MCP endpoint"""
    if request.method == 'GET':
        return Response("SSE not implemented", status=501)

    try:
        data = request.get_json()
        if not data:
            return jsonify({"error": "No JSON data"}), 400

        response = handle_jsonrpc(data)

        if response is None:
            return '', 200

        resp = jsonify(response)
        resp.headers['MCP-Protocol-Version'] = '2025-06-18'
        return resp

    except Exception as e:
        return jsonify({
            "jsonrpc": "2.0",
            "id": None,
            "error": {"code": -32700, "message": str(e)}
        }), 500


@app.route('/health', methods=['GET'])
def health():
    return jsonify({
        "status": "healthy",
        "mode": "mock" if MOCK_MODE else "real",
        "instance_url": INSTANCE_URL if not MOCK_MODE else "N/A (mock mode)",
        "auth_method": AUTH_METHOD if not MOCK_MODE else "N/A (mock mode)"
    })


if __name__ == '__main__':
    print("=" * 70)
    print("  ServiceNow MCP Server")
    print("=" * 70)
    print()
    print(f"  Mode: {'MOCK (demo data)' if MOCK_MODE else 'REAL (ServiceNow API)'}")
    if not MOCK_MODE:
        print(f"  Instance: {INSTANCE_URL}")
        print(f"  Auth Method: {AUTH_METHOD}")
    print()
    print("  Available tools:")
    for tool in TOOLS:
        print(f"    - {tool['name']}: {tool['description'][:60]}...")
    print()
    print(f"  Server running at: http://{HOST}:{PORT}/mcp")
    print()
    print("  Environment variables:")
    print("    SERVICENOW_MOCK_MODE=true|false (default: true)")
    print("    SERVICENOW_INSTANCE_URL=https://your-instance.service-now.com")
    print("    SERVICENOW_AUTH_METHOD=oauth|pat|basic (default: oauth)")
    print()
    print("    # OAuth 2.0 (recommended):")
    print("    SERVICENOW_CLIENT_ID=your_client_id")
    print("    SERVICENOW_CLIENT_SECRET=your_client_secret")
    print()
    print("    # Personal Access Token:")
    print("    SERVICENOW_PAT=your_personal_access_token")
    print()
    print("    # Basic Auth (legacy):")
    print("    SERVICENOW_USERNAME=your_username")
    print("    SERVICENOW_PASSWORD=your_password")
    print()
    print("  Press Ctrl+C to stop")
    print()

    app.run(host=HOST, port=PORT, debug=False)

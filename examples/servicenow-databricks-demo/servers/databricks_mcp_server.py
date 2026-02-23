#!/usr/bin/env python3
"""
Databricks MCP Server - Mock + Real Mode

An MCP server that provides Databricks workspace, SQL, and job management tools.
Supports both mock data for demos and real Databricks API connections.

Environment Variables:
    DATABRICKS_HOST: Databricks workspace URL (e.g., https://dbc-xxxxx.cloud.databricks.com)
    DATABRICKS_TOKEN: Databricks personal access token
    DATABRICKS_MOCK_MODE: Set to "true" for mock data (default: true)
    MCP_HOST: Host to bind to (default: 0.0.0.0)
    MCP_PORT: Port to bind to (default: 8001)

Usage:
    # Mock mode (default)
    python databricks_mcp_server.py

    # Real mode
    DATABRICKS_MOCK_MODE=false \
    DATABRICKS_HOST=https://dbc-xxxxx.cloud.databricks.com \
    DATABRICKS_TOKEN=dapi-xxxxxxxx \
    python databricks_mcp_server.py
"""

import json
import os
import requests
from datetime import datetime, timedelta
from flask import Flask, request, jsonify, Response
from typing import Any, Dict, List, Optional
import random

app = Flask(__name__)

# Configuration
MOCK_MODE = os.environ.get("DATABRICKS_MOCK_MODE", "true").lower() == "true"
DATABRICKS_HOST = os.environ.get("DATABRICKS_HOST", "")
DATABRICKS_TOKEN = os.environ.get("DATABRICKS_TOKEN", "")
HOST = os.environ.get("MCP_HOST", "0.0.0.0")
PORT = int(os.environ.get("MCP_PORT", "8001"))

# Mock data for demo purposes
MOCK_CLUSTERS = [
    {
        "cluster_id": "1234-567890-abc123",
        "cluster_name": "production-etl-cluster",
        "state": "RUNNING",
        "state_message": "",
        "spark_version": "13.3.x-scala2.12",
        "node_type_id": "i3.xlarge",
        "num_workers": 8,
        "autoscale": {"min_workers": 4, "max_workers": 16},
        "creator_user_name": "data-team@company.com",
        "start_time": (datetime.now() - timedelta(hours=12)).timestamp() * 1000,
        "last_activity_time": (datetime.now() - timedelta(minutes=5)).timestamp() * 1000,
        "cluster_memory_mb": 65536,
        "cluster_cores": 32
    },
    {
        "cluster_id": "2345-678901-bcd234",
        "cluster_name": "ml-training-cluster",
        "state": "TERMINATED",
        "state_message": "Terminated by autoscale",
        "spark_version": "13.3.x-ml-scala2.12",
        "node_type_id": "p3.2xlarge",
        "num_workers": 4,
        "creator_user_name": "ml-team@company.com",
        "start_time": (datetime.now() - timedelta(days=1)).timestamp() * 1000,
        "terminated_time": (datetime.now() - timedelta(hours=2)).timestamp() * 1000,
        "cluster_memory_mb": 131072,
        "cluster_cores": 64
    },
    {
        "cluster_id": "3456-789012-cde345",
        "cluster_name": "analytics-interactive",
        "state": "RUNNING",
        "state_message": "",
        "spark_version": "14.1.x-scala2.12",
        "node_type_id": "m5.2xlarge",
        "num_workers": 2,
        "creator_user_name": "analytics@company.com",
        "start_time": (datetime.now() - timedelta(hours=3)).timestamp() * 1000,
        "last_activity_time": (datetime.now() - timedelta(minutes=15)).timestamp() * 1000,
        "cluster_memory_mb": 32768,
        "cluster_cores": 16
    }
]

MOCK_JOBS = [
    {
        "job_id": 101,
        "settings": {
            "name": "Daily ETL Pipeline",
            "schedule": {"quartz_cron_expression": "0 0 6 * * ?", "timezone_id": "UTC"},
            "email_notifications": {"on_failure": ["data-team@company.com"]}
        },
        "created_time": (datetime.now() - timedelta(days=90)).timestamp() * 1000,
        "creator_user_name": "data-team@company.com"
    },
    {
        "job_id": 102,
        "settings": {
            "name": "Customer Analytics Refresh",
            "schedule": {"quartz_cron_expression": "0 30 * * * ?", "timezone_id": "UTC"},
            "email_notifications": {"on_failure": ["analytics@company.com"]}
        },
        "created_time": (datetime.now() - timedelta(days=60)).timestamp() * 1000,
        "creator_user_name": "analytics@company.com"
    },
    {
        "job_id": 103,
        "settings": {
            "name": "ML Model Retraining",
            "schedule": {"quartz_cron_expression": "0 0 2 * * 0", "timezone_id": "UTC"},
            "email_notifications": {"on_failure": ["ml-team@company.com"]}
        },
        "created_time": (datetime.now() - timedelta(days=30)).timestamp() * 1000,
        "creator_user_name": "ml-team@company.com"
    }
]

MOCK_JOB_RUNS = [
    {
        "run_id": 10001,
        "job_id": 101,
        "run_name": "Daily ETL Pipeline - 2024-01-15",
        "state": {"life_cycle_state": "TERMINATED", "result_state": "FAILED", "state_message": "Connection timeout to database"},
        "start_time": (datetime.now() - timedelta(hours=4)).timestamp() * 1000,
        "end_time": (datetime.now() - timedelta(hours=3, minutes=45)).timestamp() * 1000,
        "execution_duration": 900000,
        "cluster_spec": {"existing_cluster_id": "1234-567890-abc123"},
        "task": {"notebook_task": {"notebook_path": "/Production/ETL/main"}}
    },
    {
        "run_id": 10002,
        "job_id": 101,
        "run_name": "Daily ETL Pipeline - 2024-01-14",
        "state": {"life_cycle_state": "TERMINATED", "result_state": "SUCCESS"},
        "start_time": (datetime.now() - timedelta(days=1, hours=4)).timestamp() * 1000,
        "end_time": (datetime.now() - timedelta(days=1, hours=3)).timestamp() * 1000,
        "execution_duration": 3600000,
        "cluster_spec": {"existing_cluster_id": "1234-567890-abc123"},
        "task": {"notebook_task": {"notebook_path": "/Production/ETL/main"}}
    },
    {
        "run_id": 10003,
        "job_id": 102,
        "run_name": "Customer Analytics Refresh",
        "state": {"life_cycle_state": "RUNNING", "state_message": "Running"},
        "start_time": (datetime.now() - timedelta(minutes=30)).timestamp() * 1000,
        "cluster_spec": {"existing_cluster_id": "3456-789012-cde345"},
        "task": {"notebook_task": {"notebook_path": "/Analytics/CustomerRefresh"}}
    }
]

MOCK_SQL_RESULTS = {
    "error_metrics": {
        "columns": ["hour", "error_count", "error_rate", "top_error_type"],
        "data": [
            ["2024-01-15 10:00", 45, 0.05, "ConnectionTimeout"],
            ["2024-01-15 11:00", 120, 0.12, "ConnectionTimeout"],
            ["2024-01-15 12:00", 230, 0.23, "ConnectionTimeout"],
            ["2024-01-15 13:00", 180, 0.18, "ConnectionTimeout"],
            ["2024-01-15 14:00", 350, 0.35, "ConnectionTimeout"],  # Spike!
            ["2024-01-15 15:00", 280, 0.28, "ConnectionTimeout"]
        ]
    },
    "system_metrics": {
        "columns": ["metric_name", "current_value", "threshold", "status"],
        "data": [
            ["db_connection_pool_used", 95, 80, "CRITICAL"],
            ["db_query_latency_p99_ms", 2500, 1000, "WARNING"],
            ["db_active_connections", 450, 500, "WARNING"],
            ["etl_records_processed", 1250000, 0, "OK"],
            ["etl_failed_records", 15000, 1000, "CRITICAL"]
        ]
    },
    "incident_correlation": {
        "columns": ["timestamp", "event_type", "source", "details", "correlation_id"],
        "data": [
            ["2024-01-15 13:55:00", "DB_CONNECTION_POOL_EXHAUSTED", "production-db-cluster-01", "Max connections reached: 500/500", "CORR-001"],
            ["2024-01-15 13:56:00", "ETL_JOB_FAILED", "Daily ETL Pipeline", "ConnectionTimeout after 30s", "CORR-001"],
            ["2024-01-15 13:57:00", "ALERT_TRIGGERED", "PagerDuty", "P1 Alert: Database connectivity issue", "CORR-001"],
            ["2024-01-15 14:00:00", "INCIDENT_CREATED", "ServiceNow", "INC0010001: Database connection timeout", "CORR-001"]
        ]
    }
}

MOCK_TABLES = [
    {
        "catalog": "main",
        "schema": "production",
        "name": "customer_transactions",
        "table_type": "MANAGED",
        "data_source_format": "DELTA",
        "created_at": (datetime.now() - timedelta(days=365)).isoformat(),
        "updated_at": (datetime.now() - timedelta(hours=1)).isoformat(),
        "row_count": 125000000,
        "storage_size_bytes": 45000000000
    },
    {
        "catalog": "main",
        "schema": "production",
        "name": "system_events",
        "table_type": "MANAGED",
        "data_source_format": "DELTA",
        "created_at": (datetime.now() - timedelta(days=180)).isoformat(),
        "updated_at": (datetime.now() - timedelta(minutes=5)).isoformat(),
        "row_count": 500000000,
        "storage_size_bytes": 120000000000
    },
    {
        "catalog": "main",
        "schema": "analytics",
        "name": "error_metrics_hourly",
        "table_type": "MANAGED",
        "data_source_format": "DELTA",
        "created_at": (datetime.now() - timedelta(days=90)).isoformat(),
        "updated_at": (datetime.now() - timedelta(hours=1)).isoformat(),
        "row_count": 8760,
        "storage_size_bytes": 50000000
    }
]

# Tool definitions
TOOLS = [
    {
        "name": "list_clusters",
        "description": "List all Databricks clusters with their status and configuration",
        "inputSchema": {
            "type": "object",
            "properties": {
                "state_filter": {
                    "type": "string",
                    "description": "Filter by cluster state: RUNNING, TERMINATED, PENDING, etc."
                }
            },
            "required": []
        }
    },
    {
        "name": "get_cluster",
        "description": "Get detailed information about a specific cluster",
        "inputSchema": {
            "type": "object",
            "properties": {
                "cluster_id": {
                    "type": "string",
                    "description": "The cluster ID"
                }
            },
            "required": ["cluster_id"]
        }
    },
    {
        "name": "list_jobs",
        "description": "List all Databricks jobs",
        "inputSchema": {
            "type": "object",
            "properties": {
                "name_filter": {
                    "type": "string",
                    "description": "Filter jobs by name (partial match)"
                }
            },
            "required": []
        }
    },
    {
        "name": "get_job_runs",
        "description": "Get recent runs for a specific job",
        "inputSchema": {
            "type": "object",
            "properties": {
                "job_id": {
                    "type": "integer",
                    "description": "The job ID"
                },
                "limit": {
                    "type": "integer",
                    "description": "Maximum number of runs to return (default: 10)"
                },
                "include_failed_only": {
                    "type": "boolean",
                    "description": "Only return failed runs"
                }
            },
            "required": ["job_id"]
        }
    },
    {
        "name": "execute_sql",
        "description": "Execute a SQL query on Databricks SQL warehouse and return results",
        "inputSchema": {
            "type": "object",
            "properties": {
                "query": {
                    "type": "string",
                    "description": "SQL query to execute"
                },
                "warehouse_id": {
                    "type": "string",
                    "description": "SQL warehouse ID (optional, uses default if not specified)"
                },
                "limit": {
                    "type": "integer",
                    "description": "Maximum rows to return (default: 100)"
                }
            },
            "required": ["query"]
        }
    },
    {
        "name": "get_error_metrics",
        "description": "Get error metrics and rates from the analytics tables (commonly used for incident correlation)",
        "inputSchema": {
            "type": "object",
            "properties": {
                "time_range": {
                    "type": "string",
                    "description": "Time range: last_hour, last_6_hours, last_day (default: last_6_hours)"
                },
                "source_system": {
                    "type": "string",
                    "description": "Filter by source system (e.g., 'database', 'etl', 'api')"
                }
            },
            "required": []
        }
    },
    {
        "name": "get_system_health",
        "description": "Get current system health metrics including database connections, query latency, and processing status",
        "inputSchema": {
            "type": "object",
            "properties": {},
            "required": []
        }
    },
    {
        "name": "correlate_events",
        "description": "Find correlated events across systems for incident analysis",
        "inputSchema": {
            "type": "object",
            "properties": {
                "start_time": {
                    "type": "string",
                    "description": "Start time for correlation window (ISO format or relative like '-1h')"
                },
                "end_time": {
                    "type": "string",
                    "description": "End time for correlation window (ISO format or relative like 'now')"
                },
                "incident_id": {
                    "type": "string",
                    "description": "ServiceNow incident ID to correlate events for"
                }
            },
            "required": []
        }
    },
    {
        "name": "list_tables",
        "description": "List tables in Unity Catalog",
        "inputSchema": {
            "type": "object",
            "properties": {
                "catalog": {
                    "type": "string",
                    "description": "Catalog name (default: main)"
                },
                "schema": {
                    "type": "string",
                    "description": "Schema name filter"
                }
            },
            "required": []
        }
    },
    {
        "name": "get_table_info",
        "description": "Get detailed information about a table including schema, size, and recent updates",
        "inputSchema": {
            "type": "object",
            "properties": {
                "table_name": {
                    "type": "string",
                    "description": "Full table name (catalog.schema.table)"
                }
            },
            "required": ["table_name"]
        }
    }
]


class DatabricksClient:
    """Client for interacting with Databricks REST API"""

    def __init__(self, host: str, token: str):
        self.host = host.rstrip('/')
        self.headers = {
            "Authorization": f"Bearer {token}",
            "Content-Type": "application/json"
        }

    def _make_request(self, method: str, endpoint: str, params: Dict = None, data: Dict = None) -> Dict:
        url = f"{self.host}/api/2.0/{endpoint}"
        response = requests.request(
            method=method,
            url=url,
            headers=self.headers,
            params=params,
            json=data,
            timeout=60
        )
        response.raise_for_status()
        return response.json() if response.text else {}

    def list_clusters(self) -> List[Dict]:
        result = self._make_request("GET", "clusters/list")
        return result.get("clusters", [])

    def get_cluster(self, cluster_id: str) -> Dict:
        result = self._make_request("GET", "clusters/get", params={"cluster_id": cluster_id})
        return result

    def list_jobs(self) -> List[Dict]:
        result = self._make_request("GET", "jobs/list")
        return result.get("jobs", [])

    def get_job_runs(self, job_id: int, limit: int = 10) -> List[Dict]:
        result = self._make_request("GET", "jobs/runs/list", params={"job_id": job_id, "limit": limit})
        return result.get("runs", [])

    def execute_sql(self, warehouse_id: str, query: str) -> Dict:
        data = {
            "warehouse_id": warehouse_id,
            "statement": query,
            "wait_timeout": "30s"
        }
        result = self._make_request("POST", "sql/statements", data=data)
        return result


def execute_tool_mock(name: str, arguments: Dict) -> Dict:
    """Execute a tool using mock data"""

    if name == "list_clusters":
        results = MOCK_CLUSTERS.copy()
        if arguments.get("state_filter"):
            results = [c for c in results if c["state"] == arguments["state_filter"]]
        return {"mode": "mock", "count": len(results), "clusters": results}

    elif name == "get_cluster":
        cluster_id = arguments.get("cluster_id", "")
        for cluster in MOCK_CLUSTERS:
            if cluster["cluster_id"] == cluster_id:
                return {"mode": "mock", "cluster": cluster}
        return {"error": f"Cluster {cluster_id} not found"}

    elif name == "list_jobs":
        results = MOCK_JOBS.copy()
        if arguments.get("name_filter"):
            filter_str = arguments["name_filter"].lower()
            results = [j for j in results if filter_str in j["settings"]["name"].lower()]
        return {"mode": "mock", "count": len(results), "jobs": results}

    elif name == "get_job_runs":
        job_id = arguments.get("job_id")
        limit = arguments.get("limit", 10)
        include_failed_only = arguments.get("include_failed_only", False)

        results = [r for r in MOCK_JOB_RUNS if r["job_id"] == job_id]
        if include_failed_only:
            results = [r for r in results if r["state"].get("result_state") == "FAILED"]
        results = results[:limit]

        return {"mode": "mock", "job_id": job_id, "count": len(results), "runs": results}

    elif name == "execute_sql":
        query = arguments.get("query", "").lower()

        # Return appropriate mock results based on query content
        if "error" in query:
            data = MOCK_SQL_RESULTS["error_metrics"]
        elif "incident" in query or "correlat" in query:
            data = MOCK_SQL_RESULTS["incident_correlation"]
        else:
            data = MOCK_SQL_RESULTS["system_metrics"]

        return {
            "mode": "mock",
            "query": arguments.get("query"),
            "result": {
                "columns": data["columns"],
                "data": data["data"],
                "row_count": len(data["data"])
            }
        }

    elif name == "get_error_metrics":
        time_range = arguments.get("time_range", "last_6_hours")
        data = MOCK_SQL_RESULTS["error_metrics"]

        return {
            "mode": "mock",
            "time_range": time_range,
            "source_system": arguments.get("source_system", "all"),
            "summary": {
                "total_errors": sum(row[1] for row in data["data"]),
                "peak_error_rate": max(row[2] for row in data["data"]),
                "peak_hour": [row for row in data["data"] if row[2] == max(r[2] for r in data["data"])][0][0],
                "dominant_error_type": "ConnectionTimeout"
            },
            "hourly_data": [
                {"hour": row[0], "error_count": row[1], "error_rate": row[2], "top_error": row[3]}
                for row in data["data"]
            ]
        }

    elif name == "get_system_health":
        data = MOCK_SQL_RESULTS["system_metrics"]
        metrics = {row[0]: {"value": row[1], "threshold": row[2], "status": row[3]} for row in data["data"]}

        critical_count = sum(1 for row in data["data"] if row[3] == "CRITICAL")
        warning_count = sum(1 for row in data["data"] if row[3] == "WARNING")

        return {
            "mode": "mock",
            "overall_status": "CRITICAL" if critical_count > 0 else ("WARNING" if warning_count > 0 else "OK"),
            "critical_issues": critical_count,
            "warnings": warning_count,
            "metrics": metrics
        }

    elif name == "correlate_events":
        data = MOCK_SQL_RESULTS["incident_correlation"]

        return {
            "mode": "mock",
            "correlation_window": {
                "start": arguments.get("start_time", (datetime.now() - timedelta(hours=1)).isoformat()),
                "end": arguments.get("end_time", datetime.now().isoformat())
            },
            "incident_id": arguments.get("incident_id", "INC0010001"),
            "correlated_events": [
                {
                    "timestamp": row[0],
                    "event_type": row[1],
                    "source": row[2],
                    "details": row[3],
                    "correlation_id": row[4]
                }
                for row in data["data"]
            ],
            "root_cause_analysis": {
                "probable_root_cause": "Database connection pool exhaustion",
                "affected_systems": ["production-db-cluster-01", "Daily ETL Pipeline"],
                "impact_duration_minutes": 35,
                "recommendation": "Increase connection pool size and implement connection pooling optimization"
            }
        }

    elif name == "list_tables":
        results = MOCK_TABLES.copy()
        if arguments.get("catalog"):
            results = [t for t in results if t["catalog"] == arguments["catalog"]]
        if arguments.get("schema"):
            results = [t for t in results if t["schema"] == arguments["schema"]]

        return {"mode": "mock", "count": len(results), "tables": results}

    elif name == "get_table_info":
        table_name = arguments.get("table_name", "")
        parts = table_name.split(".")
        if len(parts) == 3:
            catalog, schema, name = parts
            for table in MOCK_TABLES:
                if table["catalog"] == catalog and table["schema"] == schema and table["name"] == name:
                    return {"mode": "mock", "table": table}
        return {"error": f"Table {table_name} not found"}

    else:
        return {"error": f"Unknown tool: {name}"}


# Configuration for real mode analytics queries
ANALYTICS_CATALOG = os.environ.get("DATABRICKS_CATALOG", "main")
ANALYTICS_SCHEMA = os.environ.get("DATABRICKS_SCHEMA", "analytics")
SQL_WAREHOUSE_ID = os.environ.get("DATABRICKS_WAREHOUSE_ID", "")


def execute_tool_real(name: str, arguments: Dict, client: DatabricksClient) -> Dict:
    """Execute a tool using real Databricks API"""

    try:
        if name == "list_clusters":
            clusters = client.list_clusters()
            if arguments.get("state_filter"):
                clusters = [c for c in clusters if c.get("state") == arguments["state_filter"]]
            return {"mode": "real", "count": len(clusters), "clusters": clusters}

        elif name == "get_cluster":
            cluster = client.get_cluster(arguments["cluster_id"])
            return {"mode": "real", "cluster": cluster}

        elif name == "list_jobs":
            jobs = client.list_jobs()
            if arguments.get("name_filter"):
                filter_str = arguments["name_filter"].lower()
                jobs = [j for j in jobs if filter_str in j.get("settings", {}).get("name", "").lower()]
            return {"mode": "real", "count": len(jobs), "jobs": jobs}

        elif name == "get_job_runs":
            runs = client.get_job_runs(arguments["job_id"], arguments.get("limit", 10))
            if arguments.get("include_failed_only"):
                runs = [r for r in runs if r.get("state", {}).get("result_state") == "FAILED"]
            return {"mode": "real", "job_id": arguments["job_id"], "count": len(runs), "runs": runs}

        elif name == "execute_sql":
            warehouse_id = arguments.get("warehouse_id") or SQL_WAREHOUSE_ID
            if not warehouse_id:
                return {"error": "warehouse_id is required. Set DATABRICKS_WAREHOUSE_ID or pass warehouse_id parameter."}
            result = client.execute_sql(warehouse_id, arguments["query"])
            return {"mode": "real", "query": arguments["query"], "result": result}

        elif name == "get_error_metrics":
            warehouse_id = SQL_WAREHOUSE_ID
            if not warehouse_id:
                return execute_tool_mock(name, arguments)

            time_range = arguments.get("time_range", "last_6_hours")
            hours_map = {"last_hour": 1, "last_6_hours": 6, "last_day": 24}
            hours = hours_map.get(time_range, 6)

            query = f"""
                SELECT hour, error_count, error_rate, top_error_type, source_system
                FROM {ANALYTICS_CATALOG}.{ANALYTICS_SCHEMA}.error_metrics_hourly
                WHERE timestamp >= current_timestamp() - INTERVAL {hours} HOURS
                ORDER BY timestamp DESC
            """
            result = client.execute_sql(warehouse_id, query)

            # Parse SQL result - values come back as strings from SQL API
            data = result.get("result", {}).get("data_array", [])
            if data:
                total_errors = sum(int(row[1]) for row in data if row[1])
                peak_rate = max((float(row[2]) for row in data if row[2]), default=0)
                return {
                    "mode": "real",
                    "time_range": time_range,
                    "summary": {
                        "total_errors": total_errors,
                        "peak_error_rate": peak_rate,
                        "data_points": len(data)
                    },
                    "hourly_data": [
                        {"hour": row[0], "error_count": row[1], "error_rate": row[2], "top_error": row[3], "source": row[4]}
                        for row in data
                    ]
                }
            return {"mode": "real", "time_range": time_range, "hourly_data": [], "message": "No data found"}

        elif name == "get_system_health":
            warehouse_id = SQL_WAREHOUSE_ID
            if not warehouse_id:
                return execute_tool_mock(name, arguments)

            query = f"""
                SELECT metric_name, current_value, threshold, status, component
                FROM {ANALYTICS_CATALOG}.{ANALYTICS_SCHEMA}.system_health
                WHERE timestamp >= current_timestamp() - INTERVAL 1 HOUR
                ORDER BY timestamp DESC
            """
            result = client.execute_sql(warehouse_id, query)

            data = result.get("result", {}).get("data_array", [])
            metrics = {}
            critical_count = 0
            warning_count = 0

            for row in data:
                metric_name, value, threshold, status, component = row
                metrics[metric_name] = {"value": value, "threshold": threshold, "status": status, "component": component}
                if status == "CRITICAL":
                    critical_count += 1
                elif status == "WARNING":
                    warning_count += 1

            return {
                "mode": "real",
                "overall_status": "CRITICAL" if critical_count > 0 else ("WARNING" if warning_count > 0 else "OK"),
                "critical_issues": critical_count,
                "warnings": warning_count,
                "metrics": metrics
            }

        elif name == "correlate_events":
            warehouse_id = SQL_WAREHOUSE_ID
            if not warehouse_id:
                return execute_tool_mock(name, arguments)

            incident_id = arguments.get("incident_id", "")
            hours = 2  # Default correlation window

            # Build query - filter by incident_id if provided
            if incident_id:
                query = f"""
                    SELECT event_id, timestamp, event_type, source, details, severity, correlation_id
                    FROM {ANALYTICS_CATALOG}.{ANALYTICS_SCHEMA}.system_events
                    WHERE details LIKE '%{incident_id}%'
                       OR correlation_id IN (
                           SELECT DISTINCT correlation_id
                           FROM {ANALYTICS_CATALOG}.{ANALYTICS_SCHEMA}.system_events
                           WHERE details LIKE '%{incident_id}%'
                       )
                    ORDER BY timestamp ASC
                """
            else:
                query = f"""
                    SELECT event_id, timestamp, event_type, source, details, severity, correlation_id
                    FROM {ANALYTICS_CATALOG}.{ANALYTICS_SCHEMA}.system_events
                    WHERE timestamp >= current_timestamp() - INTERVAL {hours} HOURS
                    ORDER BY timestamp ASC
                """

            result = client.execute_sql(warehouse_id, query)
            data = result.get("result", {}).get("data_array", [])

            return {
                "mode": "real",
                "incident_id": incident_id or "all",
                "event_count": len(data),
                "correlated_events": [
                    {
                        "event_id": row[0],
                        "timestamp": str(row[1]),
                        "event_type": row[2],
                        "source": row[3],
                        "details": row[4],
                        "severity": row[5],
                        "correlation_id": row[6]
                    }
                    for row in data
                ]
            }

        elif name == "list_tables":
            warehouse_id = SQL_WAREHOUSE_ID
            if not warehouse_id:
                return execute_tool_mock(name, arguments)

            catalog = arguments.get("catalog", ANALYTICS_CATALOG)
            schema = arguments.get("schema", "")

            if schema:
                query = f"SHOW TABLES IN {catalog}.{schema}"
            else:
                query = f"SHOW TABLES IN {catalog}"

            result = client.execute_sql(warehouse_id, query)
            data = result.get("result", {}).get("data_array", [])

            return {
                "mode": "real",
                "catalog": catalog,
                "schema": schema or "all",
                "tables": [{"database": row[0], "tableName": row[1], "isTemporary": row[2]} for row in data]
            }

        elif name == "get_table_info":
            warehouse_id = SQL_WAREHOUSE_ID
            if not warehouse_id:
                return execute_tool_mock(name, arguments)

            table_name = arguments.get("table_name", "")
            query = f"DESCRIBE TABLE EXTENDED {table_name}"
            result = client.execute_sql(warehouse_id, query)

            return {
                "mode": "real",
                "table_name": table_name,
                "schema": result.get("result", {}).get("data_array", [])
            }

        else:
            return {"error": f"Unknown tool: {name}"}

    except requests.exceptions.RequestException as e:
        return {"error": f"Databricks API error: {str(e)}"}


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
                    "name": "databricks-mcp-server",
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
            client = DatabricksClient(DATABRICKS_HOST, DATABRICKS_TOKEN)
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
        "databricks_host": DATABRICKS_HOST if not MOCK_MODE else "N/A (mock mode)"
    })


if __name__ == '__main__':
    print("=" * 70)
    print("  Databricks MCP Server")
    print("=" * 70)
    print()
    print(f"  Mode: {'MOCK (demo data)' if MOCK_MODE else 'REAL (Databricks API)'}")
    if not MOCK_MODE:
        print(f"  Host: {DATABRICKS_HOST}")
    print()
    print("  Available tools:")
    for tool in TOOLS:
        print(f"    - {tool['name']}: {tool['description'][:55]}...")
    print()
    print(f"  Server running at: http://{HOST}:{PORT}/mcp")
    print()
    print("  Environment variables:")
    print("    DATABRICKS_MOCK_MODE=true|false (default: true)")
    print("    DATABRICKS_HOST=https://your-workspace.cloud.databricks.com")
    print("    DATABRICKS_TOKEN=dapi-xxxxx")
    print()
    print("  Press Ctrl+C to stop")
    print()

    app.run(host=HOST, port=PORT, debug=False)

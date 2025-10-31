# SUSE AI Virtual MCP Service - Examples

This document provides practical examples of using the SUSE AI Virtual MCP Service to create MCP servers from various sources including REST APIs, databases, and complex enterprise systems.

## Table of Contents

- [Basic API Examples](#basic-api-examples)
- [Database Integration Examples](#database-integration-examples)
- [Enterprise Integration Examples](#enterprise-integration-examples)
- [Advanced Configuration Examples](#advanced-configuration-examples)
- [Real-World Use Cases](#real-world-use-cases)

## Basic API Examples

### Pet Store API Integration

Convert the classic Pet Store API into an MCP server that LLMs can use to manage pets.

#### Configuration
```json
{
  "name": "petstore-api",
  "type": "openapi",
  "source": {
    "url": "https://petstore.swagger.io/v2/swagger.json",
    "format": "openapi-v2"
  },
  "generation": {
    "tools": [
      {
        "path": "/pets",
        "methods": ["GET", "POST"],
        "tool_name": "manage_pets",
        "description": "List all pets or add a new pet"
      },
      {
        "path": "/pets/{id}",
        "methods": ["GET", "PUT", "DELETE"],
        "tool_name": "manage_pet",
        "description": "Get, update, or delete a specific pet"
      }
    ],
    "resources": [
      {
        "path": "/pets/{id}",
        "resource_name": "pet_details",
        "description": "Detailed information about a specific pet"
      }
    ]
  },
  "authentication": {
    "type": "bearer",
    "token_env": "PETSTORE_API_TOKEN"
  }
}
```

#### Generated MCP Tools
Once generated, the MCP server provides these tools to LLMs:

- `manage_pets`: List pets with optional filtering, or create new pets
- `manage_pet`: Get, update, or delete individual pets by ID
- `pet_details` resource: Real-time access to pet information

#### Usage with AI Agent
```bash
# Create an agent that can manage pets
curl -X POST http://localhost:8910/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "pet-manager",
    "supervisor": {
      "provider": "openai",
      "model": "gpt-4",
      "api": "sk-your_openai_key"
    },
    "worker": {
      "provider": "ollama",
      "model": "llama3.2:latest"
    },
    "mcp_servers": ["virtualmcp-petstore-api-12345"]
  }'

# Ask the AI to manage pets
curl -X POST http://localhost:8910/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "pet-manager",
    "messages": [
      {
        "role": "user",
        "content": "Add a new golden retriever named Max, then show me all pets available"
      }
    ]
  }'
```

### GitHub API Integration

Create an MCP server for GitHub API operations.

#### Configuration
```json
{
  "name": "github-api",
  "type": "openapi",
  "source": {
    "url": "https://api.github.com/swagger.json",
    "format": "openapi-v3"
  },
  "generation": {
    "tools": [
      {
        "path": "/repos/{owner}/{repo}/issues",
        "methods": ["GET", "POST"],
        "tool_name": "manage_issues",
        "description": "List repository issues or create new issues"
      },
      {
        "path": "/repos/{owner}/{repo}/pulls",
        "methods": ["GET", "POST"],
        "tool_name": "manage_pull_requests",
        "description": "List or create pull requests"
      },
      {
        "path": "/repos/{owner}/{repo}/contents/{path}",
        "methods": ["GET", "PUT"],
        "tool_name": "manage_files",
        "description": "Read or update repository files"
      }
    ],
    "resources": [
      {
        "path": "/repos/{owner}/{repo}/readme",
        "resource_name": "repository_readme",
        "description": "Repository README content"
      }
    ]
  },
  "authentication": {
    "type": "bearer",
    "token_env": "GITHUB_TOKEN"
  }
}
```

## Database Integration Examples

### E-commerce Analytics Database

Generate MCP tools for querying an e-commerce database.

#### Configuration
```json
{
  "name": "ecommerce-analytics",
  "type": "database",
  "source": {
    "connection": {
      "type": "postgresql",
      "host_env": "DB_HOST",
      "port_env": "DB_PORT",
      "database_env": "DB_NAME",
      "username_env": "DB_USER",
      "password_env": "DB_PASSWORD"
    },
    "ssl_mode": "require"
  },
  "generation": {
    "tables": [
      {
        "name": "orders",
        "tools": ["select", "analytics"],
        "description": "Customer order data"
      },
      {
        "name": "products",
        "tools": ["select", "count"],
        "description": "Product catalog information"
      },
      {
        "name": "customers",
        "tools": ["select"],
        "description": "Customer information (read-only)"
      }
    ],
    "queries": [
      {
        "name": "monthly_sales_report",
        "sql": "SELECT DATE_TRUNC('month', order_date) as month, COUNT(*) as orders, SUM(total_amount) as revenue FROM orders WHERE order_date >= $1 GROUP BY month ORDER BY month DESC",
        "parameters": [
          {
            "name": "start_date",
            "type": "date",
            "description": "Start date for the report"
          }
        ],
        "description": "Generate monthly sales report"
      },
      {
        "name": "top_products",
        "sql": "SELECT p.name, SUM(oi.quantity) as total_sold, SUM(oi.quantity * oi.unit_price) as revenue FROM products p JOIN order_items oi ON p.id = oi.product_id JOIN orders o ON oi.order_id = o.id WHERE o.order_date >= $1 GROUP BY p.id, p.name ORDER BY revenue DESC LIMIT $2",
        "parameters": [
          {
            "name": "start_date",
            "type": "date",
            "description": "Start date for analysis"
          },
          {
            "name": "limit",
            "type": "integer",
            "description": "Number of top products to return",
            "default": 10
          }
        ],
        "description": "Find top-selling products by revenue"
      }
    ]
  },
  "security": {
    "row_level_security": true,
    "audit_logging": true
  }
}
```

#### Generated Tools
- `select_orders`: Query order data with filtering
- `analytics_orders`: Generate analytics on order data
- `select_products`: Query product information
- `count_products`: Count products matching criteria
- `select_customers`: Query customer data (read-only)
- `monthly_sales_report`: Custom sales reporting tool
- `top_products`: Top products analysis tool

### HR Database Integration

Create MCP tools for HR data analysis.

#### Configuration
```json
{
  "name": "hr-database",
  "type": "database",
  "source": {
    "connection": {
      "type": "mysql",
      "host": "hr-db.company.com",
      "port": 3306,
      "database": "human_resources",
      "username_env": "HR_DB_USER",
      "password_env": "HR_DB_PASSWORD"
    }
  },
  "generation": {
    "tables": [
      {
        "name": "employees",
        "tools": ["select"],
        "description": "Employee information"
      },
      {
        "name": "departments",
        "tools": ["select"],
        "description": "Department information"
      }
    ],
    "queries": [
      {
        "name": "department_headcount",
        "sql": "SELECT d.name as department, COUNT(e.id) as headcount FROM departments d LEFT JOIN employees e ON d.id = e.department_id WHERE e.status = 'active' GROUP BY d.id, d.name ORDER BY headcount DESC",
        "description": "Get headcount by department"
      },
      {
        "name": "salary_analysis",
        "sql": "SELECT d.name as department, AVG(e.salary) as avg_salary, MIN(e.salary) as min_salary, MAX(e.salary) as max_salary FROM departments d JOIN employees e ON d.id = e.department_id WHERE e.status = 'active' GROUP BY d.id, d.name",
        "description": "Salary analysis by department"
      }
    ]
  },
  "security": {
    "data_masking": {
      "employees": {
        "ssn": "mask",
        "salary": "aggregate_only"
      }
    }
  }
}
```

## Enterprise Integration Examples

### SAP Integration via REST API

Convert SAP OData services to MCP tools.

#### Configuration
```json
{
  "name": "sap-odata",
  "type": "openapi",
  "source": {
    "url": "https://sap-system.company.com/sap/opu/odata/sap/API_SALES_ORDER_SRV/$metadata",
    "format": "odata"
  },
  "generation": {
    "tools": [
      {
        "path": "/A_SalesOrder",
        "methods": ["GET", "POST"],
        "tool_name": "manage_sales_orders",
        "description": "Query and create sales orders"
      },
      {
        "path": "/A_SalesOrder('{SalesOrder}')",
        "methods": ["GET", "PATCH"],
        "tool_name": "manage_sales_order",
        "description": "Get or update specific sales order"
      }
    ],
    "resources": [
      {
        "path": "/A_SalesOrder('{SalesOrder}')",
        "resource_name": "sales_order_details",
        "description": "Detailed sales order information"
      }
    ]
  },
  "authentication": {
    "type": "basic",
    "username_env": "SAP_USER",
    "password_env": "SAP_PASSWORD"
  }
}
```

### Salesforce REST API Integration

Create MCP tools for Salesforce operations.

#### Configuration
```json
{
  "name": "salesforce-api",
  "type": "openapi",
  "source": {
    "url": "https://yourinstance.salesforce.com/services/data/v55.0/sobjects",
    "format": "custom"
  },
  "generation": {
    "tools": [
      {
        "path": "/Account",
        "methods": ["GET", "POST"],
        "tool_name": "manage_accounts",
        "description": "Query and create Salesforce accounts"
      },
      {
        "path": "/Contact",
        "methods": ["GET", "POST"],
        "tool_name": "manage_contacts",
        "description": "Query and create Salesforce contacts"
      },
      {
        "path": "/Opportunity",
        "methods": ["GET", "POST"],
        "tool_name": "manage_opportunities",
        "description": "Query and create Salesforce opportunities"
      }
    ],
    "resources": [
      {
        "path": "/Account/{id}",
        "resource_name": "account_details",
        "description": "Detailed account information"
      }
    ]
  },
  "authentication": {
    "type": "oauth2",
    "token_url": "https://login.salesforce.com/services/oauth2/token",
    "client_id_env": "SF_CLIENT_ID",
    "client_secret_env": "SF_CLIENT_SECRET",
    "scopes": ["api", "refresh_token"]
  }
}
```

## Advanced Configuration Examples

### Multi-Source Aggregation

Combine multiple APIs into a single unified MCP server.

#### Configuration
```json
{
  "name": "unified-customer-data",
  "type": "composite",
  "sources": [
    {
      "name": "crm_api",
      "type": "openapi",
      "url": "https://crm.company.com/api/v1/swagger.json",
      "tools": [
        {
          "path": "/customers",
          "tool_name": "crm_customers"
        }
      ]
    },
    {
      "name": "billing_db",
      "type": "database",
      "connection": {
        "type": "postgresql",
        "host": "billing-db.company.com",
        "database": "billing"
      },
      "tables": [
        {
          "name": "invoices",
          "tools": ["select"]
        }
      ]
    }
  ],
  "generation": {
    "unified_tools": [
      {
        "name": "customer_360_view",
        "description": "Get complete customer information from CRM and billing systems",
        "implementation": "aggregate_crm_and_billing_data"
      }
    ]
  }
}
```

### Custom Tool with Business Logic

Create tools with custom business logic beyond simple CRUD operations.

#### Configuration
```json
{
  "name": "advanced-analytics-api",
  "type": "openapi",
  "source": {
    "url": "https://analytics.company.com/api/v2/swagger.json"
  },
  "generation": {
    "custom_tools": [
      {
        "name": "predict_customer_churn",
        "description": "Predict customer churn probability using ML model",
        "endpoint": "/ml/predict/churn",
        "method": "POST",
        "parameters": [
          {
            "name": "customer_id",
            "type": "string",
            "required": true,
            "description": "Customer identifier"
          },
          {
            "name": "features",
            "type": "object",
            "description": "Additional customer features for prediction"
          }
        ],
        "response_mapping": {
          "churn_probability": "result.probability",
          "risk_level": "result.risk_category"
        }
      },
      {
        "name": "generate_business_report",
        "description": "Generate comprehensive business report with charts",
        "endpoint": "/reports/generate",
        "method": "POST",
        "parameters": [
          {
            "name": "report_type",
            "type": "string",
            "enum": ["sales", "financial", "operational"],
            "required": true
          },
          {
            "name": "date_range",
            "type": "object",
            "properties": {
              "start_date": {"type": "string", "format": "date"},
              "end_date": {"type": "string", "format": "date"}
            },
            "required": true
          },
          {
            "name": "include_charts",
            "type": "boolean",
            "default": true
          }
        ]
      }
    ]
  }
}
```

### Real-Time Data Streaming

Configure MCP servers for real-time data updates.

#### Configuration
```json
{
  "name": "realtime-inventory",
  "type": "openapi",
  "source": {
    "url": "https://inventory.company.com/api/v1/swagger.json"
  },
  "generation": {
    "tools": [
      {
        "path": "/inventory/items",
        "methods": ["GET"],
        "tool_name": "get_inventory_levels"
      }
    ],
    "resources": [
      {
        "path": "/inventory/items/{sku}",
        "resource_name": "inventory_item",
        "realtime": true,
        "update_interval": "30s",
        "description": "Real-time inventory levels for specific SKU"
      },
      {
        "path": "/inventory/alerts",
        "resource_name": "inventory_alerts",
        "subscription": true,
        "description": "Real-time inventory alerts and notifications"
      }
    ]
  },
  "realtime": {
    "websocket_url": "wss://inventory.company.com/ws",
    "reconnect_policy": {
      "max_attempts": 5,
      "backoff_multiplier": 2,
      "initial_delay": "1s"
    }
  }
}
```

## Real-World Use Cases

### AI-Powered Customer Support

Combine CRM API and knowledge base to create intelligent customer support.

```json
{
  "name": "customer-support-suite",
  "type": "composite",
  "sources": [
    {
      "name": "zendesk_api",
      "type": "openapi",
      "url": "https://company.zendesk.com/api/v2/swagger.json",
      "tools": [
        {
          "path": "/tickets",
          "tool_name": "manage_support_tickets"
        }
      ]
    },
    {
      "name": "knowledge_base",
      "type": "database",
      "connection": {
        "type": "elasticsearch",
        "host": "kb-search.company.com"
      },
      "queries": [
        {
          "name": "search_kb",
          "description": "Search knowledge base for solutions"
        }
      ]
    }
  ],
  "generation": {
    "ai_enhanced_tools": [
      {
        "name": "resolve_customer_issue",
        "description": "Analyze customer issue and suggest resolution using KB and ticket history",
        "ai_prompt": "Analyze the customer issue, search relevant knowledge base articles, and suggest the best resolution approach."
      }
    ]
  }
}
```

### Financial Reporting and Analysis

Create comprehensive financial analysis tools from multiple data sources.

```json
{
  "name": "financial-intelligence",
  "type": "composite",
  "sources": [
    {
      "name": "erp_system",
      "type": "database",
      "connection": {
        "type": "oracle",
        "host": "erp-db.company.com"
      },
      "tables": ["general_ledger", "accounts_payable", "accounts_receivable"]
    },
    {
      "name": "market_data_api",
      "type": "openapi",
      "url": "https://marketdata.provider.com/api/v1/swagger.json"
    }
  ],
  "generation": {
    "analytics_tools": [
      {
        "name": "cash_flow_analysis",
        "description": "Analyze cash flow patterns and predict future trends"
      },
      {
        "name": "budget_vs_actual",
        "description": "Compare budgeted vs actual financial performance"
      },
      {
        "name": "financial_ratios",
        "description": "Calculate key financial ratios and metrics"
      }
    ],
    "reporting_tools": [
      {
        "name": "generate_financial_report",
        "description": "Generate comprehensive financial reports with charts and insights"
      }
    ]
  }
}
```

### DevOps and Infrastructure Management

Integrate CI/CD pipelines, monitoring, and infrastructure APIs.

```json
{
  "name": "devops-control-center",
  "type": "composite",
  "sources": [
    {
      "name": "jenkins_api",
      "type": "openapi",
      "url": "https://ci.company.com/api/swagger.json",
      "tools": [
        {
          "path": "/job/{jobName}/build",
          "tool_name": "trigger_build"
        }
      ]
    },
    {
      "name": "kubernetes_api",
      "type": "openapi",
      "url": "https://k8s-api.company.com/openapi/v2",
      "tools": [
        {
          "path": "/api/v1/pods",
          "tool_name": "manage_pods"
        }
      ]
    },
    {
      "name": "monitoring_db",
      "type": "database",
      "connection": {
        "type": "influxdb",
        "host": "monitoring.company.com"
      }
    }
  ],
  "generation": {
    "automation_tools": [
      {
        "name": "deploy_application",
        "description": "Trigger full CI/CD pipeline deployment"
      },
      {
        "name": "scale_service",
        "description": "Automatically scale services based on metrics"
      },
      {
        "name": "incident_response",
        "description": "Automated incident detection and response"
      }
    ]
  }
}
```

These examples demonstrate the versatility of the Virtual MCP service in bridging existing enterprise systems with modern AI capabilities, enabling organizations to leverage their current investments while gaining powerful new AI-driven functionalities.
import yaml
import requests

# Read current file
with open('/Users/alessandrofesta/Documents/innovation/suse-ai-up/config/comprehensive_mcp_servers.yaml', 'r') as f:
    current = yaml.safe_load(f) or []

existing_names = {s['name'] for s in current}

# List of server names from the registry
server_names = [
    "SQLite", "airtable-mcp-server", "ais-fleet", "aks", "amazon-bedrock-agentcore", "amazon-kendra-index", "amazon-neptune", "amazon-qbusiness-anonymous", "apify-mcp-server", "apify", "apollo-mcp-server", "arxiv-mcp-server", "asana", "ast-grep", "astra-db", "astro-docs", "atlan", "atlas-docs", "atlassian", "audiense-insights", "audioscrape", "aws-api", "aws-appsync", "aws-bedrock-custom-model-import", "aws-bedrock-data-automation", "aws-cdk-mcp-server", "aws-core-mcp-server", "aws-dataprocessing", "aws-diagram", "aws-documentation", "aws-healthomics", "aws-iot-sitewise", "aws-kb-retrieval-server", "aws-location", "aws-msk", "aws-pricing", "aws-terraform", "awslabs-billing-cost-management", "awslabs-ccapi", "awslabs-cfn", "awslabs-cloudtrail", "awslabs-cloudwatch-appsignals", "awslabs-cloudwatch", "awslabs-cost-explorer", "awslabs-dynamodb", "awslabs-elasticache", "awslabs-iam", "awslabs-memcached", "awslabs-nova-canvas", "awslabs-redshift", "awslabs-s3-tables", "awslabs-timestream-for-influxdb", "awslabs-valkey", "azure", "beagle-security", "bitrefill", "box", "brave", "browserbase", "buildkite", "camunda", "carbon-voice", "cdata-connectcloud", "charmhealth-mcp-server", "chroma", "circleci", "clickhouse", "close", "cloud-run-mcp", "cloudflare-ai-gateway", "cloudflare-audit-logs", "cloudflare-autorag", "cloudflare-browser-rendering", "cloudflare-container", "cloudflare-digital-experience-monitoring", "cloudflare-dns-analytics", "cloudflare-docs", "cloudflare-graphql", "cloudflare-logpush", "cloudflare-observability", "cloudflare-one-casb", "cloudflare-radar", "cloudflare-workers-bindings", "cloudflare-workers-builds", "cockroachdb", "context7", "couchbase", "curl", "cylera-mcp-server", "cyreslab-ai-shodan", "dappier-remote", "dappier", "dart", "database-server", "databutton", "deepwiki", "descope", "desktop-commander", "devhub-cms", "dialer", "docker", "dockerhub", "dodo-payments", "dotnet-types-explorer", "dreamfactory-mcp", "duckduckgo", "dynatrace-mcp-server", "e2b", "edubase", "effect-mcp", "elasticsearch", "elevenlabs", "everart", "exa", "explorium", "fetch", "ffmpeg", "fibery", "filesystem", "find-a-domain", "firecrawl", "firefly", "firewalla-mcp-server", "flexprice", "gemini-api-docs", "genai-toolbox", "git", "github-chat", "github-official", "github", "gitlab", "gitmcp", "glif", "globalping", "gmail-mcp", "google-flights", "google-maps-comprehensive", "google-maps", "grafana", "grafbase", "gyazo", "hackle", "handwriting-ocr", "hdx", "heroku", "hostinger-mcp-server", "hoverfly-mcp-server", "hubspot", "hugging-face", "hummingbot-mcp", "husqvarna-automower", "hyperbrowser", "hyperspell", "iaptic", "inspektor-gadget", "instant", "invideo", "javadocs", "jetbrains", "kafka-schema-reg-mcp", "kagisearch", "keboola-mcp", "kgrag-mcp-server", "kong", "kubectl-mcp-server", "kubernetes", "lara", "line", "linear", "linkedin-mcp-server", "llmtxt", "maestro-mcp-server", "manifold", "mapbox-devkit", "mapbox", "markdownify", "markitdown", "maven-tools-mcp", "mcp-api-gateway", "mcp-code-interpreter", "mcp-discord", "mcp-github-pr-issue-analyser", "mcp-hackernews", "mcp-python-refactoring", "mcp-reddit", "memory", "mercado-libre", "mercado-pago", "metabase", "microsoft-learn", "minecraft-wiki", "monday", "mongodb", "morningstar-mcp-server", "multiversx-mx", "n8n", "nasdaq-data-link", "needle-mcp", "needle", "neo4j-cloud-aura-api", "neo4j-cypher", "neo4j-data-modeling", "neo4j-memory", "neon", "next-devtools-mcp", "node-code-sandbox", "notion-remote", "notion", "novita", "npm-sentinel", "obsidian", "octagon", "okta-mcp-fctr", "omi", "onlyoffice-docspace", "openapi-schema", "openapi", "openbnb-airbnb", "openmesh", "openweather", "openzeppelin-cairo", "openzeppelin-solidity", "openzeppelin-stellar", "openzeppelin-stylus", "opik", "opine-mcp-server", "oracle", "osp_marketing_tools", "oxylabs", "paper-search", "paypal", "perplexity-ask", "pia", "pica", "pinecone", "playwright-mcp-server", "playwright", "pluggedin-mcp-proxy", "polar-signals", "pomodash", "postgres", "postman", "pref-editor", "prisma-postgres", "prometheus", "pulumi-remote", "pulumi", "puppeteer", "quantconnect", "ramparts", "razorpay", "redis-cloud", "redis", "ref", "remote-mcp", "render", "resend", "risken", "root", "ros2", "rube", "rust-mcp-filesystem", "schemacrawler-ai", "schogini-mcp-image-border", "scorecard", "scrapegraph", "scrapezy", "securenote-link-mcp-server", "semgrep", "sentry-remote", "sentry", "sequa", "sequentialthinking", "short-io", "simplechecklist", "singlestore", "slack", "smartbear", "sonarqube", "sqlite-mcp-server", "stackgen", "stackhawk", "stripe-remote", "stripe", "supadata", "suzieq", "task-orchestrator", "tavily", "teamwork", "telnyx", "tembo", "temporal", "terraform", "testkube", "text-to-graphql", "thingsboard", "tigris", "time", "triplewhale", "uberall", "unreal-engine-mcp-server", "vectra-ai-rux-mcp-server", "veyrax", "victorialogs", "victoriametrics", "victoriatraces", "vizro", "vuln-nist-mcp-server", "wayfound", "waystation", "webflow-remote", "webflow", "wikipedia-mcp", "wix", "wolfram-alpha", "youtube_transcript", "zen", "zerodha-kite"
]

new_servers = []

for name in server_names:
    if name in existing_names:
        continue
    url = f"https://raw.githubusercontent.com/docker/mcp-registry/main/servers/{name}/server.yaml"
    try:
        response = requests.get(url)
        response.raise_for_status()
        data = yaml.safe_load(response.text)
        if 'image' not in data:
            continue
        # Transform
        server = {
            'name': data['name'],
            'image': data['image'],
            'type': data.get('type', 'server'),
            'meta': data.get('meta', {}),
            'about': data.get('about', {}),
            'source': {
                'project': data.get('source', {}).get('project'),
                'commit': data.get('source', {}).get('commit')
            }
        }
        # Omit config for now
        new_servers.append(server)
    except Exception as e:
        print(f"Failed to fetch {name}: {e}")

# Combine
all_servers = current + new_servers

# Write back
with open('/Users/alessandrofesta/Documents/innovation/suse-ai-up/config/comprehensive_mcp_servers.yaml', 'w') as f:
    yaml.dump(all_servers, f, default_flow_style=False, sort_keys=False)

print(f"Added {len(new_servers)} new servers")

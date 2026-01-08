package proxy

import (
	"testing"
)

func TestParseDockerCommand(t *testing.T) {
	sm := &SidecarManager{}

	tests := []struct {
		name        string
		command     string
		wantImage   string
		wantEnvVars map[string]string
		wantPort    int
		wantErr     bool
	}{
		{
			name:      "valid uyuni command",
			command:   "docker run -it --rm -e UYUNI_SERVER=http://dummy.domain.com -e UYUNI_USER=admin -e UYUNI_PASS=admin -e UYUNI_MCP_TRANSPORT=http -e UYUNI_MCP_HOST=0.0.0.0 ghcr.io/uyuni-project/mcp-server-uyuni:latest",
			wantImage: "ghcr.io/uyuni-project/mcp-server-uyuni:latest",
			wantEnvVars: map[string]string{
				"UYUNI_SERVER":        "http://dummy.domain.com",
				"UYUNI_USER":          "admin",
				"UYUNI_PASS":          "admin",
				"UYUNI_MCP_TRANSPORT": "http",
				"UYUNI_MCP_HOST":      "0.0.0.0",
			},
			wantPort: 8000,
			wantErr:  false,
		},
		{
			name:      "valid bugzilla command",
			command:   "docker run -it --rm -e BUGZILLA_SERVER=https://bugzilla.suse.com -e BUGZILLA_HOST=0.0.0.0 -e BUGZILLA_PORT=8000 ghcr.io/openSUSE/mcp-bugzilla:latest",
			wantImage: "ghcr.io/openSUSE/mcp-bugzilla:latest",
			wantEnvVars: map[string]string{
				"BUGZILLA_SERVER": "https://bugzilla.suse.com",
				"BUGZILLA_HOST":   "0.0.0.0",
				"BUGZILLA_PORT":   "8000",
			},
			wantPort: 8000,
			wantErr:  false,
		},
		{
			name:    "invalid command - not docker run",
			command: "kubectl run test",
			wantErr: true,
		},
		{
			name:    "invalid command - no image",
			command: "docker run -it --rm -e TEST=value",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotImage, gotEnvVars, gotPort, err := sm.parseDockerCommand(tt.command)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDockerCommand() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if gotImage != tt.wantImage {
					t.Errorf("parseDockerCommand() gotImage = %v, want %v", gotImage, tt.wantImage)
				}
				if gotPort != tt.wantPort {
					t.Errorf("parseDockerCommand() gotPort = %v, want %v", gotPort, tt.wantPort)
				}
				if len(gotEnvVars) != len(tt.wantEnvVars) {
					t.Errorf("parseDockerCommand() gotEnvVars length = %v, want %v", len(gotEnvVars), len(tt.wantEnvVars))
				}
				for k, v := range tt.wantEnvVars {
					if gotEnvVars[k] != v {
						t.Errorf("parseDockerCommand() gotEnvVars[%s] = %v, want %v", k, gotEnvVars[k], v)
					}
				}
			}
		})
	}
}

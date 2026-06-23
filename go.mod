module github.com/heurema/pactum

go 1.26

require (
	github.com/alecthomas/kong v1.15.0
	github.com/coder/acp-go-sdk v0.13.5
	gopkg.in/yaml.v3 v3.0.1
)

require (
	golang.org/x/mod v0.36.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.44.0 // indirect
	golang.org/x/telemetry v0.0.0-20260508192327-42602be52be6 // indirect
	golang.org/x/tools v0.45.0 // indirect
	golang.org/x/vuln v1.3.0 // indirect
)

tool (
	golang.org/x/tools/cmd/deadcode
	golang.org/x/vuln/cmd/govulncheck
)

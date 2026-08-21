package main

// version is set at build time via:
//
//	-ldflags "-X main.version=<version>"
//
// (see .goreleaser.yml). Defaults to "dev" for local `go build`/`go run`.
var version = "dev"

func main() {
	Execute()
}

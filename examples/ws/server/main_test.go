package main

import "testing"

func TestFixedWebsocketConfig(t *testing.T) {
	if websocketURL != "ws://127.0.0.1:8999" {
		t.Fatalf("unexpected websocket URL: %q", websocketURL)
	}
	if capabilityToken != "codex-go-sdk-ws-example-token" {
		t.Fatalf("unexpected capability token: %q", capabilityToken)
	}
	if capabilityTokenSha256 != "ad26ba1e118300f710156819514d134e224a2670166802bf628843f54d9049e2" {
		t.Fatalf("unexpected capability token sha256: %q", capabilityTokenSha256)
	}
}

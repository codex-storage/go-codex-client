//go:build codex_integration

package codextestutils

import (
	"go-codex-client/codexclient"
	"testing"

	"github.com/codex-storage/codex-go-bindings/codex"
)

func NewCodexClientTest(t *testing.T) *codexclient.CodexClient {
	client, err := codexclient.NewCodexClient(codex.Config{
		DataDir:        t.TempDir(),
		LogFormat:      codex.LogFormatNoColors,
		MetricsEnabled: false,
		BlockRetries:   5,
		DiscoveryPort:  8092,
	})
	if err != nil {
		t.Fatalf("Failed to create Codex node: %v", err)
	}

	err = client.Start()
	if err != nil {
		t.Fatalf("Failed to start Codex node: %v", err)
	}

	t.Cleanup(func() {
		if err := client.Stop(); err != nil {
			t.Logf("cleanup codex: %v", err)
		}

		if err := client.Destroy(); err != nil {
			t.Logf("cleanup codex: %v", err)
		}
	})

	return client
}

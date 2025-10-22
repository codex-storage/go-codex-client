//go:build !disable_torrent
// +build !disable_torrent

package communities

import (
	"context"
)

// Mock generation instruction above will create a mock in package `mock_communities`
// (folder `mock/`) so tests can import it as e.g. `go-codex-client/communities/mock` or
// with an alias like `mocks` to avoid import-cycle issues.
//
// CodexClientInterface defines the interface for CodexClient operations needed by the downloader
//
//go:generate mockgen -package=mock_communities -source=codex_client_interface.go -destination=mock/codex_client_interface.go
type CodexClientInterface interface {
	TriggerDownloadWithContext(ctx context.Context, cid string) (*CodexManifest, error)
	HasCid(cid string) (bool, error)
}

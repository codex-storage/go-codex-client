/* Package communities
*
* Provides a CodexClient type that you can use to conveniently
* upload buffers to Codex.
*
 */
package communities

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/codex-storage/codex-go-bindings/codex"
)

// CodexClient handles basic upload/download operations with Codex storage
type CodexClient struct {
	node   *codex.CodexNode
	config *codex.Config
}

type CodexManifest = codex.Manifest
type CodexConf = codex.Config

// NewCodexClient creates a new Codex client
func NewCodexClient(config codex.Config) (*CodexClient, error) {
	node, err := codex.New(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Codex node: %w", err)
	}

	return &CodexClient{
		node:   node,
		config: &config,
	}, nil
}

func (c CodexClient) Start() error {
	return c.node.Start()
}

func (c CodexClient) Stop() error {
	return c.node.Stop()
}

func (c CodexClient) Destroy() error {
	return c.node.Destroy()
}

// Upload uploads data from a reader to Codex and returns the CID
func (c *CodexClient) Upload(data io.Reader, filename string) (string, error) {
	return c.node.UploadReader(context.Background(), codex.UploadOptions{
		Filepath: filename,
	}, data)
}

// Download downloads data from Codex by CID and writes it to the provided writer
func (c *CodexClient) Download(cid string, output io.Writer) error {
	return c.DownloadWithContext(context.Background(), cid, output)
}

func (c *CodexClient) TriggerDownload(cid string) (CodexManifest, error) {
	return c.TriggerDownloadWithContext(context.Background(), cid)
}

func (c *CodexClient) HasCid(cid string) (bool, error) {
	err := c.LocalDownload(cid, io.Discard)
	return err == nil, nil
}

func (c *CodexClient) RemoveCid(cid string) error {
	return c.node.Delete(cid)
}

// DownloadWithContext downloads data from Codex by CID with cancellation support
func (c *CodexClient) DownloadWithContext(ctx context.Context, cid string, output io.Writer) error {
	return c.node.DownloadStream(ctx, cid, codex.DownloadStreamOptions{
		Writer: output,
	})
}

func (c *CodexClient) LocalDownload(cid string, output io.Writer) error {
	return c.LocalDownloadWithContext(context.Background(), cid, output)
}

func (c *CodexClient) LocalDownloadWithContext(ctx context.Context, cid string, output io.Writer) error {
	return c.node.DownloadStream(ctx, cid, codex.DownloadStreamOptions{
		Writer: output,
		Local:  true,
	})
}

func (c *CodexClient) FetchManifestWithContext(ctx context.Context, cid string) (CodexManifest, error) {
	return c.node.DownloadManifest(cid)
}

func (c *CodexClient) TriggerDownloadWithContext(ctx context.Context, cid string) (CodexManifest, error) {
	return c.node.Fetch(cid)
}

// UploadArchive is a convenience method for uploading archive data
func (c *CodexClient) UploadArchive(encodedArchive []byte) (string, error) {
	return c.Upload(bytes.NewReader(encodedArchive), "archive-data.bin")
}

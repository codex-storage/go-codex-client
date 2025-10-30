package codexmanifest

type CodexManifest struct {
	Cid         string
	TreeCid     string
	DatasetSize int
	BlockSize   int
	Filename    string
	Mimetype    string
	Protected   bool
}

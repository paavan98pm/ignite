package manifest

import (
	"github.com/weaveworks/ignite/pkg/apis/ignite/scheme"
	"github.com/weaveworks/ignite/pkg/constants"
	"github.com/weaveworks/ignite/pkg/storage"
)

func NewManifestStorage(dataDir string) *ManifestStorage {
	gitRaw := NewManifestRawStorage(dataDir, constants.DATA_DIR)
	return &ManifestStorage{
		gitRaw:  gitRaw,
		Storage: storage.NewGenericStorage(gitRaw, scheme.Serializer),
	}
}

// ManifestStorage implements the storage interface for GitOps purposes
type ManifestStorage struct {
	storage.Storage
	gitRaw *ManifestRawStorage
}

func (s *ManifestStorage) Sync() (UpdatedFiles, error) {
	return s.gitRaw.Sync()
}

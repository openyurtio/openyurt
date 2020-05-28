package factory

import (
	"github.com/alibaba/openyurt/pkg/yurthub/storage"
	"github.com/alibaba/openyurt/pkg/yurthub/storage/disk"
)

// CreateStorage create a storage.Store for backend storage
func CreateStorage() (storage.Store, error) {
	return disk.NewDiskStorage("")
}

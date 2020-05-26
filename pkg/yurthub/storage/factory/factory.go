package factory

import (
	"github.com/alibaba/openyurt/pkg/yurthub/storage"
	"github.com/alibaba/openyurt/pkg/yurthub/storage/disk"
)

func CreateStorage() (storage.Store, error) {

	return disk.NewDiskStorage()
}

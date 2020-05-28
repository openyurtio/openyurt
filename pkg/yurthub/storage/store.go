package storage

import (
	"errors"
)

// ErrStorageAccessConflict is an error for accessing key conflict
var ErrStorageAccessConflict = errors.New("specified key is under accessing")

// Store is an interface for caching data into backend storage
type Store interface {
	Create(key string, contents []byte) error
	Delete(key string) error
	Get(key string) ([]byte, error)
	ListKeys(key string) ([]string, error)
	List(key string) ([][]byte, error)
	Update(key string, contents []byte) error
}

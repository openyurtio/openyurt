package storage

import (
	"errors"
)

var ErrStorageAccessConflict = errors.New("specified key is under accessing")

type Store interface {
	Create(key string, contents []byte) error
	Delete(key string) error
	Get(key string) ([]byte, error)
	ListKeys(key string) ([]string, error)
	List(key string) ([][]byte, error)
	Update(key string, contents []byte) error
}

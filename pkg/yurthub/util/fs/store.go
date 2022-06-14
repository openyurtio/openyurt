/*
Copyright 2022 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package fs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type ListMode string

var (
	ListModeDirs  ListMode = "dirs"
	ListModeFiles ListMode = "files"
	ListModeAll   ListMode = "all"
)

type FileSystemOperator struct{}

// Read will read the file at path.
// If the path does not exist, ErrNotExists will be returned.
// If the path is not a regular file, ErrIsNotFile will be returned.
func (fs *FileSystemOperator) Read(path string) ([]byte, error) {
	if !IfExists(path) {
		return nil, ErrNotExists
	}

	if ok, err := IsRegularFile(path); !ok || err != nil {
		if !ok {
			return nil, ErrIsNotFile
		}
		return nil, err
	}

	return os.ReadFile(path)
}

// Write will write the content at path.
// If the path does not exist, return ErrNotExists.
// If the path is not a regular file, return ErrIsNotFile.
func (fs *FileSystemOperator) Write(path string, content []byte) error {
	if !IfExists(path) {
		return ErrNotExists
	}

	if ok, err := IsRegularFile(path); !ok || err != nil {
		if !ok {
			return ErrIsNotFile
		}
		return err
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC|os.O_SYNC, 0600)
	if err != nil {
		return err
	}
	n, err := f.Write(content)
	if err == nil && n < len(content) {
		err = io.ErrShortWrite
	}
	err1 := f.Close()
	if err == nil {
		err = err1
	}
	return err
}

// list will list names of entries under the path. If isRecurisive is set, it will
// walk the path including all subdirs.
// There're three modes:
// "dirs" returns names of all directories.
// "files" returns names of all regular files.
// "all" returns names of all entries.
//
// If the path does not exist, return ErrNotExists.
// If the path is not a directory, return ErrIsNotDir.
func (fs *FileSystemOperator) List(path string, mode ListMode, isRecursive bool) ([]string, error) {
	if !IfExists(path) {
		return nil, ErrNotExists
	}
	if ok, err := IsDir(path); !ok || err != nil {
		if !ok {
			return nil, ErrIsNotDir
		}
		return nil, err
	}

	dirs := []string{}
	files := []string{}
	if isRecursive {
		filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			switch {
			case info.Mode().IsDir():
				dirs = append(dirs, info.Name())
			case info.Mode().IsRegular():
				files = append(files, info.Name())
			default:
				// something like symbolic link, currently ignore it
			}
			return nil
		})
	} else {
		infos, err := os.ReadDir(path)
		if err != nil {
			return nil, err
		}
		for i := range infos {
			switch {
			case infos[i].Type().IsDir():
				dirs = append(dirs, infos[i].Name())
			case infos[i].Type().IsRegular():
				files = append(files, infos[i].Name())
			default:
				// something like symbolic link, currently ignore it
			}
		}
	}

	switch mode {
	case ListModeDirs:
		return dirs, nil
	case ListModeFiles:
		return files, nil
	case ListModeAll:
		return append(dirs, files...), nil
	default:
		return nil, fmt.Errorf("unknown mode %s", mode)
	}
}

// DeleteFile will delete file at path.
// If the path does not exist, do nothing.
// If the path exists but is not a regular file, return ErrIsNotFile.
func (fs *FileSystemOperator) DeleteFile(path string) error {
	if !IfExists(path) {
		return nil
	}

	if ok, err := IsRegularFile(path); !ok || err != nil {
		if !ok {
			return ErrIsNotFile
		}
		return err
	}

	return os.Remove(path)
}

// DeleteDir will delete directory at path. All files and subdirs will be deleted.
// If the path does not exist, do nothing.
// If the path does not a directory, return ErrIsNotDir.
func (fs *FileSystemOperator) DeleteDir(path string) error {
	if !IfExists(path) {
		return nil
	}

	if ok, err := IsDir(path); !ok || err != nil {
		if !ok {
			return ErrIsNotDir
		}
		return err
	}

	return os.RemoveAll(path)
}

// CreateDir will create the dir at path.
// If the path has already existed and is not dir, return ErrIsNotDir.
// If the path has already existed and is a dir, return ErrExists.
func (fs *FileSystemOperator) CreateDir(path string) error {
	if IfExists(path) {
		if ok, err := IsDir(path); !ok && err == nil {
			return ErrIsNotDir
		}
		return ErrExists
	}

	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}
	return nil
}

// CreateFile will create the file at path with content.
// If the path has already existed and is not regular file, return ErrIsNotFile.
// If the path has already existed and is regular file, return ErrExists.
func (fs *FileSystemOperator) CreateFile(path string, content []byte) error {
	// check the path
	if IfExists(path) {
		if ok, err := IsRegularFile(path); !ok && err == nil {
			return ErrIsNotFile
		}
		return ErrExists
	}

	// ensure the base dir
	dir, filename := filepath.Split(path)
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	// create the file with mode and write content into it
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|os.O_SYNC, 0600)
	if err != nil {
		return err
	}
	n, err := f.Write(content)
	if err == nil && n < len(content) {
		err = io.ErrShortWrite
	}
	// close file
	err1 := f.Close()
	if err == nil {
		err = err1
	}
	return err
}

func (fs *FileSystemOperator) Rename(oldPath string, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func IfExists(path string) bool {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

func IsRegularFile(path string) (bool, error) {
	if info, err := os.Stat(path); err != nil {
		return false, err
	} else {
		return info.Mode().IsRegular(), nil
	}
}

func IsDir(path string) (bool, error) {
	if info, err := os.Stat(path); err != nil {
		return false, err
	} else {
		return info.Mode().IsDir(), nil
	}
}

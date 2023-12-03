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
	"sort"
)

type ListMode string

var (
	ListModeDirs  ListMode = "dirs"
	ListModeFiles ListMode = "files"
)

type FileSystemOperator struct{}

// Read will read the file at path.
// If the path does not exist, ErrNotExists will be returned.
// If the path is not a regular file, ErrIsNotFile will be returned.
func (fs *FileSystemOperator) Read(path string) ([]byte, error) {
	if !IfExists(path) {
		return nil, ErrNotExists
	}

	if ok, err := IsRegularFile(path); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrIsNotFile
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

	if ok, err := IsRegularFile(path); err != nil {
		return err
	} else if !ok {
		return ErrIsNotFile
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

// list will list names of entries under the rootDir(except the root dir). If isRecurisive is set, it will
// walk the path including all subdirs. The returned file names are absolute paths and
// in lexicographical order.
// There're two modes:
// "dirs" returns names of all directories.
// "files" returns names of all regular files.
//
// If the path does not exist, return ErrNotExists.
// If the path is not a directory, return ErrIsNotDir.
// If the rootDir exists but has no entry, return a empty slice and nil error.
func (fs *FileSystemOperator) List(rootDir string, mode ListMode, isRecursive bool) ([]string, error) {
	if !IfExists(rootDir) {
		return nil, ErrNotExists
	}
	if ok, err := IsDir(rootDir); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrIsNotDir
	}

	dirs := []string{}
	files := []string{}
	if isRecursive {
		filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}

			info, err := d.Info()
			if err != nil {
				return fmt.Errorf("could not get info for entry %s, %v", path, err)
			}

			switch {
			case info.Mode().IsDir():
				if path == rootDir {
					return nil
				}
				dirs = append(dirs, path)
			case info.Mode().IsRegular():
				files = append(files, path)
			default:
				// something like symbolic link, currently ignore it
			}
			return nil
		})
	} else {
		infos, err := os.ReadDir(rootDir)
		if err != nil {
			return nil, err
		}
		for i := range infos {
			switch {
			case infos[i].Type().IsDir():
				dirs = append(dirs, filepath.Join(rootDir, infos[i].Name()))
			case infos[i].Type().IsRegular():
				files = append(files, filepath.Join(rootDir, infos[i].Name()))
			default:
				// something like symbolic link, currently ignore it
			}
		}
	}

	switch mode {
	case ListModeDirs:
		sort.Strings(dirs)
		return dirs, nil
	case ListModeFiles:
		sort.Strings(files)
		return files, nil
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

	if ok, err := IsRegularFile(path); err != nil {
		return err
	} else if !ok {
		return ErrIsNotFile
	}

	return os.RemoveAll(path)
}

// DeleteDir will delete directory at path. All files and subdirs will be deleted.
// If the path does not exist, do nothing.
// If the path does not a directory, return ErrIsNotDir.
func (fs *FileSystemOperator) DeleteDir(path string) error {
	if !IfExists(path) {
		return nil
	}

	if ok, err := IsDir(path); err != nil {
		return err
	} else if !ok {
		return ErrIsNotDir
	}

	return os.RemoveAll(path)
}

// CreateDir will create the dir at path. If the parent dir of this path does not
// exist, it will be created first.
// If the path has already existed and is not dir, return ErrIsNotDir.
// If the path has already existed and is a dir, return ErrExists.
func (fs *FileSystemOperator) CreateDir(path string) error {
	if IfExists(path) {
		if ok, err := IsDir(path); !ok && err == nil {
			return ErrIsNotDir
		}
		return ErrExists
	}

	return os.MkdirAll(path, 0755)
}

// CreateFile will create the file at path with content. If its parent dir does not
// exist, it will be created first.
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
	dir := filepath.Dir(path)
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
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|os.O_SYNC, 0600)
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

// Rename will rename file(or directory) at oldPath as newPath.
// If file of newPath has already existed, it will be replaced.
// If path does not exists, return ErrNotExists.
// If oldPath and newPath does not under the same parent dir, return ErrInvalidPath.
func (fs *FileSystemOperator) Rename(oldPath string, newPath string) error {
	if !IfExists(oldPath) {
		return ErrNotExists
	}

	if ok, err := IsDir(newPath); ok && err == nil {
		if err := fs.DeleteDir(newPath); err != nil {
			return err
		}
	}

	if filepath.Dir(oldPath) != filepath.Dir(newPath) {
		return ErrInvalidPath
	}

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
		if os.IsNotExist(err) {
			return false, ErrNotExists
		}
		return false, err
	} else {
		return info.Mode().IsRegular(), nil
	}
}

func IsDir(path string) (bool, error) {
	if info, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, ErrNotExists
		}
		return false, err
	} else {
		return info.Mode().IsDir(), nil
	}
}

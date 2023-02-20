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
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var testDirAbsPath = "/tmp/testFS"
var testSubDirs = map[string]string{
	"read":   "readTestDir",
	"write":  "writeTestDir",
	"create": "createTestDir",
	"delete": "deleteTestDir",
	"list":   "listTestDir",
	"rename": "renameTestDir",
}
var multiLineContents = []byte("line 1\nline 2\nline 3\n")
var notExistFileName = "fileNotExist"
var notExistDirName = "dirNotExist"

var _ = BeforeSuite(func() {
	err := os.RemoveAll(testDirAbsPath)
	Expect(err).To(BeNil())
	err = os.MkdirAll(testDirAbsPath, 0755)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("failed to create test dir %s, %v", testDirAbsPath, err))
	for _, subDir := range testSubDirs {
		err := os.Mkdir(filepath.Join(testDirAbsPath, subDir), 0755)
		Expect(err).To(BeNil(), fmt.Sprintf("failed to create sub dir %s, %v", subDir, err))
	}
})

var _ = AfterSuite(func() {
	err := os.RemoveAll(testDirAbsPath)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("failed to clean test dir %s, %v", testDirAbsPath, err))
})

var _ = Describe("Test FileSystemOperator Read", func() {
	var testName string
	var baseDir string
	var fsOperator *FileSystemOperator
	var contents []byte
	var fileName string
	var err error

	BeforeEach(func() {
		testName = "read"
		baseDir = filepath.Join(testDirAbsPath, testSubDirs[testName])
		fsOperator = &FileSystemOperator{}
		contents = multiLineContents
		fileName = uuid.New().String()
		err = createAndWriteToFile(filepath.Join(baseDir, fileName), contents)
		Expect(err).To(BeNil(), fmt.Sprintf("failed to prepare file for read test %v", err))
	})
	AfterEach(func() {
		err := deletePath(filepath.Join(baseDir, fileName))
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("failed to remove read test dir, %v", err))
	})

	It("should read all contents from file", func() {
		buf, err := fsOperator.Read(filepath.Join(baseDir, fileName))
		Expect(err).To(BeNil())
		Expect(buf).To(Equal(contents))
	})

	It("should return ErrNotExists if file does not exist", func() {
		_, err := fsOperator.Read(filepath.Join(baseDir, notExistFileName))
		Expect(err).To(Equal(ErrNotExists))
	})

	It("should return ErrIsNotFile if path is not a regular file", func() {
		_, err := fsOperator.Read(baseDir)
		Expect(err).To(Equal(ErrIsNotFile))
	})
})

var _ = Describe("Test FileSystemOperator Write", func() {
	var testName string
	var baseDir string
	var contents []byte
	var fsOperator *FileSystemOperator
	var fileName string
	var err error

	BeforeEach(func() {
		testName = "write"
		baseDir = filepath.Join(testDirAbsPath, testSubDirs[testName])
		contents = multiLineContents
		fileName = uuid.New().String()
		fsOperator = &FileSystemOperator{}
		err = createAndWriteToFile(filepath.Join(baseDir, fileName), []byte{})
		Expect(err).To(BeNil())
	})
	AfterEach(func() {
		err := deletePath(filepath.Join(baseDir, fileName))
		Expect(err).To(BeNil(), fmt.Errorf("failed to delete file %s for write test, %v", fileName, err))
	})

	It("should write contents to file", func() {
		err := fsOperator.Write(filepath.Join(baseDir, fileName), contents)
		Expect(err).To(BeNil())
		buf, err := readFile(filepath.Join(baseDir, fileName))
		Expect(err).To(BeNil())
		Expect(buf).To(Equal(contents))
	})

	It("should return ErrIsNotFile if file is not a regular file", func() {
		err := fsOperator.Write(baseDir, contents)
		Expect(err).To(Equal(ErrIsNotFile))
	})

	It("should return ErrNotExists if file does not exist", func() {
		err := fsOperator.Write(filepath.Join(baseDir, notExistFileName), contents)
		Expect(err).To(Equal(ErrNotExists))
	})
})

var _ = Describe("Test FileSystemOperator List", func() {
	var testName string
	var baseDir string
	var baseEmptyDir string
	var fsOperator *FileSystemOperator
	var generatedNames []string
	var generatedPaths []string

	BeforeEach(func() {
		testName = "list"
		fsOperator = &FileSystemOperator{}
		generatedNames = make([]string, 9)
		baseDir = filepath.Join(testDirAbsPath, testSubDirs[testName], uuid.New().String())
		baseEmptyDir = filepath.Join(testDirAbsPath, testSubDirs[testName], uuid.New().String())
		for i := range generatedNames {
			generatedNames[i] = uuid.New().String()
		}

		// fs tree to generate
		generatedPaths = []string{
			filepath.Join(baseDir, generatedNames[0]),
			filepath.Join(baseDir, generatedNames[1]),
			filepath.Join(baseDir, generatedNames[2]),
			filepath.Join(baseDir, generatedNames[3], generatedNames[4]),
			filepath.Join(baseDir, generatedNames[3], generatedNames[5]),
			filepath.Join(baseDir, generatedNames[6], generatedNames[7], generatedNames[8]),
		}

		Expect(os.Mkdir(baseDir, 0755)).To(BeNil())
		Expect(os.Mkdir(baseEmptyDir, 0755)).To(BeNil())
		for _, path := range generatedPaths {
			dir := filepath.Dir(path)
			err := os.MkdirAll(dir, 0755)
			Expect(err).To(BeNil())
			err = createAndWriteToFile(path, []byte{})
			Expect(err).To(BeNil())
		}
	})

	AfterEach(func() {
		Expect(os.RemoveAll(baseEmptyDir)).To(BeNil())
		Expect(os.RemoveAll(baseDir)).To(BeNil())
	})

	It("should list files non-recursively", func() {
		files, err := fsOperator.List(baseDir, ListModeFiles, false)
		Expect(err).To(BeNil())
		sorted := []string{generatedPaths[0], generatedPaths[1], generatedPaths[2]}
		sort.Strings(sorted)
		Expect(files).To(Equal(sorted))
	})

	It("should list dirs non-recursively", func() {
		dirs, err := fsOperator.List(baseDir, ListModeDirs, false)
		Expect(err).To(BeNil())
		sorted := []string{
			filepath.Join(baseDir, generatedNames[3]),
			filepath.Join(baseDir, generatedNames[6]),
		}
		sort.Strings(sorted)
		Expect(dirs).To(Equal(sorted))
	})

	It("should list files recursively", func() {
		files, err := fsOperator.List(baseDir, ListModeFiles, true)
		Expect(err).To(BeNil())
		sort.Strings(generatedPaths)
		Expect(files).To(Equal(generatedPaths))
	})

	It("should list dirs recursively", func() {
		dirs, err := fsOperator.List(baseDir, ListModeDirs, true)
		Expect(err).To(BeNil())
		sorted := []string{
			filepath.Join(baseDir, generatedNames[3]),
			filepath.Join(baseDir, generatedNames[6]),
			filepath.Join(baseDir, generatedNames[6], generatedNames[7]),
		}
		sort.Strings(sorted)
		Expect(dirs).To(Equal(sorted))
	})

	It("should return ErrNotExists if path does not exist", func() {
		_, err := fsOperator.List(filepath.Join(baseDir, notExistDirName), ListModeFiles, false)
		Expect(err).To(Equal(ErrNotExists))
	})

	It("should return ErrIsNotDir if path is not dir", func() {
		_, err := fsOperator.List(generatedPaths[0], ListModeFiles, false)
		Expect(err).To(Equal(ErrIsNotDir))
	})

	It("should return empty slice if the dir is empty", func() {
		files, err := fsOperator.List(baseEmptyDir, ListModeFiles, false)
		Expect(err).To(BeNil())
		Expect(len(files)).To(BeZero())
	})
})

var _ = Describe("Test FileSystemOperator Delete", func() {
	var testName string
	var baseDir string
	var fsOperator *FileSystemOperator
	var fileName string
	var dirName string
	var err error

	BeforeEach(func() {
		testName = "delete"
		baseDir = filepath.Join(testDirAbsPath, testSubDirs[testName])
		fileName = uuid.New().String()
		dirName = uuid.New().String()
		fsOperator = &FileSystemOperator{}

		err = createAndWriteToFile(filepath.Join(baseDir, fileName), []byte{})
		Expect(err).To(BeNil())
		err = os.Mkdir(filepath.Join(baseDir, dirName), 0755)
		Expect(err).To(BeNil())
	})
	AfterEach(func() {
		err = deletePath(filepath.Join(baseDir, fileName))
		Expect(err).To(BeNil())
		err = deletePath(filepath.Join(baseDir, dirName))
		Expect(err).To(BeNil())
	})

	Context("TestDeleteFile", func() {
		It("should delete file when file exists", func() {
			path := filepath.Join(baseDir, fileName)
			err = fsOperator.DeleteFile(path)
			Expect(err).To(BeNil())
			_, err := os.Stat(path)
			Expect(os.IsNotExist(err)).To(Equal(true))
		})
		It("should return nil when file does not exist", func() {
			err = fsOperator.DeleteFile(filepath.Join(baseDir, notExistFileName))
			Expect(err).To(BeNil())
		})
		It("should return ErrIsNotFile if path is a directory", func() {
			err = fsOperator.DeleteFile(filepath.Join(baseDir, dirName))
			Expect(err).To(Equal(ErrIsNotFile))
		})
	})

	Context("TestDeleteDir", func() {
		It("should delete dir when dir exists", func() {
			path := filepath.Join(baseDir, dirName)
			err = fsOperator.DeleteDir(path)
			Expect(err).To(BeNil())
			_, err := os.Stat(path)
			Expect(os.IsNotExist(err)).To(Equal(true))
		})
		It("should return nil when dir does not exist", func() {
			err = fsOperator.DeleteDir(filepath.Join(baseDir, notExistDirName))
			Expect(err).To(BeNil())
		})
		It("should return ErrIsNotDir when path is not a directory", func() {
			err = fsOperator.DeleteDir(filepath.Join(baseDir, fileName))
			Expect(err).To(Equal(ErrIsNotDir))
		})
	})
})

var _ = Describe("Test FileSystemOperator Create", func() {
	var testName string
	var baseDir string
	var contents []byte
	var fsOperator *FileSystemOperator
	var fileName string
	var dirName string
	var err error

	BeforeEach(func() {
		testName = "create"
		contents = multiLineContents
		baseDir = filepath.Join(testDirAbsPath, testSubDirs[testName])
		fileName = uuid.New().String()
		dirName = uuid.New().String()
		fsOperator = &FileSystemOperator{}
	})
	AfterEach(func() {
		// nothing to do
	})

	Context("TestCreateFile", func() {
		It("should create file directly with content if it does not exist but dir exists", func() {
			err = fsOperator.CreateFile(filepath.Join(baseDir, fileName), contents)
			Expect(err).To(BeNil())
			buf, err := readFile(filepath.Join(baseDir, fileName))
			Expect(err).To(BeNil(), fmt.Sprintf("failed to read file at %s, %v", filepath.Join(baseDir, fileName), err))
			Expect(buf).To(Equal(contents))
		})
		It("should create the parent dir if it does not exist and then create the file with content", func() {
			parentDir := filepath.Join(baseDir, uuid.New().String())
			err = fsOperator.CreateFile(filepath.Join(parentDir, fileName), contents)
			Expect(err).To(BeNil())
			buf, err := readFile(filepath.Join(parentDir, fileName))
			Expect(err).To(BeNil())
			Expect(buf).To(Equal(contents))
		})
		It("should return ErrExists if the path has already existed", func() {
			err = createAndWriteToFile(filepath.Join(baseDir, fileName), []byte{})
			Expect(err).To(BeNil())
			err = fsOperator.CreateFile(filepath.Join(baseDir, fileName), []byte{})
			Expect(err).To(Equal(ErrExists))
		})
		It("should return ErrIsNotFile if the path exists but is not a regular file", func() {
			err = os.MkdirAll(filepath.Join(baseDir, fileName), 0755)
			Expect(err).To(BeNil())
			err = fsOperator.CreateFile(filepath.Join(baseDir, fileName), contents)
			Expect(err).To(Equal(ErrIsNotFile))
		})
	})

	Context("TestCreateDir", func() {
		It("should create dir directly if it does not exist but its parent dir exists", func() {
			err = fsOperator.CreateDir(filepath.Join(baseDir, dirName))
			Expect(err).To(BeNil())
			info, err := os.Stat(filepath.Join(baseDir, dirName))
			Expect(err).To(BeNil())
			Expect(info.IsDir()).To(BeTrue())
		})
		It("should create the parent dir if it does not exist and then create the dir", func() {
			parentDir := filepath.Join(baseDir, uuid.New().String())
			err = fsOperator.CreateDir(filepath.Join(parentDir, dirName))
			Expect(err).To(BeNil())
			info, err := os.Stat(filepath.Join(parentDir, dirName))
			Expect(err).To(BeNil())
			Expect(info.IsDir()).To(BeTrue())
		})
		It("should return ErrExists if the path has already existed", func() {
			err = os.MkdirAll(filepath.Join(baseDir, dirName), 0755)
			Expect(err).To(BeNil())
			err = fsOperator.CreateDir(filepath.Join(baseDir, dirName))
			Expect(err).To(Equal(ErrExists))
		})
		It("should return ErrIsNotDir if the path exists but is not a directory", func() {
			err = createAndWriteToFile(filepath.Join(baseDir, dirName), contents)
			Expect(err).To(BeNil())
			err = fsOperator.CreateDir(filepath.Join(baseDir, dirName))
			Expect(err).To(Equal(ErrIsNotDir))
		})
	})
})

var _ = Describe("Test FileSystemOperator Rename", func() {
	var testName string
	var baseDir string
	var contents []byte
	var fsOperator *FileSystemOperator
	var fileName string
	var newFileName string
	var dirName string
	var newDirName string
	var err error

	BeforeEach(func() {
		testName = "rename"
		baseDir = filepath.Join(testDirAbsPath, testSubDirs[testName])
		contents = multiLineContents
		fsOperator = &FileSystemOperator{}
		fileName = uuid.New().String()
		newFileName = uuid.New().String()
		dirName = uuid.New().String()
		newDirName = uuid.New().String()
		err = createAndWriteToFile(filepath.Join(baseDir, fileName), contents)
		Expect(err).To(BeNil())
		err = os.Mkdir(filepath.Join(baseDir, dirName), 0755)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		err = deletePath(filepath.Join(baseDir, fileName))
		Expect(err).To(BeNil())
		err = deletePath(filepath.Join(baseDir, dirName))
		Expect(err).To(BeNil())
	})

	It("should rename file under the same dir and replace the file at newPath if it has already existed", func() {
		err = createAndWriteToFile(filepath.Join(baseDir, newFileName), []byte("old content"))
		Expect(err).To(BeNil())
		err = fsOperator.Rename(filepath.Join(baseDir, fileName), filepath.Join(baseDir, newFileName))
		Expect(err).To(BeNil())
		_, err = os.Stat(filepath.Join(baseDir, fileName))
		Expect(os.IsNotExist(err)).To(BeTrue())
		buf, err := readFile(filepath.Join(baseDir, newFileName))
		Expect(err).To(BeNil())
		Expect(buf).To(Equal(contents))
	})
	It("should rename dir under the same dir", func() {
		err = fsOperator.Rename(filepath.Join(baseDir, dirName), filepath.Join(baseDir, newDirName))
		Expect(err).To(BeNil())
		_, err = os.Stat(filepath.Join(baseDir, dirName))
		Expect(os.IsNotExist(err)).To(BeTrue())
		info, err := os.Stat(filepath.Join(baseDir, newDirName))
		Expect(err).To(BeNil())
		Expect(info.IsDir()).To(BeTrue())
	})
	It("should return ErrNotExists if path does not exist", func() {
		err = fsOperator.Rename(filepath.Join(baseDir, notExistFileName), filepath.Join(baseDir, newFileName))
		Expect(err).To(Equal(ErrNotExists))
	})
	It("should return ErrInvalidPath if oldPath and newPath are not under the same dir", func() {
		err = fsOperator.Rename(filepath.Join(baseDir, fileName), filepath.Join(newFileName))
		Expect(err).To(Equal(ErrInvalidPath))
	})
})

func deletePath(path string) error {
	return os.RemoveAll(path)
}

func createAndWriteToFile(filePath string, content []byte) error {
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0755)
	if err != nil {
		return fmt.Errorf("failed to create file %s for read test, %v", filePath, err)
	}
	n, err := f.Write(content)
	if err != nil {
		return fmt.Errorf("failed to write to file %s for read test, %v", filePath, err)
	}
	if n != len(content) {
		return fmt.Errorf("failed to write all data into file %s, %v", filePath, err)
	}
	return f.Close()
}

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func TestFileSystemOperator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "FileSystemOperator Suite")
}

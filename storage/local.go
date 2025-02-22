// Copyright © 2024 Ha Nguyen <captainnemot1k60@gmail.com>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package storage

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	"github.com/hantbk/vtsbackup/helper"
	"github.com/hantbk/vtsbackup/logger"
)

// Local storage
//
// type: local
// path: /data/backups
type Local struct {
	Base
	path string
}

func (s *Local) open() error {
	s.path = s.viper.GetString("path")
	return helper.MkdirP(s.path)
}

func (s *Local) close() {}

func (s *Local) upload(fileKey string) (err error) {
	logger := logger.Tag("Local")

	// Related path
	if !path.IsAbs(s.path) {
		s.path = path.Join(s.model.WorkDir, s.path)
	}

	targetPath := path.Join(s.path, fileKey)
	targetDir := path.Dir(targetPath)
	if err := helper.MkdirP(targetDir); err != nil {
		logger.Errorf("failed to mkdir %q, %v", targetDir, err)
	}

	_, err = helper.Exec("cp", "-a", s.archivePath, targetPath)
	if err != nil {
		return err
	}
	logger.Info("Store succeeded", targetPath)
	return nil
}

func (s *Local) delete(fileKey string) (err error) {
	targetPath := filepath.Join(s.path, fileKey)
	logger.Info("Deleting", targetPath)

	return os.Remove(targetPath)
}

// List all files
func (s *Local) list(parent string) ([]FileItem, error) {
	remotePath := filepath.Join(s.path, parent)
	var items = []FileItem{}

	files, err := ioutil.ReadDir(remotePath)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		if !file.IsDir() {
			items = append(items, FileItem{
				Filename:     file.Name(),
				Size:         file.Size(),
				LastModified: file.ModTime(),
			})
		}
	}

	return items, nil
}

func (s *Local) download(fileKey string) (string, error) {
	return "", fmt.Errorf("Local is not support download")
}

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
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hantbk/vtsbackup/config"
	"github.com/hantbk/vtsbackup/helper"
	"github.com/hantbk/vtsbackup/logger"
)

type PackageList []Package

// When `FileKeys` is not empty, `FileKey` is the directory
type Package struct {
	FileKey   string    `json:"file_key"`
	FileKeys  []string  `json:"file_keys,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

var (
	cyclerPath = filepath.Join(config.VtsBackupDir, "cycler")
)

type Cycler struct {
	name     string
	packages PackageList
	isLoaded bool
}

func (c *Cycler) add(fileKey string, fileKeys []string) {
	c.packages = append(c.packages, Package{
		FileKey:   fileKey,
		FileKeys:  fileKeys,
		CreatedAt: time.Now(),
	})
}

func (c *Cycler) shiftByKeep(keep int) (first *Package) {
	total := len(c.packages)
	if total <= keep {
		return nil
	}

	first, c.packages = &c.packages[0], c.packages[1:]
	return
}

func (c *Cycler) run(fileKey string, fileKeys []string, keep int, deletePackage func(fileKey string) error) {
	logger := logger.Tag("Cycler")

	cyclerFileName := filepath.Join(cyclerPath, c.name+".json")

	c.load(cyclerFileName)
	c.add(fileKey, fileKeys)
	defer c.save(cyclerFileName)

	if keep == 0 {
		return
	}

	for {
		pkg := c.shiftByKeep(keep)
		if pkg == nil {
			break
		}

		fk := pkg.FileKey
		if len(pkg.FileKeys) != 0 && !strings.HasSuffix(fk, "/") {
			fk += "/"
		}
		for _, k := range append(pkg.FileKeys, fk) {
			// deletePackage() should handle directory case which has `/` suffix
			err := deletePackage(k)
			if err != nil {
				logger.Warnf("Remove %s failed: %v", k, err)
			} else {
				logger.Info("Removed", k)
			}
		}
	}
}

func (c *Cycler) load(cyclerFileName string) {
	logger := logger.Tag("Cycler")

	if err := helper.MkdirP(cyclerPath); err != nil {
		logger.Errorf("Failed to mkdir cycler path %s: %v", cyclerPath, err)
		return
	}

	// write example JSON if not exist
	if !helper.IsExistsPath(cyclerFileName) {
		if err := os.WriteFile(cyclerFileName, []byte("[]"), 0660); err != nil {
			logger.Errorf("Failed to write file %s: %v", cyclerFileName, err)
			return
		}
	}

	f, err := os.ReadFile(cyclerFileName)
	if err != nil {
		logger.Error("Load cycler.json failed:", err)
		return
	}
	err = json.Unmarshal(f, &c.packages)
	if err != nil {
		logger.Error("Unmarshal cycler.json failed:", err)
	}
	c.isLoaded = true
}

func (c *Cycler) save(cyclerFileName string) {
	logger := logger.Tag("Cycler")

	if !c.isLoaded {
		logger.Warn("Skip save cycler.json because it is not loaded")
		return
	}

	data, err := json.Marshal(&c.packages)
	if err != nil {
		logger.Error("Marshal packages to cycler.json failed: ", err)
		return
	}

	err = os.WriteFile(cyclerFileName, data, 0660)
	if err != nil {
		logger.Error("Save cycler.json failed: ", err)
		return
	}
}

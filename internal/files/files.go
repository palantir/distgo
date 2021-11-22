// Copyright 2021 Palantir Technologies, Inc.
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

package files

import (
	"os"
	"path/filepath"

	"github.com/nmiyake/pkg/gofiles"
)

// WriteGoFiles to the provided directory as the root directory.
func WriteGoFiles(dir string, files []gofiles.GoFileSpec) error {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}

	for _, currFile := range files {
		filePath := filepath.Join(dir, currFile.RelPath)
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(filePath, []byte(currFile.Src), 0644); err != nil {
			return err
		}
	}
	return nil
}

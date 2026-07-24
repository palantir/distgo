// Copyright 2016 Palantir Technologies, Inc.
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

package imports_test

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/nmiyake/pkg/dirs"
	"github.com/nmiyake/pkg/gofiles"
	"github.com/palantir/distgo/distgo/build/imports"
	"github.com/palantir/distgo/internal/files"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllFilesGoModOff(t *testing.T) {
	tmpDir, cleanup, err := dirs.TempDir(".", "")
	require.NoError(t, err)
	defer cleanup()
	err = os.WriteFile(path.Join(tmpDir, ".gitignore"), []byte(`*
*/
`), 0644)
	require.NoError(t, err)

	origModValue := os.Getenv("GO111MODULE")
	defer func() {
		require.NoError(t, os.Setenv("GO111MODULE", origModValue))
	}()
	require.NoError(t, os.Setenv("GO111MODULE", "off"))

	for i, currCase := range []struct {
		name    string
		pkgPath string
		filesFn func(projectDir string) []gofiles.GoFileSpec
		want    func(projectDir string) imports.BuildInputFiles
	}{
		{
			name:    "returns files for primary package",
			pkgPath: ".",
			filesFn: func(projectDir string) []gofiles.GoFileSpec {
				return []gofiles.GoFileSpec{
					{
						RelPath: "main.go",
						Src:     `package main; import "fmt"; func main() {}`,
					},
					{
						RelPath: "main_helper.go",
						Src:     `package main; func Helper() string { return "helper" }`,
					},
				}
			},
			want: func(projectDir string) imports.BuildInputFiles {
				absPkgDir, err := filepath.Abs(projectDir)
				require.NoError(t, err)
				return map[string][]string{
					path.Join("github.com/palantir/distgo/distgo/build/imports", projectDir): {
						path.Join(absPkgDir, "main.go"),
						path.Join(absPkgDir, "main_helper.go"),
					},
				}
			},
		},
		{
			name:    "test files are excluded",
			pkgPath: ".",
			filesFn: func(projectDir string) []gofiles.GoFileSpec {
				return []gofiles.GoFileSpec{
					{
						RelPath: "main.go",
						Src:     `package main; import "fmt"; func main() {}`,
					},
					{
						RelPath: "main_test.go",
						Src:     `package main; import "testing"; func TestMain(t *testing.T) {}`,
					},
					{
						RelPath: "another_test.go",
						Src:     `package main_test; import "testing"; func TestMain(t *testing.T) {}`,
					},
				}
			},
			want: func(projectDir string) imports.BuildInputFiles {
				absPkgDir, err := filepath.Abs(projectDir)
				require.NoError(t, err)
				return map[string][]string{
					path.Join("github.com/palantir/distgo/distgo/build/imports", projectDir): {
						path.Join(absPkgDir, "main.go"),
					},
				}
			},
		},
		{
			name:    "returns files for primary package and imported package",
			pkgPath: ".",
			filesFn: func(projectDir string) []gofiles.GoFileSpec {
				fooImportPath := path.Join("github.com/palantir/distgo/distgo/build/imports", projectDir, "foo")
				return []gofiles.GoFileSpec{
					{
						RelPath: "main.go",
						Src:     fmt.Sprintf(`package main; import "fmt"; import "%s"; func main() { fmt.Println(foo.Foo()) }`, fooImportPath),
					},
					{
						RelPath: "foo/foo.go",
						Src:     `package foo; func Foo() string { return "foo" }`,
					},
					{
						RelPath: "foo/foo_helper.go",
						Src:     `package foo`,
					},
				}
			},
			want: func(projectDir string) imports.BuildInputFiles {
				absPkgDir, err := filepath.Abs(projectDir)
				require.NoError(t, err)
				return map[string][]string{
					path.Join("github.com/palantir/distgo/distgo/build/imports", projectDir): {
						path.Join(absPkgDir, "main.go"),
					},
					path.Join("github.com/palantir/distgo/distgo/build/imports", projectDir, "foo"): {
						path.Join(absPkgDir, "foo", "foo.go"),
						path.Join(absPkgDir, "foo", "foo_helper.go"),
					},
				}
			},
		},
		{
			name:    "returns vendored dependency files",
			pkgPath: ".",
			filesFn: func(projectDir string) []gofiles.GoFileSpec {
				return []gofiles.GoFileSpec{
					{
						RelPath: "main.go",
						Src:     `package main; import "fmt"; import "github.com/foo"; func main() { fmt.Println(foo.Foo()) }`,
					},
					{
						RelPath: "vendor/github.com/foo/foo.go",
						Src:     `package foo; func Foo() string { return "foo" }`,
					},
					{
						RelPath: "vendor/github.com/foo/bar/bar.go",
						Src:     `package bar`,
					},
				}
			},
			want: func(projectDir string) imports.BuildInputFiles {
				absPkgDir, err := filepath.Abs(projectDir)
				require.NoError(t, err)
				return map[string][]string{
					path.Join("github.com/palantir/distgo/distgo/build/imports", projectDir): {
						path.Join(absPkgDir, "main.go"),
					},
					path.Join("github.com/palantir/distgo/distgo/build/imports", projectDir, "vendor/github.com/foo"): {
						path.Join(absPkgDir, "vendor/github.com/foo", "foo.go"),
					},
				}
			},
		},
	} {
		t.Run(currCase.name, func(t *testing.T) {
			currProjectDir, err := os.MkdirTemp(tmpDir, "")
			require.NoError(t, err, "Case %d", i)

			err = files.WriteGoFiles(currProjectDir, currCase.filesFn(currProjectDir))
			require.NoError(t, err, "Case %d", i)

			got, err := imports.AllFiles(currProjectDir, "", "")
			require.NoError(t, err, "Case %d", i)
			assert.Equal(t, currCase.want(currProjectDir), got, "Case %d", i)
		})
	}
}

func TestAllFilesGoModOn(t *testing.T) {
	tmpDir, cleanup, err := dirs.TempDir(".", "")
	require.NoError(t, err)
	defer cleanup()
	err = os.WriteFile(path.Join(tmpDir, ".gitignore"), []byte(`*
*/
`), 0644)
	require.NoError(t, err)

	origModValue := os.Getenv("GO111MODULE")
	defer func() {
		require.NoError(t, os.Setenv("GO111MODULE", origModValue))
	}()
	require.NoError(t, os.Setenv("GO111MODULE", "on"))

	origGoFlagValue := os.Getenv("GOFLAGS")
	defer func() {
		require.NoError(t, os.Setenv("GOFLAGS", origGoFlagValue))
	}()
	require.NoError(t, os.Setenv("GOFLAGS", "-mod=vendor"))

	for i, currCase := range []struct {
		name    string
		pkgPath string
		files   []gofiles.GoFileSpec
		want    func(projectDir string) imports.BuildInputFiles
	}{
		{
			name:    "returns files for primary package",
			pkgPath: ".",
			files: []gofiles.GoFileSpec{
				{
					RelPath: "go.mod",
					Src:     `module foo`,
				},
				{
					RelPath: "main.go",
					Src:     `package main; import "fmt"; func main() {}`,
				},
				{
					RelPath: "main_helper.go",
					Src:     `package main; func Helper() string { return "helper" }`,
				},
			},
			want: func(projectDir string) imports.BuildInputFiles {
				absPkgDir, err := filepath.Abs(projectDir)
				require.NoError(t, err)
				return map[string][]string{
					"foo": {
						path.Join(absPkgDir, "main.go"),
						path.Join(absPkgDir, "main_helper.go"),
					},
				}
			},
		},
		{
			name:    "test files are excluded",
			pkgPath: ".",
			files: []gofiles.GoFileSpec{
				{
					RelPath: "go.mod",
					Src:     `module foo`,
				},
				{
					RelPath: "main.go",
					Src:     `package main; import "fmt"; func main() {}`,
				},
				{
					RelPath: "main_test.go",
					Src:     `package main; import "testing"; func TestMain(t *testing.T) {}`,
				},
				{
					RelPath: "another_test.go",
					Src:     `package main_test; import "testing"; func TestMain(t *testing.T) {}`,
				},
			},
			want: func(projectDir string) imports.BuildInputFiles {
				absPkgDir, err := filepath.Abs(projectDir)
				require.NoError(t, err)
				return map[string][]string{
					"foo": {
						path.Join(absPkgDir, "main.go"),
					},
				}
			},
		},
		{
			name:    "returns files for primary package and imported package",
			pkgPath: ".",
			files: []gofiles.GoFileSpec{
				{
					RelPath: "go.mod",
					Src:     `module github.com/foo`,
				},
				{
					RelPath: "main.go",
					Src:     `package main; import "fmt"; import "github.com/foo/foo"; func main() { fmt.Println(foo.Foo()) }`,
				},
				{
					RelPath: "foo/foo.go",
					Src:     `package foo; func Foo() string { return "foo" }`,
				},
				{
					RelPath: "foo/foo_helper.go",
					Src:     `package foo`,
				},
			},
			want: func(projectDir string) imports.BuildInputFiles {
				absPkgDir, err := filepath.Abs(projectDir)
				require.NoError(t, err)
				return map[string][]string{
					"github.com/foo": {
						path.Join(absPkgDir, "main.go"),
					},
					"github.com/foo/foo": {
						path.Join(absPkgDir, "foo", "foo.go"),
						path.Join(absPkgDir, "foo", "foo_helper.go"),
					},
				}
			},
		},
		{
			name:    "returns vendored dependency files",
			pkgPath: ".",
			files: []gofiles.GoFileSpec{
				{
					RelPath: "go.mod",
					Src:     `module foo`,
				},
				{
					RelPath: "main.go",
					Src:     `package main; import "fmt"; import "github.com/foo"; func main() { fmt.Println(foo.Foo()) }`,
				},
				{
					RelPath: "vendor/github.com/foo/foo.go",
					Src:     `package foo; func Foo() string { return "foo" }`,
				},
				{
					RelPath: "vendor/github.com/foo/bar/bar.go",
					Src:     `package bar`,
				},
			},
			want: func(projectDir string) imports.BuildInputFiles {
				absPkgDir, err := filepath.Abs(projectDir)
				require.NoError(t, err)
				return map[string][]string{
					"foo": {
						path.Join(absPkgDir, "main.go"),
					},
					"github.com/foo": {
						path.Join(absPkgDir, "vendor/github.com/foo", "foo.go"),
					},
				}
			},
		},
		{
			name:    "embedded and other non-Go files are included",
			pkgPath: ".",
			files: []gofiles.GoFileSpec{
				{
					RelPath: "go.mod",
					Src:     `module foo`,
				},
				{
					RelPath: "main.go",
					Src: `package main

import (
	_ "embed"
	"fmt"
)

//go:embed assets/data.txt
var data string

func main() { fmt.Println(data) }`,
				},
				{
					RelPath: "assets/data.txt",
					Src:     "embedded content",
				},
			},
			want: func(projectDir string) imports.BuildInputFiles {
				absPkgDir, err := filepath.Abs(projectDir)
				require.NoError(t, err)
				return map[string][]string{
					"foo": {
						path.Join(absPkgDir, "main.go"),
						path.Join(absPkgDir, "assets", "data.txt"),
					},
				}
			},
		},
	} {
		t.Run(currCase.name, func(t *testing.T) {
			currProjectDir, err := os.MkdirTemp(tmpDir, "")
			require.NoError(t, err, "Case %d", i)

			err = files.WriteGoFiles(currProjectDir, currCase.files)
			require.NoError(t, err, "Case %d", i)

			got, err := imports.AllFiles(currProjectDir, "", "")
			require.NoError(t, err, "Case %d", i)
			assert.Equal(t, currCase.want(currProjectDir), got, "Case %d", i)
		})
	}
}

func TestNewerThanFileIsNewer(t *testing.T) {
	tmpDir, cleanup, err := dirs.TempDir(".", "")
	require.NoError(t, err)
	defer cleanup()
	err = os.WriteFile(path.Join(tmpDir, ".gitignore"), []byte(`*
*/
`), 0644)
	require.NoError(t, err)

	tmpFile, err := os.CreateTemp(tmpDir, "")
	require.NoError(t, err)
	fi, err := tmpFile.Stat()
	require.NoError(t, err)
	err = tmpFile.Close()
	require.NoError(t, err)

	// sleep for 1 second to ensure that mtimes differ
	time.Sleep(time.Second)

	err = os.WriteFile(path.Join(tmpDir, "main.go"), []byte(`package main; import "fmt"; func main() {}`), 0644)
	require.NoError(t, err)

	goFiles, err := imports.AllFiles(tmpDir, "", "")
	require.NoError(t, err)

	newer, err := goFiles.NewerThan(fi)
	require.NoError(t, err)
	assert.True(t, newer)
}

func TestNewerThanFileIsNotNewer(t *testing.T) {
	tmpDir, cleanup, err := dirs.TempDir(".", "")
	require.NoError(t, err)
	defer cleanup()
	err = os.WriteFile(path.Join(tmpDir, ".gitignore"), []byte(`*
*/
`), 0644)
	require.NoError(t, err)

	err = os.WriteFile(path.Join(tmpDir, "main.go"), []byte(`package main; import "fmt"; func main() {}`), 0644)
	require.NoError(t, err)

	goFiles, err := imports.AllFiles(tmpDir, "", "")
	require.NoError(t, err)

	// sleep for 1 second to ensure that mtimes differ
	time.Sleep(time.Second)

	tmpFile, err := os.CreateTemp(tmpDir, "")
	require.NoError(t, err)
	fi, err := tmpFile.Stat()
	require.NoError(t, err)
	err = tmpFile.Close()
	require.NoError(t, err)

	newer, err := goFiles.NewerThan(fi)
	require.NoError(t, err)
	assert.False(t, newer)
}

// TestNewerThanEmbedFileIsNewer verifies that a change to only a //go:embed'd asset (no Go file touched) is detected as
// newer than the build artifact.
func TestNewerThanEmbedFileIsNewer(t *testing.T) {
	tmpDir, cleanup, err := dirs.TempDir(".", "")
	require.NoError(t, err)
	defer cleanup()
	err = os.WriteFile(path.Join(tmpDir, ".gitignore"), []byte(`*
*/
`), 0644)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(path.Join(tmpDir, "go.mod"), []byte("module foo"), 0644))
	require.NoError(t, os.MkdirAll(path.Join(tmpDir, "assets"), 0755))
	require.NoError(t, os.WriteFile(path.Join(tmpDir, "assets", "data.txt"), []byte("v1"), 0644))
	require.NoError(t, os.WriteFile(path.Join(tmpDir, "main.go"), []byte(`package main

import (
	_ "embed"
	"fmt"
)

//go:embed assets/data.txt
var data string

func main() { fmt.Println(data) }`), 0644))

	buildFiles, err := imports.AllFiles(tmpDir, "", "")
	require.NoError(t, err)

	// sleep to ensure mtimes differ, then create a baseline file representing the already-built artifact. At this
	// point every input file is older than the artifact.
	time.Sleep(time.Second)
	tmpFile, err := os.CreateTemp(tmpDir, "")
	require.NoError(t, err)
	fi, err := tmpFile.Stat()
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	newer, err := buildFiles.NewerThan(fi)
	require.NoError(t, err)
	require.False(t, newer, "no input should be newer than the artifact before the embedded asset is modified")

	// modify only the embedded asset (not the Go source) and confirm it flips the result
	time.Sleep(time.Second)
	require.NoError(t, os.WriteFile(path.Join(tmpDir, "assets", "data.txt"), []byte("v2"), 0644))

	newer, err = buildFiles.NewerThan(fi)
	require.NoError(t, err)
	assert.True(t, newer)
}

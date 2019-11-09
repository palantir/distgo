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
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/nmiyake/pkg/dirs"
	"github.com/nmiyake/pkg/gofiles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/palantir/distgo/pkg/imports"
)

func TestAllFilesGoModOff(t *testing.T) {
	tmpDir, cleanup, err := dirs.TempDir(".", "")
	require.NoError(t, err)
	defer cleanup()
	err = ioutil.WriteFile(path.Join(tmpDir, ".gitignore"), []byte(`*
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
		files   []gofiles.GoFileSpec
		want    func(projectDir string) imports.GoFiles
	}{
		{
			name:    "returns files for primary package",
			pkgPath: ".",
			files: []gofiles.GoFileSpec{
				{
					RelPath: "main.go",
					Src:     `package main; import "fmt"; func main() {}`,
				},
				{
					RelPath: "main_helper.go",
					Src:     `package main; func Helper() string { return "helper" }`,
				},
			},
			want: func(projectDir string) imports.GoFiles {
				absPkgDir, err := filepath.Abs(projectDir)
				require.NoError(t, err)
				return map[string][]string{
					path.Join("github.com/palantir/distgo/pkg/imports", projectDir): {
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
			want: func(projectDir string) imports.GoFiles {
				absPkgDir, err := filepath.Abs(projectDir)
				require.NoError(t, err)
				return map[string][]string{
					path.Join("github.com/palantir/distgo/pkg/imports", projectDir): {
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
					RelPath: "main.go",
					Src:     `package main; import "fmt"; import "{{index . "foo/foo.go"}}"; func main() { fmt.Println(foo.Foo()) }`,
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
			want: func(projectDir string) imports.GoFiles {
				absPkgDir, err := filepath.Abs(projectDir)
				require.NoError(t, err)
				return map[string][]string{
					path.Join("github.com/palantir/distgo/pkg/imports", projectDir): {
						path.Join(absPkgDir, "main.go"),
					},
					path.Join("github.com/palantir/distgo/pkg/imports", projectDir, "foo"): {
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
			want: func(projectDir string) imports.GoFiles {
				absPkgDir, err := filepath.Abs(projectDir)
				require.NoError(t, err)
				return map[string][]string{
					path.Join("github.com/palantir/distgo/pkg/imports", projectDir): {
						path.Join(absPkgDir, "main.go"),
					},
					path.Join("github.com/palantir/distgo/pkg/imports", projectDir, "vendor/github.com/foo"): {
						path.Join(absPkgDir, "vendor/github.com/foo", "foo.go"),
					},
				}
			},
		},
	} {
		t.Run(currCase.name, func(t *testing.T) {
			currProjectDir, err := ioutil.TempDir(tmpDir, "")
			require.NoError(t, err, "Case %d", i)

			_, err = gofiles.Write(currProjectDir, currCase.files)
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
	err = ioutil.WriteFile(path.Join(tmpDir, ".gitignore"), []byte(`*
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
		want    func(projectDir string) imports.GoFiles
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
			want: func(projectDir string) imports.GoFiles {
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
			want: func(projectDir string) imports.GoFiles {
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
			want: func(projectDir string) imports.GoFiles {
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
			want: func(projectDir string) imports.GoFiles {
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
	} {
		t.Run(currCase.name, func(t *testing.T) {
			currProjectDir, err := ioutil.TempDir(tmpDir, "")
			require.NoError(t, err, "Case %d", i)

			_, err = gofiles.Write(currProjectDir, currCase.files)
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
	err = ioutil.WriteFile(path.Join(tmpDir, ".gitignore"), []byte(`*
*/
`), 0644)
	require.NoError(t, err)

	tmpFile, err := ioutil.TempFile(tmpDir, "")
	require.NoError(t, err)
	fi, err := tmpFile.Stat()
	require.NoError(t, err)
	err = tmpFile.Close()
	require.NoError(t, err)

	// sleep for 1 second to ensure that mtimes differ
	time.Sleep(time.Second)

	err = ioutil.WriteFile(path.Join(tmpDir, "main.go"), []byte(`package main; import "fmt"; func main() {}`), 0644)
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
	err = ioutil.WriteFile(path.Join(tmpDir, ".gitignore"), []byte(`*
*/
`), 0644)
	require.NoError(t, err)

	err = ioutil.WriteFile(path.Join(tmpDir, "main.go"), []byte(`package main; import "fmt"; func main() {}`), 0644)
	require.NoError(t, err)

	goFiles, err := imports.AllFiles(tmpDir, "", "")
	require.NoError(t, err)

	// sleep for 1 second to ensure that mtimes differ
	time.Sleep(time.Second)

	tmpFile, err := ioutil.TempFile(tmpDir, "")
	require.NoError(t, err)
	fi, err := tmpFile.Stat()
	require.NoError(t, err)
	err = tmpFile.Close()
	require.NoError(t, err)

	newer, err := goFiles.NewerThan(fi)
	require.NoError(t, err)
	assert.False(t, newer)
}

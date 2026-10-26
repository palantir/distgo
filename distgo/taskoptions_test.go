// Copyright 2026 Palantir Technologies, Inc.
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

package distgo_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/distgo/dist"
	"github.com/palantir/distgo/distgo/docker"
	"github.com/palantir/distgo/distgo/publish"
	"github.com/palantir/godel/v2/pkg/osarch"
	"github.com/stretchr/testify/require"
)

type taskFixture struct {
	info        distgo.ProjectInfo
	project     distgo.ProjectParam
	configTime  time.Time
	source      string
	binary      string
	archive     string
	workDir     string
	distRuns    int
	dockerRuns  int
	publishRuns int
}

func newTaskFixture(t *testing.T) *taskFixture {
	t.Helper()
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOTOOLCHAIN", "local")
	f := &taskFixture{info: distgo.ProjectInfo{ProjectDir: t.TempDir(), Version: "1.0.0"}}
	base := time.Now().Truncate(time.Second).Add(-time.Hour)
	f.configTime = base
	f.source = filepath.Join(f.info.ProjectDir, "foo", "main.go")
	writeTaskFile(t, filepath.Join(f.info.ProjectDir, "go.mod"), "module example.com/fixture\n\ngo 1.21\n", base)
	writeTaskFile(t, f.source, "package main\nfunc main() {}\n", base)
	writeTaskFile(t, filepath.Join(f.info.ProjectDir, "docker", "Dockerfile"), "FROM scratch\n", base)
	product := distgo.ProductParam{
		ID: "foo", Name: "foo",
		Build: &distgo.BuildParam{
			NameTemplate: "{{Product}}", OutputDir: "out/build", MainPkg: "./foo",
			OSArchs: []osarch.OSArch{osarch.Current()},
			Script:  "#!/bin/sh\ntouch build-hook\n",
		},
		Dist: &distgo.DistParam{
			OutputDir: "out/dist",
			DistParams: map[distgo.DistID]distgo.DisterParam{
				"tgz": {NameTemplate: "{{Product}}-{{Version}}", Dister: f,
					Script: "#!/bin/sh\ntouch dist-hook\n"},
			},
		},
		Docker: &distgo.DockerParam{
			OutputDir: "out/docker",
			DockerBuilderParams: map[distgo.DockerID]distgo.DockerBuilderParam{
				"image": {
					DockerBuilder: f, ContextDir: "docker", DockerfilePath: "Dockerfile",
					InputProductsDir: "inputs",
					InputBuilds:      []distgo.ProductBuildID{distgo.ProductBuildID("foo." + osarch.Current().String())},
					InputDists:       []distgo.ProductDistID{"foo.tgz"},
					TagTemplates: distgo.TagTemplatesMap{
						Templates:   map[distgo.DockerTagID]string{"test": "{{Product}}:{{Version}}"},
						OrderedKeys: []distgo.DockerTagID{"test"},
					},
					Script: "#!/bin/sh\ntouch docker-hook\n",
				},
			},
		},
	}
	f.project.Products = map[distgo.ProductID]distgo.ProductParam{"foo": product}
	info, err := distgo.ToProductTaskOutputInfo(f.info, product)
	require.NoError(t, err)
	f.binary = info.ProductBuildArtifactPaths()[osarch.Current()]
	f.archive = info.ProductDistArtifactPaths()["tgz"][0]
	f.workDir = info.ProductDistWorkDirs()["tgz"]
	writeTaskFile(t, f.binary, "binary", base.Add(time.Minute))
	writeTaskFile(t, f.archive, "archive", base.Add(2*time.Minute))
	require.NoError(t, os.MkdirAll(f.workDir, 0755))
	require.NoError(t, os.Chtimes(f.workDir, base.Add(2*time.Minute), base.Add(2*time.Minute)))
	return f
}

func writeTaskFile(t *testing.T, name, content string, modTime time.Time) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(name), 0755))
	require.NoError(t, os.WriteFile(name, []byte(content), 0644))
	require.NoError(t, os.Chtimes(name, modTime, modTime))
}

func (f *taskFixture) TypeName() (string, error)               { return "fixture", nil }
func (f *taskFixture) PackagingExtension() (string, error)     { return "tgz", nil }
func (f *taskFixture) Artifacts(name string) ([]string, error) { return []string{name + ".tgz"}, nil }
func (f *taskFixture) RunDist(distgo.DistID, distgo.ProductTaskOutputInfo) ([]byte, error) {
	f.distRuns++
	return nil, nil
}
func (f *taskFixture) GenerateDistArtifacts(id distgo.DistID, info distgo.ProductTaskOutputInfo, _ []byte) error {
	for _, output := range info.ProductDistArtifactPaths()[id] {
		if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(output, []byte("generated archive"), 0644); err != nil {
			return err
		}
	}
	return nil
}
func (f *taskFixture) RunDockerBuild(distgo.DockerID, distgo.ProductTaskOutputInfo, bool, bool, io.Writer) error {
	f.dockerRuns++
	return nil
}
func (f *taskFixture) Flags() ([]distgo.PublisherFlag, error) { return nil, nil }
func (f *taskFixture) RunPublish([]distgo.ProductPublishInfo, map[distgo.PublisherFlagName]any, bool, io.Writer) error {
	f.publishRuns++
	return nil
}

func (f *taskFixture) run(operation string, dryRun bool) error {
	opts := distgo.TaskOptions{NoDependentTasks: true}
	configTime := &f.configTime
	if f.configTime.IsZero() {
		configTime = nil
	}
	switch operation {
	case "dist":
		// A nil config timestamp forces the requested dist action while still requiring current build inputs.
		return dist.ProductsWithOptions(f.info, f.project, nil, []distgo.ProductDistID{"foo.tgz"}, dryRun, true, io.Discard, opts)
	case "docker":
		return docker.BuildProductsWithOptions(f.info, f.project, configTime, []distgo.ProductDockerID{"foo.image"}, nil, false, dryRun, io.Discard, opts)
	default:
		return publish.ProductsWithOptions(f.info, f.project, configTime, []distgo.ProductDistID{"foo.tgz"}, f, nil, dryRun, io.Discard, opts)
	}
}

func TestNoDependentTasksRejectsPrerequisites(t *testing.T) {
	for _, operation := range []string{"dist", "docker", "publish"} {
		t.Run(operation, func(t *testing.T) {
			for _, dryRun := range []bool{false, true} {
				name := "execute"
				if dryRun {
					name = "dry-run"
				}
				t.Run(name, func(t *testing.T) {
					for _, scenario := range []string{"missing build", "stale build", "unverifiable build", "missing dist", "missing work directory", "stale configuration", "unknown configuration", "transitive build"} {
						if operation == "dist" && (strings.Contains(scenario, "dist") || strings.Contains(scenario, "configuration") || scenario == "missing work directory") {
							continue
						}
						t.Run(scenario, func(t *testing.T) {
							f := newTaskFixture(t)
							switch scenario {
							case "missing build":
								require.NoError(t, os.Remove(f.binary))
							case "stale build":
								future := time.Now().Add(time.Minute)
								require.NoError(t, os.Chtimes(f.source, future, future))
							case "unverifiable build":
								require.NoError(t, os.Remove(f.source))
							case "missing dist":
								require.NoError(t, os.Remove(f.archive))
							case "missing work directory":
								require.NoError(t, os.RemoveAll(f.workDir))
							case "stale configuration":
								f.configTime = time.Now().Add(time.Minute)
							case "unknown configuration":
								f.configTime = time.Time{}
							case "transitive build":
								foo := f.project.Products["foo"]
								dep := foo
								dep.ID, dep.Name, dep.Docker = "bar", "bar", nil
								buildCopy := *dep.Build
								buildCopy.MainPkg = "./bar"
								dep.Build = &buildCopy
								foo.FirstLevelDependencies = []distgo.ProductID{"bar"}
								foo.AllDependencies = map[distgo.ProductID]distgo.ProductParam{"bar": dep}
								f.project.Products["foo"], f.project.Products["bar"] = foo, dep
							}
							err := f.run(operation, dryRun)
							require.ErrorContains(t, err, "--no-dependent-tasks")
							require.Zero(t, f.distRuns)
							require.Zero(t, f.dockerRuns)
							require.Zero(t, f.publishRuns)
							for _, hook := range []string{"build-hook", "dist-hook", "docker-hook"} {
								_, err := os.Stat(filepath.Join(f.info.ProjectDir, hook))
								require.True(t, os.IsNotExist(err), "hook %s executed", hook)
							}
						})
					}
				})
			}
		})
	}
}

func TestNoDependentTasksRunsRequestedStage(t *testing.T) {
	for _, operation := range []string{"dist", "docker", "publish"} {
		t.Run(operation, func(t *testing.T) {
			f := newTaskFixture(t)
			before, err := os.Stat(f.binary)
			require.NoError(t, err)
			require.NoError(t, f.run(operation, false))
			after, err := os.Stat(f.binary)
			require.NoError(t, err)
			require.Equal(t, before.ModTime(), after.ModTime())
			if operation == "dist" {
				require.Equal(t, 1, f.distRuns)
			} else {
				require.Zero(t, f.distRuns)
			}
			if operation == "docker" {
				require.Equal(t, 1, f.dockerRuns)
			} else {
				require.Zero(t, f.dockerRuns)
			}
			if operation == "publish" {
				require.Equal(t, 1, f.publishRuns)
			} else {
				require.Zero(t, f.publishRuns)
			}
			_, err = os.Stat(filepath.Join(f.info.ProjectDir, "build-hook"))
			require.True(t, os.IsNotExist(err))
		})
	}
}

func TestNoDependentTasksRespectsSelection(t *testing.T) {
	f := newTaskFixture(t)
	foo := f.project.Products["foo"]
	unused := foo
	unused.ID, unused.Name = "unused", "unused"
	f.project.Products["unused"] = unused
	// The selected distribution exists; this additional, unselected configuration does not.
	foo.Dist.DistParams["unused"] = distgo.DisterParam{NameTemplate: "missing", Dister: f}
	f.project.Products["foo"] = foo
	require.NoError(t, f.run("dist", false))
	require.NoError(t, f.run("publish", false))
}

func TestNoDependentTasksChecksDockerBuildWithoutDist(t *testing.T) {
	f := newTaskFixture(t)
	foo := f.project.Products["foo"]
	foo.Dist = nil
	f.project.Products["foo"] = foo
	require.NoError(t, os.Remove(f.binary))
	require.ErrorContains(t, f.run("docker", false), "build required for foo")
	require.Zero(t, f.dockerRuns)
}

func TestNoDependentTasksChecksEveryBuildTarget(t *testing.T) {
	f := newTaskFixture(t)
	target, err := osarch.New("linux-arm64")
	require.NoError(t, err)
	if target == osarch.Current() {
		target, err = osarch.New("linux-amd64")
		require.NoError(t, err)
	}
	f.project.Products["foo"].Build.OSArchs = append(f.project.Products["foo"].Build.OSArchs, target)
	require.ErrorContains(t, f.run("dist", false), target.String())
	require.Zero(t, f.distRuns)
}

func TestRequiredTasksErrorIsStable(t *testing.T) {
	a := distgo.RequiredTask{Task: "dist", Product: "z", Target: "tgz", Outputs: []string{"z", "a"}, Reason: "stale"}
	b := distgo.RequiredTask{Task: "build", Product: "a", Target: "linux-amd64", Outputs: []string{"binary"}, Reason: "missing"}
	first := distgo.RequiredTasksError("publish", []distgo.RequiredTask{a, b, a})
	second := distgo.RequiredTasksError("publish", []distgo.RequiredTask{b, a})
	require.EqualError(t, first, second.Error())
	require.Equal(t, []string{"z", "a"}, a.Outputs)
	require.Nil(t, distgo.RequiredTasksError("publish", nil))
}

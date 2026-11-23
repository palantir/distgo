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

package testfuncs

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/palantir/distgo/dister/osarchbin"
	"github.com/palantir/distgo/distgo"
	"github.com/palantir/godel/v2/pkg/osarch"
	"github.com/stretchr/testify/require"
)

// TaskOptionsDistID is the dist ID of the product created by NewTaskOptionsProject.
const TaskOptionsDistID distgo.DistID = "tgz"

// TaskOptionsProject is a project with a product "foo" whose build and dist outputs are current.
type TaskOptionsProject struct {
	Info       distgo.ProjectInfo
	Project    distgo.ProjectParam
	ConfigTime time.Time
	Source     string
	Binary     string
	Archive    string
	WorkDir    string
}

// NewTaskOptionsProject creates a TaskOptionsProject, adding a Docker image "image" if dockerBuilder is non-nil.
func NewTaskOptionsProject(t *testing.T, dockerBuilder distgo.DockerBuilder) *TaskOptionsProject {
	t.Helper()
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOTOOLCHAIN", "local")
	p := &TaskOptionsProject{Info: distgo.ProjectInfo{ProjectDir: t.TempDir(), Version: "1.0.0"}}
	base := time.Unix(1_000_000_000, 0)
	p.ConfigTime = base
	p.Source = filepath.Join(p.Info.ProjectDir, "foo", "main.go")
	WriteFileWithModTime(t, filepath.Join(p.Info.ProjectDir, "go.mod"), "module example.com/fixture\n\ngo 1.21\n", base)
	WriteFileWithModTime(t, p.Source, "package main\nfunc main() {}\n", base)
	WriteFileWithModTime(t, filepath.Join(p.Info.ProjectDir, "docker", "Dockerfile"), "FROM scratch\n", base)
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
				TaskOptionsDistID: {NameTemplate: "{{Product}}-{{Version}}", Dister: osarchbin.New(osarch.Current()),
					Script: "#!/bin/sh\ntouch dist-hook\n"},
			},
		},
	}
	if dockerBuilder != nil {
		product.Docker = &distgo.DockerParam{
			OutputDir: "out/docker",
			DockerBuilderParams: map[distgo.DockerID]distgo.DockerBuilderParam{
				"image": {
					DockerBuilder: dockerBuilder, ContextDir: "docker", DockerfilePath: "Dockerfile",
					InputProductsDir: "inputs",
					InputBuilds:      []distgo.ProductBuildID{distgo.ProductBuildID("foo." + osarch.Current().String())},
					InputDists:       []distgo.ProductDistID{distgo.ProductDistID("foo." + string(TaskOptionsDistID))},
					TagTemplates: distgo.TagTemplatesMap{
						Templates:   map[distgo.DockerTagID]string{"test": "{{Product}}:{{Version}}"},
						OrderedKeys: []distgo.DockerTagID{"test"},
					},
					Script: "#!/bin/sh\ntouch docker-hook\n",
				},
			},
		}
	}
	p.Project.Products = map[distgo.ProductID]distgo.ProductParam{"foo": product}
	p.Binary, p.Archive, p.WorkDir = p.writeOutputs(t, product)
	return p
}

// ConfigModTime returns the configuration modification time, or nil if it is unknown.
func (p *TaskOptionsProject) ConfigModTime() *time.Time {
	if p.ConfigTime.IsZero() {
		return nil
	}
	return &p.ConfigTime
}

// AddDependency adds a product "bar" with current outputs as a dependency of "foo" and returns its output paths.
func (p *TaskOptionsProject) AddDependency(t *testing.T) (binary, archive, workDir string) {
	t.Helper()
	foo := p.Project.Products["foo"]
	dep := foo
	dep.ID, dep.Name, dep.Docker = "bar", "bar", nil
	foo.FirstLevelDependencies = []distgo.ProductID{"bar"}
	foo.AllDependencies = map[distgo.ProductID]distgo.ProductParam{"bar": dep}
	p.Project.Products["foo"], p.Project.Products["bar"] = foo, dep
	return p.writeOutputs(t, dep)
}

// RequireNoHooks fails the test if any build, dist, or Docker hook script ran.
func (p *TaskOptionsProject) RequireNoHooks(t *testing.T) {
	t.Helper()
	for _, hook := range []string{"build-hook", "dist-hook", "docker-hook"} {
		require.NoFileExists(t, filepath.Join(p.Info.ProjectDir, hook))
	}
}

func (p *TaskOptionsProject) writeOutputs(t *testing.T, product distgo.ProductParam) (binary, archive, workDir string) {
	t.Helper()
	info, err := distgo.ToProductTaskOutputInfo(p.Info, product)
	require.NoError(t, err)
	binary = info.ProductBuildArtifactPaths()[osarch.Current()]
	archive = info.ProductDistArtifactPaths()[TaskOptionsDistID][0]
	workDir = info.ProductDistWorkDirs()[TaskOptionsDistID]
	distTime := p.ConfigTime.Add(2 * time.Minute)
	WriteFileWithModTime(t, binary, "binary", p.ConfigTime.Add(time.Minute))
	WriteFileWithModTime(t, archive, "archive", distTime)
	require.NoError(t, os.MkdirAll(workDir, 0755))
	require.NoError(t, os.Chtimes(workDir, distTime, distTime))
	return binary, archive, workDir
}

// TaskOptionsScenario modifies a TaskOptionsProject so that a prerequisite of the requested stage must run.
type TaskOptionsScenario struct {
	Name string
	// BuildOnly is true if the scenario only affects build outputs, so it also applies to dist.
	BuildOnly bool
	// Reason is a substring of the expected error, if any.
	Reason  string
	Prepare func(t *testing.T, p *TaskOptionsProject)
}

// PrerequisiteScenarios returns scenarios in which a build or dist prerequisite is missing, stale, or unverifiable.
func PrerequisiteScenarios() []TaskOptionsScenario {
	return []TaskOptionsScenario{
		{Name: "missing build", BuildOnly: true, Prepare: func(t *testing.T, p *TaskOptionsProject) {
			require.NoError(t, os.Remove(p.Binary))
		}},
		{Name: "stale build", BuildOnly: true, Prepare: func(t *testing.T, p *TaskOptionsProject) {
			future := p.ConfigTime.Add(3 * time.Minute)
			require.NoError(t, os.Chtimes(p.Source, future, future))
		}},
		{Name: "unverifiable build", BuildOnly: true, Prepare: func(t *testing.T, p *TaskOptionsProject) {
			require.NoError(t, os.Remove(p.Source))
		}},
		{Name: "transitive build", BuildOnly: true, Prepare: func(t *testing.T, p *TaskOptionsProject) {
			foo := p.Project.Products["foo"]
			dep := foo
			dep.ID, dep.Name, dep.Docker = "bar", "bar", nil
			buildCopy := *dep.Build
			buildCopy.MainPkg = "./bar"
			dep.Build = &buildCopy
			foo.FirstLevelDependencies = []distgo.ProductID{"bar"}
			foo.AllDependencies = map[distgo.ProductID]distgo.ProductParam{"bar": dep}
			p.Project.Products["foo"], p.Project.Products["bar"] = foo, dep
		}},
		{Name: "missing dist", Prepare: func(t *testing.T, p *TaskOptionsProject) {
			require.NoError(t, os.Remove(p.Archive))
		}},
		{Name: "artifact is directory", Prepare: func(t *testing.T, p *TaskOptionsProject) {
			require.NoError(t, os.Remove(p.Archive))
			require.NoError(t, os.Mkdir(p.Archive, 0755))
		}},
		{Name: "missing work directory", Prepare: func(t *testing.T, p *TaskOptionsProject) {
			require.NoError(t, os.RemoveAll(p.WorkDir))
		}},
		{Name: "stale configuration", Prepare: func(t *testing.T, p *TaskOptionsProject) {
			p.ConfigTime = p.ConfigTime.Add(3 * time.Minute)
		}},
		{Name: "unknown configuration", Prepare: func(t *testing.T, p *TaskOptionsProject) {
			p.ConfigTime = time.Time{}
		}},
	}
}

// StaleDistributionScenarios returns scenarios in which dist outputs exist but are stale.
func StaleDistributionScenarios() []TaskOptionsScenario {
	return []TaskOptionsScenario{
		{Name: "build newer than dist", Reason: "distribution is older than its build outputs", Prepare: func(t *testing.T, p *TaskOptionsProject) {
			newer := p.ConfigTime.Add(3 * time.Minute)
			require.NoError(t, os.Chtimes(p.Binary, newer, newer))
		}},
		{Name: "config in same second as dist", Reason: "distribution is not newer than its configuration", Prepare: func(t *testing.T, p *TaskOptionsProject) {
			p.ConfigTime = p.ConfigTime.Add(2*time.Minute + time.Nanosecond)
		}},
		{Name: "dependency newer than dist", Reason: "distribution is older than dependency bar", Prepare: func(t *testing.T, p *TaskOptionsProject) {
			_, archive, _ := p.AddDependency(t)
			newer := p.ConfigTime.Add(3 * time.Minute)
			require.NoError(t, os.Chtimes(archive, newer, newer))
		}},
		{Name: "dependency dist missing", Reason: "dist required for bar", Prepare: func(t *testing.T, p *TaskOptionsProject) {
			_, archive, _ := p.AddDependency(t)
			require.NoError(t, os.Remove(archive))
		}},
	}
}

// RestoreTimestamps adds a dependency and touches all outputs to one fixed time after the configuration, as CI does
// after restoring artifacts.
func (p *TaskOptionsProject) RestoreTimestamps(t *testing.T) {
	t.Helper()
	binary, archive, workDir := p.AddDependency(t)
	p.ConfigTime = time.Now().Truncate(time.Second)
	restored := p.ConfigTime.Add(2 * time.Second)
	require.NoError(t, os.Chtimes(p.Source, p.ConfigTime, p.ConfigTime))
	for _, output := range []string{p.Binary, p.Archive, p.WorkDir, binary, archive, workDir} {
		require.NoError(t, os.Chtimes(output, restored, restored))
	}
}

// WriteFileWithModTime writes content to name, creating parent directories, and sets its modification time.
func WriteFileWithModTime(t *testing.T, name, content string, modTime time.Time) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(name), 0755))
	require.NoError(t, os.WriteFile(name, []byte(content), 0644))
	require.NoError(t, os.Chtimes(name, modTime, modTime))
}

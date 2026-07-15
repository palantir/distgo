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

package defaultdockerbuilder

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/palantir/distgo/distgo"
)

// dependencyImageBuildContextArgs returns "--build-context <renderedTag>=oci-layout://<layout>" args for each declared
// dependency Docker image, so a "FROM <dep tag>" resolves from the dependency's on-disk OCI layout instead of a
// registry. The layout is the wrapper written by the Docker build task after a dependency builds (see
// distgo.WriteDockerBuildContextLayout). buildx validates every --build-context eagerly, so outside of dry runs an
// entry is emitted only when the layout exists on disk: dependencies build before dependents, so an OCI-producing
// dependency's layout is present, while a daemon-only dependency produces none and is skipped.
func dependencyImageBuildContextArgs(productTaskOutputInfo distgo.ProductTaskOutputInfo, dryRun bool) []string {
	depIDs := make([]string, 0, len(productTaskOutputInfo.Deps))
	for depID := range productTaskOutputInfo.Deps {
		depIDs = append(depIDs, string(depID))
	}
	sort.Strings(depIDs)

	var args []string
	for _, depID := range depIDs {
		depOutputInfo := productTaskOutputInfo.Deps[distgo.ProductID(depID)]
		if depOutputInfo.DockerOutputInfos == nil {
			continue
		}
		for _, dockerID := range depOutputInfo.DockerOutputInfos.DockerIDs {
			builderOutputInfo, ok := depOutputInfo.DockerOutputInfos.DockerBuilderOutputInfos[dockerID]
			if !ok || len(builderOutputInfo.RenderedTags) == 0 {
				continue
			}
			ociDir := distgo.ProductDistOutputDir(productTaskOutputInfo.Project, depOutputInfo, distgo.DistID(fmt.Sprintf("oci-%s", dockerID)))
			if ociDir == "" {
				continue
			}
			layoutDir := filepath.Join(ociDir, distgo.DockerBuildContextLayoutSubdir)
			if !dryRun {
				if _, err := os.Stat(filepath.Join(layoutDir, "index.json")); err != nil {
					continue
				}
			}
			for _, tag := range builderOutputInfo.RenderedTags {
				args = append(args, "--build-context", fmt.Sprintf("%s=oci-layout://%s", tag, layoutDir))
			}
		}
	}
	return args
}

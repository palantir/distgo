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

package dist

import (
	"maps"
	"os"
	"slices"
	"time"

	"github.com/palantir/distgo/distgo"
	"github.com/pkg/errors"
)

// RequiresDist returns a pointer to a distgo.ProductParam that contains only the Dister parameters for the output dist
// artifacts that require generation. A product is considered to require generating dist artifacts if any of the
// following is true:
//   - Any of the dist artifact output paths do not exist
//   - The product's dist configuration (as specified by configModTime) is more recent than any of its dist artifacts
//   - The product has dependencies and any of the dependent build or dist artifacts are newer (have a later
//     modification date) than any of the dist artifacts for the provided product
//   - The product does not define a dist configuration
//
// Returns nil if all of the outputs exist and are up-to-date.
func RequiresDist(projectInfo distgo.ProjectInfo, productParam distgo.ProductParam, configModTime *time.Time) (*distgo.ProductParam, error) {
	if productParam.Dist == nil {
		return nil, nil
	}

	// create a copy of the dist parameter so that it is safe for modification
	distCopy := *productParam.Dist
	productParam.Dist = &distCopy

	productTaskOutputInfo, err := distgo.ToProductTaskOutputInfo(projectInfo, productParam)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to compute output information for %s", productParam.ID)
	}

	requiresDistIDs := make(map[distgo.DistID]struct{})
	for _, currDistID := range productTaskOutputInfo.Product.DistOutputInfos.DistIDs {
		if !disterRequiresDist(currDistID, productTaskOutputInfo, configModTime) {
			continue
		}
		requiresDistIDs[currDistID] = struct{}{}
	}

	if len(requiresDistIDs) == 0 {
		return nil, nil
	}
	requiresDistParams := make(map[distgo.DistID]distgo.DisterParam)
	for distID, distParam := range productParam.Dist.DistParams {
		if _, ok := requiresDistIDs[distID]; !ok {
			continue
		}
		requiresDistParams[distID] = distParam
	}
	productParam.Dist.DistParams = requiresDistParams
	return &productParam, nil
}

func disterRequiresDist(distID distgo.DistID, productTaskOutputInfo distgo.ProductTaskOutputInfo, configModTime *time.Time) bool {
	return checkDistCurrent(distID, productTaskOutputInfo, configModTime, false) != nil
}

// checkDistCurrent returns nil if the distribution is current, or an error describing why it must be regenerated.
func checkDistCurrent(distID distgo.DistID, productTaskOutputInfo distgo.ProductTaskOutputInfo, configModTime *time.Time, checked bool) error {
	// determine oldest dist artifact for current Dister. If any artifact is missing, Dister needs to be run.
	oldestDistTime := time.Now()
	if checked {
		// Restored artifacts may be touched ahead of checkout; do not cap their timestamps at the current time.
		oldestDistTime = time.Time{}
		if err := checkDistOutputShape(distID, productTaskOutputInfo); err != nil {
			return err
		}
	}
	for _, currArtifactPath := range productTaskOutputInfo.ProductDistWorkDirsAndArtifactPaths()[distID] {
		fi, err := os.Stat(currArtifactPath)
		if err != nil {
			return errors.Wrapf(err, "cannot inspect output %s", currArtifactPath)
		}
		if fiModTime := fi.ModTime(); (checked && oldestDistTime.IsZero()) || fiModTime.Before(oldestDistTime) {
			oldestDistTime = fiModTime
		}
	}

	// if the newest build artifact is more recent than the oldest dist, consider dist out-of-date
	if newestBuildArtifactForDist := newestArtifactModTime(buildArtifactPaths(productTaskOutputInfo.Project, productTaskOutputInfo.Product)); newestBuildArtifactForDist != nil && newestBuildArtifactForDist.Truncate(time.Second).After(oldestDistTime.Truncate(time.Second)) {
		return errors.New("distribution is older than its build outputs")
	}

	// if the configuration modification time was not provided or was modified more recently than the oldest dist
	// artifact, consider it out-of-date. Truncate times to second granularity for purposes of comparison. If mod time
	// and artifact generation time are the same, consider out-of-date and run.
	if configModTime == nil {
		return errors.New("configuration modification time is unavailable")
	}
	if !configModTime.Truncate(time.Second).Before(oldestDistTime.Truncate(time.Second)) {
		return errors.New("distribution is not newer than its configuration")
	}

	// if any dependent artifact (build or dist) is newer than the oldest dist artifact, consider dist artifact out-of-date
	for _, id := range slices.Sorted(maps.Keys(productTaskOutputInfo.Deps)) {
		depProductOutputInfo := productTaskOutputInfo.Deps[id]
		newestDependencyTime := newestArtifactModTime(append(
			buildArtifactPaths(productTaskOutputInfo.Project, depProductOutputInfo),
			distArtifactPaths(productTaskOutputInfo.Project, distID, depProductOutputInfo)...,
		))
		if newestDependencyTime != nil && newestDependencyTime.After(oldestDistTime) {
			return errors.Errorf("distribution is older than dependency %s", depProductOutputInfo.ID)
		}
	}
	return nil
}

// checkDistOutputShape returns nil if the dist work path is a directory and every artifact is a regular
// file. An extensionless manual distribution uses the same path for both, so a regular file is accepted there.
func checkDistOutputShape(distID distgo.DistID, productTaskOutputInfo distgo.ProductTaskOutputInfo) error {
	workDir := productTaskOutputInfo.ProductDistWorkDirs()[distID]
	artifactPaths := productTaskOutputInfo.ProductDistArtifactPaths()[distID]
	fi, err := os.Stat(workDir)
	if err != nil {
		return errors.Wrapf(err, "cannot inspect output %s", workDir)
	}
	if !fi.IsDir() && !(fi.Mode().IsRegular() && slices.Contains(artifactPaths, workDir)) {
		return errors.Errorf("distribution work path is not a directory: %s", workDir)
	}
	for _, artifactPath := range artifactPaths {
		fi, err := os.Stat(artifactPath)
		if err != nil {
			return errors.Wrapf(err, "cannot inspect output %s", artifactPath)
		}
		if !fi.Mode().IsRegular() {
			return errors.Errorf("distribution artifact is not a regular file: %s", artifactPath)
		}
	}
	return nil
}

func buildArtifactPaths(projectInfo distgo.ProjectInfo, productInfo distgo.ProductOutputInfo) []string {
	if productInfo.BuildOutputInfo == nil {
		return nil
	}
	var artifacts []string
	for _, v := range distgo.ProductBuildArtifactPaths(projectInfo, productInfo) {
		artifacts = append(artifacts, v)
	}
	return artifacts
}

func distArtifactPaths(projectInfo distgo.ProjectInfo, distID distgo.DistID, productInfo distgo.ProductOutputInfo) []string {
	if productInfo.DistOutputInfos == nil {
		return nil
	}
	var artifacts []string
	for _, v := range distgo.ProductDistWorkDirsAndArtifactPaths(projectInfo, productInfo)[distID] {
		artifacts = append(artifacts, v)
	}
	return artifacts
}

func newestArtifactModTime(artifacts []string) *time.Time {
	var newestModTime *time.Time
	newestModTimeFn := func(currPath string) {
		fi, err := os.Stat(currPath)
		if err != nil {
			return
		}
		if fiModTime := fi.ModTime(); newestModTime == nil || fiModTime.After(*newestModTime) {
			newestModTime = &fiModTime
		}
	}
	for _, v := range artifacts {
		newestModTimeFn(v)
	}
	return newestModTime
}

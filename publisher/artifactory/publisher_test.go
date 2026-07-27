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

package artifactory_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/publisher/artifactory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunPublish_UsesEachInputsOwnConfig(t *testing.T) {
	inputs := []distgo.ProductPublishInfo{
		testProductInput("foo", "fooRepo"),
		testProductInput("bar", "barRepo"),
	}

	publisher := artifactory.PublisherCreator().Publisher()
	var stdout bytes.Buffer
	err := publisher.RunPublish(inputs, nil, true, &stdout)
	require.NoError(t, err)

	assert.Contains(t, stdout.String(), "http://artifactory.domain.com/artifactory/fooRepo/com/test/group/foo/1.0.0/foo-1.0.0-linux-amd64.tgz")
	assert.Contains(t, stdout.String(), "http://artifactory.domain.com/artifactory/barRepo/com/test/group/bar/1.0.0/bar-1.0.0-linux-amd64.tgz")
	assert.NotContains(t, stdout.String(), "http://artifactory.domain.com/artifactory/fooRepo/com/test/group/bar")
	assert.NotContains(t, stdout.String(), "http://artifactory.domain.com/artifactory/barRepo/com/test/group/foo")
}

func TestRunPublish_StopsOnFirstError(t *testing.T) {
	badInput := testProductInput("foo", "fooRepo")
	badInput.PublisherConfigYML = []byte("no-pom: true\n") // missing required "url" and "repository"
	goodInput := testProductInput("bar", "barRepo")

	publisher := artifactory.PublisherCreator().Publisher()
	var stdout bytes.Buffer
	err := publisher.RunPublish([]distgo.ProductPublishInfo{badInput, goodInput}, nil, true, &stdout)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "foo")
	assert.NotContains(t, stdout.String(), "barRepo")
}

func testProductInput(productID, repository string) distgo.ProductPublishInfo {
	artifactName := fmt.Sprintf("%s-1.0.0-linux-amd64.tgz", productID)
	return distgo.ProductPublishInfo{
		ProductTaskOutputInfo: distgo.ProductTaskOutputInfo{
			Project: distgo.ProjectInfo{
				Version: "1.0.0",
			},
			Product: distgo.ProductOutputInfo{
				ID:                distgo.ProductID(productID),
				Name:              productID,
				PublishOutputInfo: &distgo.PublishOutputInfo{GroupID: "com.test.group"},
				DistOutputInfos: &distgo.DistOutputInfos{
					DistOutputDir: "out/dist",
					DistIDs:       []distgo.DistID{"os-arch-bin"},
					DistInfos: map[distgo.DistID]distgo.DistOutputInfo{
						"os-arch-bin": {
							DistNameTemplateRendered: fmt.Sprintf("%s-1.0.0", productID),
							DistArtifactNames:        []string{artifactName},
							PackagingExtension:       "tgz",
						},
					},
				},
			},
		},
		PublisherConfigYML: fmt.Appendf(nil, "url: http://artifactory.domain.com\nrepository: %s\nno-pom: true\n", repository),
	}
}

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

package maven

import (
	"fmt"

	"github.com/palantir/distgo/distgo"
)

// Based on https://maven.apache.org/ref/3.5.3/maven-model/maven.html
const pomTemplate = `<project xmlns="http://maven.apache.org/POM/4.0.0" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
  xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 http://maven.apache.org/xsd/maven-4.0.0.xsd">
  <modelVersion>4.0.0</modelVersion>

  <groupId>{{GroupID}}</groupId>
  <artifactId>{{Product}}</artifactId>
  <version>{{Version}}</version>{{ if ne Packaging "" }}
  <packaging>{{Packaging}}</packaging>{{end}}
</project>
`

func POM(groupID, packaging string, outputInfo distgo.ProductTaskOutputInfo) (string, string, error) {
	pomName := fmt.Sprintf("%s-%s.pom", outputInfo.Product.ID, outputInfo.Project.Version)

	pomContent, err := renderPOM(outputInfo.Product.ID, outputInfo.Project.Version, groupID, packaging)
	if err != nil {
		return "", "", err
	}
	return pomName, pomContent, nil
}

func Packaging(distID distgo.DistID, outputInfo distgo.ProductTaskOutputInfo) string {
	if outputInfo.Product.DistOutputInfos == nil {
		return ""
	}
	return outputInfo.Product.DistOutputInfos.DistInfos[distID].PackagingExtension
}

func renderPOM(productID distgo.ProductID, version, groupID, packaging string) (string, error) {
	return distgo.RenderTemplate(pomTemplate, nil,
		distgo.ProductTemplateFunction(productID),
		distgo.VersionTemplateFunction(version),
		distgo.GroupIDTemplateFunction(groupID),
		distgo.PackagingTemplateFunction(packaging),
	)
}

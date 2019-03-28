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

package config

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"strings"

	"github.com/pkg/errors"
)

func EncodeProperties(properties map[string]string) (string, error) {
	if len(properties) == 0 {
		return "", nil
	}

	var encoded []string
	for k, v := range properties {
		tmpl, err := template.New("properties").Funcs(template.FuncMap{
			"env": func(key string) string {
				return os.Getenv(key)
			},
		}).Parse(v)
		if err != nil {
			return "", errors.Wrapf(err, "failed to parse template")
		}
		output := &bytes.Buffer{}
		if err := tmpl.Execute(output, nil); err != nil {
			return "", errors.Wrapf(err, "failed to execute template")
		}
		if len(output.String()) > 0 {
			encoded = append(encoded, fmt.Sprintf("%s=%s", k, output.String()))
		}
	}
	return strings.Join(encoded, ";"), nil
}

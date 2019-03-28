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

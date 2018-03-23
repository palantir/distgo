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

package publisherfactory

import (
	"github.com/pkg/errors"

	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/publisher"
)

func New(providedPublisherCreators []publisher.Creator, providedConfigUpgraders []distgo.ConfigUpgrader) (distgo.PublisherFactory, error) {
	publisherCreators := make(map[string]publisher.Creator)
	configUpgraders := make(map[string]distgo.ConfigUpgrader)
	for k, v := range builtinPublishers() {
		publisherCreators[k] = v.Creator
		configUpgraders[k] = v.Upgrader
	}
	for _, currCreator := range providedPublisherCreators {
		publisherCreators[currCreator.TypeName()] = currCreator
	}
	for _, currUpgrader := range providedConfigUpgraders {
		currUpgrader := currUpgrader
		configUpgraders[currUpgrader.TypeName()] = currUpgrader
	}
	return &publisherFactoryImpl{
		publisherCreators:        publisherCreators,
		publisherConfigUpgraders: configUpgraders,
	}, nil
}

type publisherFactoryImpl struct {
	types                    []string
	publisherCreators        map[string]publisher.Creator
	publisherConfigUpgraders map[string]distgo.ConfigUpgrader
}

func (f *publisherFactoryImpl) Types() []string {
	return f.types
}

func (f *publisherFactoryImpl) NewPublisher(typeName string) (distgo.Publisher, error) {
	creator, ok := f.publisherCreators[typeName]
	if !ok {
		return nil, errors.Errorf("no publisher registered for publisher type %q (registered publishers: %v)", typeName, f.types)
	}
	return creator.Publisher(), nil
}

func (f *publisherFactoryImpl) ConfigUpgrader(typeName string) (distgo.ConfigUpgrader, error) {
	if _, ok := f.publisherCreators[typeName]; !ok {
		return nil, errors.Errorf("no publisher registered for publisher type %q (registered publishers: %v)", typeName, f.types)
	}
	upgrader, ok := f.publisherConfigUpgraders[typeName]
	if !ok {
		return nil, errors.Errorf("%s is a valid publisher but does not have a config upgrader", typeName)
	}
	return upgrader, nil
}

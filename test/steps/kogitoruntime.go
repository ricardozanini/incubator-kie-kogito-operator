// Copyright 2019 Red Hat, Inc. and/or its affiliates
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package steps

import (
	"github.com/cucumber/godog"
	"github.com/cucumber/messages-go/v10"
	"github.com/kiegroup/kogito-cloud-operator/pkg/apis/app/v1alpha1"
	"github.com/kiegroup/kogito-cloud-operator/test/framework"
)

func registerKogitoRuntimeSteps(s *godog.Suite, data *Data) {
	// Deploy steps
	s.Step(`^Deploy (quarkus|springboot) example runtime service "([^"]*)" with configuration:$`, data.deployExampleRuntimeServiceWithConfiguration)
	// Service steps
	s.Step(`^Expose runtime service "([^"]*)"$`, data.exposeRuntimeService)
}

// Deploy service steps

func (data *Data) deployExampleRuntimeServiceWithConfiguration(runtimeType, imageTag string, table *messages.PickleStepArgument_PickleTable) error {
	kogitoRuntime, err := getKogitoRuntimeExamplesStub(data.Namespace, runtimeType, imageTag, table)
	if err != nil {
		return err
	}

	return framework.DeployRuntimeService(data.Namespace, framework.GetDefaultInstallerType(), kogitoRuntime)
}

// Service steps

func (data *Data) exposeRuntimeService(serviceName string) error {
	return framework.ExposeServiceOnKubernetes(data.Namespace, serviceName)
}

// Misc methods

// getKogitoRuntimeExamplesStub Get basic KogitoRuntime stub with GIT properties initialized to common Kogito examples
func getKogitoRuntimeExamplesStub(namespace, runtimeType, imageTag string, table *messages.PickleStepArgument_PickleTable) (*v1alpha1.KogitoRuntime, error) {
	kogitoRuntime := framework.GetKogitoRuntimeStub(namespace, runtimeType, imageTag)

	if err := configureKogitoRuntimeFromTable(table, kogitoRuntime); err != nil {
		return nil, err
	}

	return kogitoRuntime, nil
}

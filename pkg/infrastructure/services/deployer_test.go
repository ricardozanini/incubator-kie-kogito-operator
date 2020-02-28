// Copyright 2020 Red Hat, Inc. and/or its affiliates
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

package services

import (
	"github.com/kiegroup/kogito-cloud-operator/pkg/apis/app/v1alpha1"
	"github.com/kiegroup/kogito-cloud-operator/pkg/client/kubernetes"
	"github.com/kiegroup/kogito-cloud-operator/pkg/client/meta"
	"github.com/kiegroup/kogito-cloud-operator/pkg/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"testing"
)

func Test_serviceDeployer_Deploy(t *testing.T) {
	service := &v1alpha1.KogitoJobsService{
		ObjectMeta: v1.ObjectMeta{
			Name:      "jobs-service",
			Namespace: t.Name(),
		},
		Spec: v1alpha1.KogitoJobsServiceSpec{
			InfinispanMeta:    v1alpha1.InfinispanMeta{InfinispanProperties: v1alpha1.InfinispanConnectionProperties{UseKogitoInfra: true}},
			KogitoServiceSpec: v1alpha1.KogitoServiceSpec{Replicas: 1},
		},
	}
	serviceList := &v1alpha1.KogitoJobsServiceList{}
	cli := test.CreateFakeClientOnOpenShift([]runtime.Object{service}, nil, nil)
	definition := ServiceDefinition{
		DefaultImageName: "kogito-jobs-service",
		Namespace:        t.Name(),
	}
	deployer := NewServiceDeployer(definition, serviceList, cli, meta.GetRegisteredSchema())
	reconcileAfter, err := deployer.Deploy()
	assert.NoError(t, err)
	assert.True(t, reconcileAfter > 0) // we just deployed Infinispan

	exists, err := kubernetes.ResourceC(cli).Fetch(service)
	assert.NoError(t, err)
	assert.True(t, exists)
	assert.True(t, len(service.Status.Conditions) == 1)
	assert.True(t, service.Spec.Replicas == 1)
	assert.Equal(t, v1alpha1.ProvisioningConditionType, service.Status.Conditions[0].Type)
}

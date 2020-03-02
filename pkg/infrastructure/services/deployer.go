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
	"fmt"
	"github.com/RHsyseng/operator-utils/pkg/resource/write"
	"github.com/kiegroup/kogito-cloud-operator/pkg/apis/app/v1alpha1"
	"github.com/kiegroup/kogito-cloud-operator/pkg/client"
	"github.com/kiegroup/kogito-cloud-operator/pkg/client/kubernetes"
	"github.com/kiegroup/kogito-cloud-operator/pkg/infrastructure"
	"github.com/kiegroup/kogito-cloud-operator/pkg/logger"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"time"
)

var log = logger.GetLogger("services_definition")

const (
	reconciliationPeriodAfterSingletonError = time.Minute
)

// ServiceDefinition defines the structure for a Kogito Service
type ServiceDefinition struct {
	// DefaultImageName is the name of the default image distributed for Kogito, e.g. kogito-jobs-service, kogito-data-index and so on
	DefaultImageName string
	// Namespace where the service should be deployed (you get this value from the request parameter)
	Namespace string
	// OnDeploymentCreate applies custom deployment configuration in the required Deployment resource
	OnDeploymentCreate func(deployment *appsv1.Deployment, kogitoService v1alpha1.KogitoService) error
	// SingleReplica avoids that the service has more than one pod replica
	SingleReplica bool
	// KafkaTopics is a collection of Kafka Topics to be created within the service
	KafkaTopics []KafkaTopicDefinition
	// infinispanIntegration whether or not to handle Infinispan integration in this service (inject variables, deploy if needed, and so on)
	infinispanIntegration bool
	// kafkaIntegration whether or not to handle Kafka integration in this service (inject variables, deploy if needed, and so on)
	kafkaIntegration bool
}

// KafkaTopicDefinition ...
type KafkaTopicDefinition struct {
	// TopicName name of the given topic
	TopicName string
	// MessagingType is the type for the Kafka topic: INCOMING or OUTGOING
	MessagingType KafkaTopicMessagingType
}

// KafkaTopicMessagingType ...
type KafkaTopicMessagingType string

const (
	// KafkaTopicIncoming ...
	KafkaTopicIncoming KafkaTopicMessagingType = "INCOMING"
	// KafkaTopicOutgoing ...
	KafkaTopicOutgoing KafkaTopicMessagingType = "OUTGOING"
)

// ServiceDeployer is the API to handle a Kogito Service deployment by Operator SDK controllers
type ServiceDeployer interface {
	// Deploy deploys the Kogito Service in the Kubernetes cluster according to a given ServiceDefinition
	Deploy() (reconcileAfter time.Duration, err error)
}

// NewSingletonServiceDeployer creates a new ServiceDeployer to handle Singleton Kogito Services instances and to be handled by Operator SDK controller
func NewSingletonServiceDeployer(definition ServiceDefinition, serviceList v1alpha1.KogitoServiceList, cli *client.Client, scheme *runtime.Scheme) ServiceDeployer {
	return &serviceDeployer{definition: definition, instanceList: serviceList, client: cli, scheme: scheme}
}

type serviceDeployer struct {
	definition   ServiceDefinition
	instanceList v1alpha1.KogitoServiceList
	client       *client.Client
	scheme       *runtime.Scheme
}

func (s *serviceDeployer) Deploy() (reconcileAfter time.Duration, err error) {
	// our services must be singleton instances
	if reconcile, err := s.ensureSingletonService(); err != nil || reconcile {
		return reconciliationPeriodAfterSingletonError, err
	}

	// we get our service
	service := s.instanceList.GetItemAt(0)
	reconcileAfter = 0

	// always update its status
	defer s.updateStatus(service, &err)

	if _, isInfinispan := service.GetSpec().(v1alpha1.InfinispanAware); isInfinispan {
		log.Debugf("Kogito Service %s depends on Infinispan", service.GetName())
		s.definition.infinispanIntegration = true
	}
	if _, isKafka := service.GetSpec().(v1alpha1.KafkaAware); isKafka {
		log.Debugf("Kogito Service %s depends on Kafka", service.GetName())
		s.definition.kafkaIntegration = true
	}

	// deploy Infinispan
	if s.definition.infinispanIntegration {
		reconcileAfter, err = s.deployInfinispan(service)
		if err != nil {
			return
		} else if reconcileAfter > 0 {
			return
		}
	}

	// deploy Kafka
	if s.definition.kafkaIntegration {
		reconcileAfter, err = s.deployKafka(service)
		if err != nil {
			return
		} else if reconcileAfter > 0 {
			return
		}
	}

	// create our resources
	requestedResources, err := s.createRequiredResources(service)
	if err != nil {
		return
	}

	// get the deployed ones
	deployedResources, err := s.getDeployedResources(service)
	if err != nil {
		return
	}

	// compare required and deployed, in case of any differences, we should create update or delete the k8s resources
	comparator := s.getComparator()
	deltas := comparator.Compare(deployedResources, requestedResources)
	writer := write.New(s.client.ControlCli).WithOwnerController(service, s.scheme)
	for resourceType, delta := range deltas {
		if !delta.HasChanges() {
			continue
		}
		log.Infof("Will create %d, update %d, and delete %d instances of %v", len(delta.Added), len(delta.Updated), len(delta.Removed), resourceType)
		_, err = writer.AddResources(delta.Added)
		if err != nil {
			return
		}
		_, err = writer.UpdateResources(deployedResources[resourceType], delta.Updated)
		if err != nil {
			return
		}
		_, err = writer.RemoveResources(delta.Removed)
		if err != nil {
			return
		}
	}

	return
}

func (s *serviceDeployer) ensureSingletonService() (reconcile bool, err error) {
	if err := kubernetes.ResourceC(s.client).ListWithNamespace(s.definition.Namespace, s.instanceList); err != nil {
		return true, err
	}
	if s.instanceList.GetItemsCount() > 1 {
		return true, fmt.Errorf("There's more than one Kogito Service resource in the namespace %s, please delete one of them ", s.definition.Namespace)
	} else if s.instanceList.GetItemsCount() == 0 {
		return true, nil
	}
	return false, nil
}

func (s *serviceDeployer) updateStatus(instance v1alpha1.KogitoService, err *error) {
	log.Infof("Updating status for Kogito Service %s", instance.GetName())
	if statusErr := manageStatus(instance, s.definition.DefaultImageName, s.client, *err); statusErr != nil {
		// this error will return to the operator console
		err = &statusErr
	}
	log.Infof("Successfully reconciled Kogito Service %s", instance.GetName())
}

func (s *serviceDeployer) deployInfinispan(instance v1alpha1.KogitoService) (requeueAfter time.Duration, err error) {
	requeueAfter = 0
	if !instance.GetSpec().(v1alpha1.InfinispanAware).GetInfinispanProperties().UseKogitoInfra {
		return
	}
	if !infrastructure.IsInfinispanAvailable(s.client) {
		log.Warnf("Looks like that the service %s requires Infinispan, but there's no Infinispan CRD in the namespace %s. Aborting installation.", instance.GetName(), instance.GetNamespace())
		return
	}
	update := false
	if update, requeueAfter, err =
		infrastructure.DeployInfinispanWithKogitoInfra(instance.GetSpec().(v1alpha1.InfinispanAware), instance.GetNamespace(), s.client); err != nil {
		return
	} else if update {
		if err = kubernetes.ResourceC(s.client).Update(instance); err != nil {
			return
		}
	}
	return
}

func (s *serviceDeployer) deployKafka(instance v1alpha1.KogitoService) (requeueAfter time.Duration, err error) {
	requeueAfter = 0
	if !instance.GetSpec().(v1alpha1.KafkaAware).GetKafkaProperties().UseKogitoInfra {
		return
	}
	if !infrastructure.IsStrimziAvailable(s.client) {
		log.Warnf("Looks like that the service %s requires Kafka, but there's no Kafka CRD in the namespace %s. Aborting installation.", instance.GetName(), instance.GetNamespace())
		return
	}

	update := false
	if update, requeueAfter, err =
		infrastructure.DeployKafkaWithKogitoInfra(instance.GetSpec().(v1alpha1.KafkaAware), instance.GetNamespace(), s.client); err != nil {
		return
	} else if update {
		if err = kubernetes.ResourceC(s.client).Update(instance); err != nil {
			return
		}
	}
	return
}

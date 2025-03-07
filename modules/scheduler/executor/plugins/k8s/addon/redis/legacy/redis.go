// Copyright (c) 2021 Terminus, Inc.
//
// This program is free software: you can use, redistribute, and/or modify
// it under the terms of the GNU Affero General Public License, version 3
// or later ("AGPL"), as published by the Free Software Foundation.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program. If not, see <http://www.gnu.org/licenses/>.

package legacy

import (
	"bytes"
	"fmt"

	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/erda-project/erda/apistructs"
	"github.com/erda-project/erda/modules/scheduler/executor/plugins/k8s/addon"
	"github.com/erda-project/erda/modules/scheduler/executor/plugins/k8s/k8sapi"
	"github.com/erda-project/erda/modules/scheduler/schedulepolicy/constraintbuilders"
	"github.com/erda-project/erda/pkg/http/httpclient"
	"github.com/erda-project/erda/pkg/strutil"
)

type RedisOperator struct {
	k8s         addon.K8SUtil
	deployment  addon.DeploymentUtil
	statefulset addon.StatefulsetUtil
	ns          addon.NamespaceUtil
	service     addon.ServiceUtil
	overcommit  addon.OvercommitUtil
	client      *httpclient.HTTPClient
}

func New(k8sutil addon.K8SUtil,
	deploy addon.DeploymentUtil,
	sts addon.StatefulsetUtil,
	service addon.ServiceUtil,
	ns addon.NamespaceUtil,
	overcommit addon.OvercommitUtil,
	client *httpclient.HTTPClient) *RedisOperator {
	return &RedisOperator{
		k8s:         k8sutil,
		deployment:  deploy,
		statefulset: sts,
		service:     service,
		ns:          ns,
		overcommit:  overcommit,
		client:      client,
	}
}

func (ro *RedisOperator) IsSupported() bool {
	resp, err := ro.client.Get(ro.k8s.GetK8SAddr()).
		Path("/apis/storage.spotahome.com/v1alpha2").
		Do().
		DiscardBody()
	if err != nil {
		logrus.Errorf("failed to query /apis/storage.spotahome.com/v1alpha2, host: %v, err: %v",
			ro.k8s.GetK8SAddr(), err)
		return false
	}
	if !resp.IsOK() {
		return false
	}
	return true
}

// Validate
func (ro *RedisOperator) Validate(sg *apistructs.ServiceGroup) error {
	operator, ok := sg.Labels["USE_OPERATOR"]
	if !ok {
		return fmt.Errorf("[BUG] sg need USE_OPERATOR label")
	}
	if strutil.ToLower(operator) != svcNameRedis {
		return fmt.Errorf("[BUG] value of label USE_OPERATOR should be 'redis'")
	}
	if len(sg.Services) != 2 {
		return fmt.Errorf("illegal services num: %d", len(sg.Services))
	}
	if sg.Services[0].Name != svcNameRedis && sg.Services[0].Name != svcNameSentinel {
		return fmt.Errorf("illegal service: %+v, should be one of [redis, sentinel]", sg.Services[0])
	}
	if sg.Services[1].Name != svcNameRedis && sg.Services[1].Name != svcNameSentinel {
		return fmt.Errorf("illegal service: %+v, should be one of [redis, sentinel]", sg.Services[1])
	}
	var redis apistructs.Service
	if sg.Services[0].Name == svcNameRedis {
		redis = sg.Services[0]
	}
	// if sg.Services[0].Name == svcNameSentinel {
	// 	sentinel = sg.Services[0]
	// }
	if sg.Services[1].Name == svcNameRedis {
		redis = sg.Services[1]
	}
	// if sg.Services[1].Name == svcNameSentinel {
	// 	sentinel = sg.Services[1]
	// }
	if _, ok := redis.Env["requirepass"]; !ok {
		return fmt.Errorf("redis service not provide 'requirepass' env")
	}
	return nil
}

func (ro *RedisOperator) Convert(sg *apistructs.ServiceGroup) interface{} {
	svc0 := sg.Services[0]
	svc1 := sg.Services[1]
	var redis RedisSettings
	var sentinel SentinelSettings
	switch svc0.Name {
	case svcNameRedis:
		redis = ro.convertRedis(svc0)
	case svcNameSentinel:
		sentinel = convertSentinel(svc0)
	}
	switch svc1.Name {
	case svcNameRedis:
		redis = ro.convertRedis(svc1)
	case svcNameSentinel:
		sentinel = convertSentinel(svc1)
	}
	scheinfo := sg.ScheduleInfo2
	scheinfo.Stateful = true
	affinity := constraintbuilders.K8S(&scheinfo, nil, nil, nil).Affinity.NodeAffinity

	return RedisFailover{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "storage.spotahome.com/v1alpha2",
			Kind:       "RedisFailover",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      sg.ID,
			Namespace: genK8SNamespace(sg.Type, sg.ID),
		},
		Spec: RedisFailoverSpec{
			Redis:        redis,
			Sentinel:     sentinel,
			NodeAffinity: affinity,
		},
	}
}

func (ro *RedisOperator) Create(k8syml interface{}) error {
	redis, ok := k8syml.(RedisFailover)
	if !ok {
		return fmt.Errorf("[BUG] this k8syml should be RedisFailover")
	}
	if err := ro.ns.Exists(redis.Namespace); err != nil {
		if err := ro.ns.Create(redis.Namespace, nil); err != nil {
			return err
		}
	}

	var b bytes.Buffer
	resp, err := ro.client.Post(ro.k8s.GetK8SAddr()).
		Path(fmt.Sprintf("/apis/storage.spotahome.com/v1alpha2/namespaces/%s/redisfailovers", redis.Namespace)).
		JSONBody(redis).
		Do().
		Body(&b)
	if err != nil {
		return fmt.Errorf("failed to create redisfailover, %s/%s, err: %v", redis.Namespace, redis.Name, err)
	}
	if !resp.IsOK() {
		return fmt.Errorf("failed to create redisfailover, %s/%s, statuscode: %v, body: %v",
			redis.Namespace, redis.Name, resp.StatusCode(), b.String())
	}
	return nil
}

func (ro *RedisOperator) Inspect(sg *apistructs.ServiceGroup) (*apistructs.ServiceGroup, error) {
	deploylist, err := ro.deployment.List(genK8SNamespace(sg.Type, sg.ID), nil)
	if err != nil {
		return nil, err
	}
	stslist, err := ro.statefulset.List(genK8SNamespace(sg.Type, sg.ID))
	if err != nil {
		return nil, err
	}
	svclist, err := ro.service.List(genK8SNamespace(sg.Type, sg.ID))
	if err != nil {
		return nil, err
	}
	var redis, sentinel *apistructs.Service
	if sg.Services[0].Name == svcNameRedis {
		redis = &(sg.Services[0])
	}
	if sg.Services[1].Name == svcNameRedis {
		redis = &(sg.Services[1])
	}
	if sg.Services[0].Name == svcNameSentinel {
		sentinel = &(sg.Services[0])
	}
	if sg.Services[1].Name == svcNameSentinel {
		sentinel = &(sg.Services[1])
	}
	for _, deploy := range deploylist.Items {
		for _, cond := range deploy.Status.Conditions {
			if cond.Type == appsv1.DeploymentAvailable {
				if cond.Status == corev1.ConditionTrue {
					sentinel.Status = apistructs.StatusHealthy
				} else {
					sentinel.Status = apistructs.StatusUnHealthy
				}
			}
		}
	}
	for _, sts := range stslist.Items {
		if sts.Spec.Replicas == nil {
			redis.Status = apistructs.StatusUnknown
		} else if *sts.Spec.Replicas == sts.Status.ReadyReplicas {
			redis.Status = apistructs.StatusHealthy
		} else {
			redis.Status = apistructs.StatusUnHealthy
		}
	}

	for _, svc := range svclist.Items {
		sentinel.Vip = strutil.Join([]string{svc.Name, svc.Namespace, "svc.cluster.local"}, ".")
	}
	if redis.Status == apistructs.StatusHealthy && sentinel.Status == apistructs.StatusHealthy {
		sg.Status = apistructs.StatusHealthy
	} else {
		sg.Status = apistructs.StatusUnHealthy
	}
	return sg, nil
}

func (ro *RedisOperator) Remove(sg *apistructs.ServiceGroup) error {
	k8snamespace := genK8SNamespace(sg.Type, sg.ID)
	var b bytes.Buffer
	resp, err := ro.client.Delete(ro.k8s.GetK8SAddr()).
		Path(fmt.Sprintf("/apis/storage.spotahome.com/v1alpha2/namespaces/%s/redisfailovers/%s", k8snamespace, sg.ID)).
		JSONBody(k8sapi.DeleteOptions).
		Do().
		Body(&b)
	if err != nil {
		return fmt.Errorf("failed to delele redisfailover: %s/%s, err: %v", sg.Type, sg.ID, err)
	}
	if !resp.IsOK() {
		if resp.IsNotfound() {
			return nil
		}
		return fmt.Errorf("failed to delete redisfailover: %s/%s, statuscode: %v, body: %v",
			sg.Type, sg.ID, resp.StatusCode(), b.String())
	}

	if err := ro.ns.Delete(k8snamespace); err != nil {
		logrus.Errorf("failed to delete namespace: %s: %v", k8snamespace, err)
		return nil
	}
	return nil
}

func (ro *RedisOperator) Update(k8syml interface{}) error {
	// TODO:
	return fmt.Errorf("redisoperator not impl Update yet")
}

func (ro *RedisOperator) convertRedis(svc apistructs.Service) RedisSettings {
	settings := RedisSettings{}
	settings.Version = "3.2.12"
	settings.Envs = svc.Env
	settings.Replicas = int32(svc.Scale)
	settings.Resources = RedisFailoverResources{
		Requests: CPUAndMem{
			CPU:    fmt.Sprintf("%dm", int(1000*ro.overcommit.CPUOvercommit(svc.Resources.Cpu))),
			Memory: fmt.Sprintf("%dMi", ro.overcommit.MemoryOvercommit(int(svc.Resources.Mem))),
		},
		Limits: CPUAndMem{
			CPU:    fmt.Sprintf("%dm", int(1000*svc.Resources.Cpu)),
			Memory: fmt.Sprintf("%dMi", int(svc.Resources.Mem)),
		},
	}
	settings.Image = svc.Image
	return settings
}

func convertSentinel(svc apistructs.Service) SentinelSettings {
	settings := SentinelSettings{}
	settings.Envs = svc.Env
	settings.Replicas = int32(svc.Scale)
	settings.Resources = RedisFailoverResources{
		Requests: CPUAndMem{ // sentinel Not over-provisioned, because it should already occupy very little resources
			CPU:    fmt.Sprintf("%dm", int(1000*svc.Resources.Cpu)),
			Memory: fmt.Sprintf("%dMi", int(svc.Resources.Mem)),
		},
		Limits: CPUAndMem{
			CPU:    fmt.Sprintf("%dm", int(1000*svc.Resources.Cpu)),
			Memory: fmt.Sprintf("%dMi", int(svc.Resources.Mem)),
		},
	}
	settings.CustomConfig = []string{
		fmt.Sprintf("auth-pass %s", svc.Env["requirepass"]),
		"down-after-milliseconds 12000",
		"failover-timeout 12000",
	}
	return settings
}

func genK8SNamespace(namespace, name string) string {
	return strutil.Concat(namespace, "--", name)
}

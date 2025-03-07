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

package registercenter

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/erda-project/erda-proto-go/msp/registercenter/pb"
	"github.com/erda-project/erda/modules/msp/instance"
	instancedb "github.com/erda-project/erda/modules/msp/instance/db"
	"github.com/erda-project/erda/modules/msp/registercenter/nacos"
	"github.com/erda-project/erda/modules/msp/registercenter/zkproxy"
	"github.com/erda-project/erda/pkg/common/errors"
)

const engineName = "registercenter"

type registerCenterService struct {
	p                *provider
	instanceTenantDB *instancedb.InstanceTenantDB
	instanceDB       *instancedb.InstanceDB
}

func (s *registerCenterService) ListInterface(ctx context.Context, req *pb.ListInterfaceRequest) (*pb.ListInterfaceResponse, error) {
	result := &pb.ListInterfaceResponse{Data: make([]*pb.Interface, 0)}
	if len(req.TenantGroup) <= 0 {
		return result, nil
	}
	clusterName, err := s.instanceTenantDB.GetClusterNameByTenantGroup(req.TenantGroup)
	if err != nil {
		return nil, errors.NewDataBaseError(err)
	}
	ins := instance.New(s.p.DB)
	config, err := ins.GetConfigOptionByGroup(engineName, req.TenantGroup)
	if err != nil {
		return nil, errors.NewDataBaseError(err)
	}
	if addr, ok := config.Config["ZKPROXY_PUBLIC_HOST"].(string); ok {
		namespace := req.TenantID
		if len(namespace) <= 0 {
			namespace = req.ProjectID + "_" + strings.ToLower(req.Env)
		}
		adp := zkproxy.NewAdapter(clusterName, addr)
		list, err := adp.GetAllInterfaceList(req.ProjectID, req.Env, namespace)
		if err != nil {
			return nil, errors.NewServiceInvokingError("zkproxy.GetAllInterfaceList", err)
		}
		result.Data = append(result.Data, list...)
	} else if addr, ok := config.Config["NACOS_ADDRESS"].(string); ok {
		namespace, _ := config.Config["NACOS_TENANT_ID"].(string)
		adp := nacos.NewAdapter(clusterName, addr)
		list, err := adp.GetDubboInterfaceList(namespace)
		if err != nil {
			return nil, errors.NewServiceInvokingError("nacos.GetDubboInterfaceList", err)
		}
		result.Data = append(result.Data, list...)
	}
	return result, nil
}

func (s *registerCenterService) GetHTTPServices(ctx context.Context, req *pb.GetHTTPServicesRequest) (*pb.GetHTTPServicesResponse, error) {
	result := &pb.GetHTTPServicesResponse{
		Data: &pb.HTTPServices{ServiceList: make([]*pb.HTTPService, 0)},
	}
	if len(req.TenantGroup) <= 0 {
		return result, nil
	}
	clusterName, err := s.instanceTenantDB.GetClusterNameByTenantGroup(req.TenantGroup)
	if err != nil {
		return nil, errors.NewDataBaseError(err)
	}
	ins := instance.New(s.p.DB)
	config, err := ins.GetConfigOptionByGroup(engineName, req.TenantGroup)
	if err != nil {
		return nil, errors.NewDataBaseError(err)
	}
	if addr, ok := config.Config["NACOS_ADDRESS"].(string); ok {
		namespace, _ := config.Config["NACOS_TENANT_ID"].(string)
		adp := nacos.NewAdapter(clusterName, addr)
		list, err := adp.GetHTTPInterfaceList(namespace)
		if err != nil {
			return nil, errors.NewServiceInvokingError("nacos.GetHTTPInterfaceList", err)
		}
		result.Data.ServiceList = append(result.Data.ServiceList, list...)
	}
	return result, nil
}

func (s *registerCenterService) EnableHTTPService(ctx context.Context, req *pb.EnableHTTPServiceRequest) (*pb.EnableHTTPServiceResponse, error) {
	result := &pb.EnableHTTPServiceResponse{}
	if len(req.TenantGroup) <= 0 {
		return result, nil
	}
	clusterName, err := s.instanceTenantDB.GetClusterNameByTenantGroup(req.TenantGroup)
	if err != nil {
		return nil, errors.NewDataBaseError(err)
	}
	ins := instance.New(s.p.DB)
	config, err := ins.GetConfigOptionByGroup(engineName, req.TenantGroup)
	if err != nil {
		return nil, errors.NewDataBaseError(err)
	}
	if addr, ok := config.Config["NACOS_ADDRESS"].(string); ok {
		namespace, _ := config.Config["NACOS_TENANT_ID"].(string)
		adp := nacos.NewAdapter(clusterName, addr)
		data, err := adp.EnableHTTPService(namespace, req.Service)
		if err != nil {
			return nil, errors.NewServiceInvokingError("nacos.EnableHTTPService", err)
		}
		result.Data = data
	}
	return result, nil
}

func (s *registerCenterService) getzkProxyHost(clusterName string) (string, error) {
	data, err := s.instanceDB.GetByFields(map[string]interface{}{
		"Engine":  engineName,
		"Version": "1.0.0",
		"Az":      clusterName,
	})
	if err != nil {
		return "", err
	}
	if data == nil {
		return "", fmt.Errorf("fail to get registercenter from tmc")
	}
	config := make(map[string]interface{})
	json.Unmarshal([]byte(data.Config), &config)
	host, ok := config["ZKPROXY_PUBLIC_HOST"].(string)
	if !ok {
		return "", fmt.Errorf("fail to get zkproxy config from tmc")
	}
	return host, nil
}

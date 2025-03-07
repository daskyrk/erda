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

package endpoints

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/erda-project/erda/apistructs"
	"github.com/erda-project/erda/modules/cmdb/dao"
	"github.com/erda-project/erda/modules/cmdb/services/apierrors"
	"github.com/erda-project/erda/modules/pkg/user"
	"github.com/erda-project/erda/pkg/http/httpserver"
	"github.com/erda-project/erda/pkg/http/httputil"
	"github.com/erda-project/erda/pkg/strutil"
)

// CreatePublisher 创建Publisher
func (e *Endpoints) CreatePublisher(ctx context.Context, r *http.Request, vars map[string]string) (httpserver.Responser, error) {
	// 获取当前用户
	userID, err := user.GetUserID(r)
	if err != nil {
		return apierrors.ErrCreatePublisher.NotLogin().ToResp(), nil
	}

	if r.Body == nil {
		return apierrors.ErrCreatePublisher.MissingParameter("body").ToResp(), nil
	}
	var publisherCreateReq apistructs.PublisherCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&publisherCreateReq); err != nil {
		return apierrors.ErrCreatePublisher.InvalidParameter(err).ToResp(), nil
	}
	logrus.Infof("request body: %+v", publisherCreateReq)

	// 操作鉴权
	req := apistructs.PermissionCheckRequest{
		UserID:   userID.String(),
		Scope:    apistructs.OrgScope,
		ScopeID:  publisherCreateReq.OrgID,
		Resource: apistructs.PublisherResource,
		Action:   apistructs.CreateAction,
	}
	if access, err := e.permission.CheckPermission(&req); err != nil || !access {
		return apierrors.ErrCreatePublisher.AccessDenied().ToResp(), nil
	}

	publisherID, err := e.publisher.Create(userID.String(), &publisherCreateReq)
	if err != nil {
		return apierrors.ErrCreatePublisher.InternalError(err).ToResp(), nil
	}

	return httpserver.OkResp(publisherID)
}

// UpdatePublisher 更新Publisher
func (e *Endpoints) UpdatePublisher(ctx context.Context, r *http.Request, vars map[string]string) (httpserver.Responser, error) {
	// 获取当前用户
	userID, err := user.GetUserID(r)
	if err != nil {
		return apierrors.ErrUpdatePublisher.NotLogin().ToResp(), nil
	}
	orgIDStr := r.Header.Get(httputil.OrgHeader)
	orgID, err := strutil.Atoi64(orgIDStr)
	if err != nil {
		return apierrors.ErrUpdatePublisher.InvalidParameter(err).ToResp(), nil
	}

	// 检查request body合法性
	if r.Body == nil {
		return apierrors.ErrUpdatePublisher.MissingParameter("body").ToResp(), nil
	}
	var publisherUpdateReq apistructs.PublisherUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&publisherUpdateReq); err != nil {
		return apierrors.ErrUpdatePublisher.InvalidParameter(err).ToResp(), nil
	}
	logrus.Infof("request body: %+v", publisherUpdateReq)

	// 检查publisherID合法性
	if publisherUpdateReq.ID == 0 {
		return apierrors.ErrUpdatePublisher.InvalidParameter("need publisher id.").ToResp(), nil
	}

	// 操作鉴权
	req := apistructs.PermissionCheckRequest{
		UserID:   userID.String(),
		Scope:    apistructs.OrgScope,
		ScopeID:  uint64(orgID),
		Resource: apistructs.PublisherResource,
		Action:   apistructs.UpdateAction,
	}
	if access, err := e.permission.CheckPermission(&req); err != nil || !access {
		// 若非Publisher管理员，判断用户是否为企业管理员(数据中心)
		req := apistructs.PermissionCheckRequest{
			UserID:   userID.String(),
			Scope:    apistructs.OrgScope,
			ScopeID:  uint64(orgID),
			Resource: apistructs.PublisherResource,
			Action:   apistructs.UpdateAction,
		}
		if access, err := e.permission.CheckPermission(&req); err != nil || !access {
			return apierrors.ErrUpdatePublisher.AccessDenied().ToResp(), nil
		}
	}

	// 更新Publisher信息至DB
	err = e.publisher.Update(&publisherUpdateReq)
	if err != nil {
		return apierrors.ErrUpdatePublisher.InternalError(err).ToResp(), nil
	}

	return httpserver.OkResp(publisherUpdateReq.ID)
}

// GetPublisher 获取Publisher详情
func (e *Endpoints) GetPublisher(ctx context.Context, r *http.Request, vars map[string]string) (httpserver.Responser, error) {
	// 检查publisherID合法性
	publisherID, err := strutil.Atoi64(vars["publisherID"])
	if err != nil {
		return apierrors.ErrGetPublisher.InvalidParameter(err).ToResp(), nil
	}
	publisher, err := e.publisher.Get(publisherID)
	if err != nil {
		if err == dao.ErrNotFoundPublisher {
			return apierrors.ErrGetPublisher.NotFound().ToResp(), nil
		}
		return apierrors.ErrGetPublisher.InternalError(err).ToResp(), nil
	}
	internalClient := r.Header.Get(httputil.InternalHeader)
	if internalClient == "" {
		userID, err := user.GetUserID(r)
		if err != nil {
			return apierrors.ErrGetPublisher.NotLogin().ToResp(), nil
		}
		// 操作鉴权
		req := apistructs.PermissionCheckRequest{
			UserID:   userID.String(),
			Scope:    apistructs.OrgScope,
			ScopeID:  publisher.OrgID,
			Resource: apistructs.PublisherResource,
			Action:   apistructs.GetAction,
		}
		if access, err := e.permission.CheckPermission(&req); err != nil || !access {
			return apierrors.ErrGetPublisher.AccessDenied().ToResp(), nil
		}
	}

	return httpserver.OkResp(*publisher)
}

// DeletePublisher 删除Publisher
func (e *Endpoints) DeletePublisher(ctx context.Context, r *http.Request, vars map[string]string) (httpserver.Responser, error) {
	// 获取当前用户
	userID, err := user.GetUserID(r)
	if err != nil {
		return apierrors.ErrDeletePublisher.NotLogin().ToResp(), nil
	}

	// 获取 OrgID
	orgIDStr := r.Header.Get(httputil.OrgHeader)
	if orgIDStr == "" {
		return apierrors.ErrDeletePublisher.MissingParameter("org id").ToResp(), nil
	}
	orgID, err := strutil.Atoi64(orgIDStr)
	if err != nil {
		return apierrors.ErrDeletePublisher.InvalidParameter(err).ToResp(), nil
	}

	// 检查publisherID合法性
	publisherID, err := strutil.Atoi64(vars["publisherID"])
	if err != nil {
		return apierrors.ErrDeletePublisher.InvalidParameter(err).ToResp(), nil
	}

	// 操作鉴权
	req := apistructs.PermissionCheckRequest{
		UserID:   userID.String(),
		Scope:    apistructs.OrgScope,
		ScopeID:  uint64(orgID),
		Resource: apistructs.PublisherResource,
		Action:   apistructs.DeleteAction,
	}
	if access, err := e.permission.CheckPermission(&req); err != nil || !access {
		return apierrors.ErrDeletePublisher.AccessDenied().ToResp(), nil
	}

	// 删除Publisher
	err = e.publisher.Delete(publisherID, orgID)
	if err != nil {
		return apierrors.ErrDeletePublisher.InternalError(err).ToResp(), nil
	}

	return httpserver.OkResp(publisherID)
}

// ListPublisher 所有Publisher列表
func (e *Endpoints) ListPublishers(ctx context.Context, r *http.Request, vars map[string]string) (httpserver.Responser, error) {
	// 获取当前用户
	userID, err := user.GetUserID(r)
	if err != nil {
		return apierrors.ErrListPublisher.NotLogin().ToResp(), nil
	}

	// 获取请求参数
	params, err := getListPublishersParam(r)
	if err != nil {
		return apierrors.ErrListPublisher.InvalidParameter(err).ToResp(), nil
	}

	// 企业管理员和 Support 都可以调用
	internalClient := r.Header.Get(httputil.InternalHeader)
	if internalClient == "" {
		req := apistructs.PermissionCheckRequest{
			UserID:   userID.String(),
			Scope:    apistructs.OrgScope,
			ScopeID:  params.OrgID,
			Resource: apistructs.PublisherResource,
			Action:   apistructs.ListAction,
		}
		perm, err := e.permission.CheckPermission(&req)
		if err != nil {
			return apierrors.ErrListPublisher.InternalError(err).ToResp(), nil
		}
		if !perm {
			return apierrors.ErrListPublisher.AccessDenied().ToResp(), nil
		}
	}

	pagingPublishers, err := e.publisher.ListAllPublishers(userID.String(), params)
	if err != nil {
		return apierrors.ErrListPublisher.InternalError(err).ToResp(), nil
	}

	// userIDs
	userIDs := make([]string, 0, len(pagingPublishers.List))
	for _, n := range pagingPublishers.List {
		userIDs = append(userIDs, n.Creator)
	}
	userIDs = strutil.DedupSlice(userIDs, true)

	return httpserver.OkResp(*pagingPublishers, userIDs)
}

// ListMyPublisher 我的Publisher列表
func (e *Endpoints) ListMyPublishers(ctx context.Context, r *http.Request, vars map[string]string) (httpserver.Responser, error) {
	// 获取当前用户
	userID, err := user.GetUserID(r)
	if err != nil {
		return apierrors.ErrListPublisher.NotLogin().ToResp(), nil
	}

	// 获取请求参数
	params, err := getListPublishersParam(r)
	if err != nil {
		return apierrors.ErrListPublisher.InvalidParameter(err).ToResp(), nil
	}

	pagingPublishers, err := e.publisher.ListJoinedPublishers(userID.String(), params)
	if err != nil {
		return apierrors.ErrListPublisher.InternalError(err).ToResp(), nil
	}

	return httpserver.OkResp(*pagingPublishers)
}

// Publisher列表时获取请求参数
func getListPublishersParam(r *http.Request) (*apistructs.PublisherListRequest, error) {
	// 获取企业Id
	orgIDStr := r.URL.Query().Get("orgId")
	if orgIDStr == "" {
		orgIDStr = r.Header.Get(httputil.OrgHeader)
		if orgIDStr == "" {
			return nil, errors.Errorf("invalid param, orgId is empty")
		}
	}
	orgID, err := strconv.ParseInt(orgIDStr, 10, 64)
	if err != nil {
		return nil, errors.Errorf("invalid param, orgId is invalid")
	}

	// 按Publisher名称搜索
	keyword := r.URL.Query().Get("q")

	// 获取pageSize
	pageSizeStr := r.URL.Query().Get("pageSize")
	if pageSizeStr == "" {
		pageSizeStr = "20"
	}
	pageSize, err := strconv.Atoi(pageSizeStr)
	if err != nil {
		return nil, errors.Errorf("invalid param, pageSize is invalid")
	}
	// 获取pageNo
	pageNoStr := r.URL.Query().Get("pageNo")
	if pageNoStr == "" {
		pageNoStr = "1"
	}
	pageNo, err := strconv.Atoi(pageNoStr)
	if err != nil {
		return nil, errors.Errorf("invalid param, pageNo is invalid")
	}

	return &apistructs.PublisherListRequest{
		OrgID:    uint64(orgID),
		Query:    keyword,
		Name:     r.URL.Query().Get("name"),
		PageNo:   pageNo,
		PageSize: pageSize,
	}, nil
}

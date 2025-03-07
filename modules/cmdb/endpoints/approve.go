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
	"github.com/erda-project/erda/modules/pkg/user"
	"github.com/erda-project/erda/pkg/http/httpserver"
	"github.com/erda-project/erda/pkg/http/httputil"
	"github.com/erda-project/erda/pkg/strutil"

	"github.com/erda-project/erda/modules/cmdb/dao"
	"github.com/erda-project/erda/modules/cmdb/services/apierrors"
)

// CreateApprove 创建审批流程
func (e *Endpoints) CreateApprove(ctx context.Context, r *http.Request, vars map[string]string) (httpserver.Responser, error) {
	// 获取 body 信息
	var approveCreateReq apistructs.ApproveCreateRequest
	if r.Body == nil {
		return apierrors.ErrCreateApprove.MissingParameter("body").ToResp(), nil
	}
	if err := json.NewDecoder(r.Body).Decode(&approveCreateReq); err != nil {
		return apierrors.ErrCreateApprove.InvalidParameter(err).ToResp(), nil
	}
	logrus.Debugf("create approve request body: %+v", approveCreateReq)

	// 操作鉴权
	identityInfo, err := user.GetIdentityInfo(r)
	if err != nil {
		return apierrors.ErrGetUser.InvalidParameter(err).ToResp(), nil
	}
	if !identityInfo.IsInternalClient() {
		req := apistructs.PermissionCheckRequest{
			UserID:   identityInfo.UserID,
			Scope:    apistructs.AppScope,
			ScopeID:  approveCreateReq.TargetID,
			Resource: apistructs.ApproveResource,
			Action:   apistructs.CreateAction,
		}
		if approveCreateReq.Type == apistructs.ApproveUnblockAppication {
			req.Scope = apistructs.ProjectScope
		}
		if access, err := e.permission.CheckPermission(&req); err != nil || !access {
			return apierrors.ErrCreateApprove.AccessDenied().ToResp(), nil
		}
	}

	// 获取 orgID
	if approveCreateReq.OrgID == 0 {
		orgIDStr := r.Header.Get(httputil.OrgHeader)
		orgID, err := strconv.ParseUint(orgIDStr, 10, 64)
		if err != nil {
			return apierrors.ErrUpdateCertificate.InvalidParameter(err).ToResp(), nil
		}
		approveCreateReq.OrgID = orgID
	}

	approveID, err := e.approve.Create(identityInfo.UserID, &approveCreateReq)
	if err != nil {
		return apierrors.ErrCreateApprove.InternalError(err).ToResp(), nil
	}

	return httpserver.OkResp(approveID)
}

// UpdateApprove 更新Approve
func (e *Endpoints) UpdateApprove(ctx context.Context, r *http.Request, vars map[string]string) (httpserver.Responser, error) {
	approveID, err := strutil.Atoi64(vars["approveId"])
	if err != nil {
		return apierrors.ErrGetApprove.InvalidParameter(err).ToResp(), nil
	}

	// 检查approveID合法性
	if approveID == 0 {
		return apierrors.ErrUpdateApprove.InvalidParameter("need approveId").ToResp(), nil
	}

	// 获取 body 信息
	var approveUpdateReq apistructs.ApproveUpdateRequest
	if r.Body == nil {
		return apierrors.ErrUpdateApprove.MissingParameter("body").ToResp(), nil
	}
	if err := json.NewDecoder(r.Body).Decode(&approveUpdateReq); err != nil {
		return apierrors.ErrUpdateApprove.InvalidParameter(err).ToResp(), nil
	}
	logrus.Infof("update approve request body: %+v", approveUpdateReq)

	// 操作鉴权
	identityInfo, err := user.GetIdentityInfo(r)
	if err != nil {
		return apierrors.ErrGetUser.InvalidParameter(err).ToResp(), nil
	}
	if !identityInfo.IsInternalClient() {
		orgIDStr := r.Header.Get(httputil.OrgHeader)
		orgID, err := strconv.ParseUint(orgIDStr, 10, 64)
		if err != nil {
			return apierrors.ErrUpdateApprove.InvalidParameter(err).ToResp(), nil
		}

		req := apistructs.PermissionCheckRequest{
			UserID:   identityInfo.UserID,
			Scope:    apistructs.OrgScope,
			ScopeID:  orgID,
			Resource: apistructs.ApproveResource,
			Action:   apistructs.UpdateAction,
		}
		if access, err := e.permission.CheckPermission(&req); err != nil || !access {
			return apierrors.ErrUpdateApprove.AccessDenied().ToResp(), nil
		}
	}

	// 获取 orgID
	if approveUpdateReq.OrgID == 0 {
		orgIDStr := r.Header.Get(httputil.OrgHeader)
		orgID, err := strconv.ParseUint(orgIDStr, 10, 64)
		if err != nil {
			return apierrors.ErrUpdateCertificate.InvalidParameter(err).ToResp(), nil
		}
		approveUpdateReq.OrgID = orgID
	}

	// 更新审批人
	approveUpdateReq.Approver = identityInfo.UserID

	// 更新Approve信息至DB
	err = e.approve.Update(approveID, &approveUpdateReq)
	if err != nil {
		return apierrors.ErrUpdateApprove.InternalError(err).ToResp(), nil
	}

	return httpserver.OkResp(approveID)
}

// GetApprove 获取Approve详情
func (e *Endpoints) GetApprove(ctx context.Context, r *http.Request, vars map[string]string) (httpserver.Responser, error) {
	// 检查approveID合法性
	approveID, err := strutil.Atoi64(vars["approveId"])
	if err != nil {
		return apierrors.ErrGetApprove.InvalidParameter(err).ToResp(), nil
	}

	// 操作鉴权
	identityInfo, err := user.GetIdentityInfo(r)
	if err != nil {
		return apierrors.ErrGetUser.InvalidParameter(err).ToResp(), nil
	}
	if !identityInfo.IsInternalClient() {
		var (
			req    = apistructs.PermissionCheckRequest{}
			access bool
		)
		appIDStr := r.URL.Query().Get("appID")
		if appIDStr != "" {
			appID, err := strconv.ParseUint(appIDStr, 10, 64)
			if err != nil {
				return apierrors.ErrGetApprove.InvalidParameter(err).ToResp(), nil
			}

			req = apistructs.PermissionCheckRequest{
				UserID:   identityInfo.UserID,
				Scope:    apistructs.AppScope,
				ScopeID:  appID,
				Resource: apistructs.ApproveResource,
				Action:   apistructs.GetAction,
			}
		}

		access, err = e.permission.CheckPermission(&req)
		if err != nil || !access {
			orgIDStr := r.Header.Get(httputil.OrgHeader)
			orgID, err := strconv.ParseUint(orgIDStr, 10, 64)
			if err != nil {
				return apierrors.ErrUpdateApprove.InvalidParameter(err).ToResp(), nil
			}

			req = apistructs.PermissionCheckRequest{
				UserID:   identityInfo.UserID,
				Scope:    apistructs.OrgScope,
				ScopeID:  orgID,
				Resource: apistructs.ApproveResource,
				Action:   apistructs.GetAction,
			}
			if access, err = e.permission.CheckPermission(&req); err != nil || !access {
				return apierrors.ErrUpdateApprove.AccessDenied().ToResp(), nil
			}
		}
	}

	approve, err := e.approve.Get(approveID)
	if err != nil {
		if err == dao.ErrNotFoundApprove {
			return apierrors.ErrGetApprove.NotFound().ToResp(), nil
		}
		return apierrors.ErrGetApprove.InternalError(err).ToResp(), nil
	}

	return httpserver.OkResp(*approve)
}

// ListApprove 所有Approve列表
func (e *Endpoints) ListApproves(ctx context.Context, r *http.Request, vars map[string]string) (httpserver.Responser, error) {
	// 获取请求参数
	params, err := getListApprovesParam(r)
	if err != nil {
		return apierrors.ErrListApprove.InvalidParameter(err).ToResp(), nil
	}

	// 操作鉴权
	identityInfo, err := user.GetIdentityInfo(r)
	if err != nil {
		return apierrors.ErrGetUser.InvalidParameter(err).ToResp(), nil
	}
	if !identityInfo.IsInternalClient() {
		req := apistructs.PermissionCheckRequest{
			UserID:   identityInfo.UserID,
			Scope:    apistructs.OrgScope,
			ScopeID:  params.OrgID,
			Resource: apistructs.ApproveResource,
			Action:   apistructs.ListAction,
		}
		if access, err := e.permission.CheckPermission(&req); err != nil || !access {
			return apierrors.ErrListApprove.AccessDenied().ToResp(), nil
		}
	}

	pagingApproves, err := e.approve.ListAllApproves(params)
	if err != nil {
		return apierrors.ErrListApprove.InternalError(err).ToResp(), nil
	}

	// userIDs
	userIDs := make([]string, 0, len(pagingApproves.List))
	for _, n := range pagingApproves.List {
		userIDs = append(userIDs, n.Submitter, n.Approver)
	}
	userIDs = strutil.DedupSlice(userIDs, true)

	return httpserver.OkResp(*pagingApproves, userIDs)
}

// Approve列表时获取请求参数
func getListApprovesParam(r *http.Request) (*apistructs.ApproveListRequest, error) {
	// 获取企业Id
	orgIDStr := r.URL.Query().Get("orgId")
	if orgIDStr == "" {
		orgIDStr = r.Header.Get(httputil.OrgHeader)
		if orgIDStr == "" {
			return nil, errors.Errorf("invalid param, orgId is empty")
		}
	}
	orgID, err := strconv.ParseUint(orgIDStr, 10, 64)
	if err != nil {
		return nil, errors.Errorf("invalid param, orgId is invalid")
	}

	var status []string
	statusMap := r.URL.Query()
	if statusList, ok := statusMap["status"]; ok {
		for _, s := range statusList {
			if s != string(apistructs.ApprovalStatusPending) &&
				s != string(apistructs.ApprovalStatusApproved) &&
				s != string(apistructs.ApprovalStatusDeined) {
				return nil, errors.Errorf("status type error")
			}
			status = append(status, s)
		}
	}

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
	var id *int64
	id_str := r.URL.Query().Get("id")
	if id_str != "" {
		id_int, err := strconv.ParseInt(id_str, 10, 64)
		if err != nil {
			return nil, errors.Errorf("invalid param, id is invalid")
		}
		id = &id_int
	}

	return &apistructs.ApproveListRequest{
		OrgID:    orgID,
		Status:   status,
		PageNo:   pageNo,
		PageSize: pageSize,
		ID:       id,
	}, nil
}

// WatchApprovalStatusChanged 监听审批流状态变更，同步审批流状态至依赖方
func (e *Endpoints) WatchApprovalStatusChanged(ctx context.Context, r *http.Request, vars map[string]string) (httpserver.Responser, error) {
	var (
		event apistructs.ApprovalStatusChangedEvent
		err   error
	)
	if err = json.NewDecoder(r.Body).Decode(&event); err != nil {
		return apierrors.ErrApprovalStatusChanged.InvalidParameter(err).ToResp(), nil
	}
	logrus.Infof("approvalStatusChangedEvent: %+v", event)

	// 处理审批流状态变更通知
	switch event.Content.ApprovalType {
	case apistructs.ApproveCeritficate:
		err = e.appCertificate.ModifyApprovalStatus(int64(event.Content.ApprovalID), event.Content.ApprovalStatus)
	case apistructs.ApproveLibReference:
		err = e.libReference.UpdateApprovalStatus(event.Content.ApprovalID, event.Content.ApprovalStatus)
	}

	if err != nil {
		return apierrors.ErrApprovalStatusChanged.InternalError(err).ToResp(), nil
	}

	return httpserver.OkResp("handle success")
}

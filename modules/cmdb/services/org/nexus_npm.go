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

package org

import (
	"fmt"

	"github.com/erda-project/erda/apistructs"
	"github.com/erda-project/erda/modules/cmdb/conf"
	"github.com/erda-project/erda/modules/cmdb/model"
	"github.com/erda-project/erda/pkg/crypto/uuid"
	"github.com/erda-project/erda/pkg/nexus"
)

// ensureNexusNpmGroupOrgRepos
// 1. npm hosted org snapshot repo
// 2. npm hosted publisher release repo
// 3. npm proxy official publisher repo
// 4. npm proxy thirdparty repos
// 5. one npm group org repo
func (o *Org) ensureNexusNpmGroupOrgRepos(org *model.Org) error {
	var npmMemberRepos []*apistructs.NexusRepository

	// 1. npm hosted org snapshot repo
	npmHostedOrgSnapshotRepo, err := o.ensureNexusNpmHostedOrgSnapshotRepo(org)
	if err != nil {
		return err
	}
	npmMemberRepos = append(npmMemberRepos, npmHostedOrgSnapshotRepo)

	// 2. npm hosted publisher release repo
	publisherID := o.GetPublisherID(org.ID)
	var npmHostedPublisherReleaseRepo *apistructs.NexusRepository
	if publisherID > 0 {
		dbRepos, err := o.db.ListNexusRepositories(apistructs.NexusRepositoryListRequest{
			PublisherID: &[]uint64{uint64(publisherID)}[0],
			OrgID:       &[]uint64{uint64(org.ID)}[0],
			Formats:     []nexus.RepositoryFormat{nexus.RepositoryFormatNpm},
			Types:       []nexus.RepositoryType{nexus.RepositoryTypeHosted},
		})
		if err != nil {
			return err
		}
		if len(dbRepos) > 0 {
			npmHostedPublisherReleaseRepo = o.nexusSvc.ConvertRepo(dbRepos[0])
		}
	}
	if npmHostedPublisherReleaseRepo != nil {
		npmMemberRepos = append(npmMemberRepos, npmHostedPublisherReleaseRepo)
	}

	// 3. npm proxy official publisher repo
	// TODO

	// 4. npm proxy thirdpary repos
	thirdPartyDbRepos, err := o.db.ListNexusRepositories(apistructs.NexusRepositoryListRequest{
		OrgID:   &[]uint64{uint64(org.ID)}[0],
		Formats: []nexus.RepositoryFormat{nexus.RepositoryFormatNpm},
		Types:   []nexus.RepositoryType{nexus.RepositoryTypeProxy},
	})
	if err != nil {
		return err
	}
	npmMemberRepos = append(npmMemberRepos, o.nexusSvc.ConvertRepos(thirdPartyDbRepos)...)

	// 5. one npm group org repo
	_, err = o.ensureNexusNpmGroupOrgRepo(org, npmMemberRepos)
	return err
}

func (o *Org) ensureNexusNpmHostedOrgSnapshotRepo(org *model.Org) (*apistructs.NexusRepository, error) {
	nexusServer := nexus.Server{
		Addr:     conf.CentralNexusAddr(),
		Username: conf.CentralNexusUsername(),
		Password: conf.CentralNexusPassword(),
	}
	// ensure repo
	npmRepoName := nexus.MakeOrgRepoName(nexus.RepositoryFormatNpm, nexus.RepositoryTypeHosted, uint64(org.ID), "snapshot")
	repo, err := o.nexusSvc.EnsureRepository(apistructs.NexusRepositoryEnsureRequest{
		OrgID:       &[]uint64{uint64(org.ID)}[0],
		PublisherID: nil,
		ClusterName: conf.DiceClusterName(),
		NexusServer: nexusServer,
		NexusCreateRequest: nexus.NpmHostedRepositoryCreateRequest{
			HostedRepositoryCreateRequest: nexus.HostedRepositoryCreateRequest{
				BaseRepositoryCreateRequest: nexus.BaseRepositoryCreateRequest{
					Name:   npmRepoName,
					Online: true,
					Storage: nexus.HostedRepositoryStorageConfig{
						BlobStoreName:               npmRepoName,
						StrictContentTypeValidation: true,
						WritePolicy:                 nexus.RepositoryStorageWritePolicyAllowRedploy,
					},
					Cleanup: nil,
				},
			},
		},
		SyncConfigToPipelineCM: apistructs.NexusSyncConfigToPipelineCM{
			SyncOrg: &apistructs.NexusSyncConfigToPipelineCMItem{
				ConfigPrefix: "org.npm.snapshot.",
			},
		},
	})
	if err != nil {
		return nil, err
	}

	// ensure npm hosted org snapshot deployment user
	user, err := o.ensureNexusHostedOrgDeploymentUser(org, repo, apistructs.NexusSyncConfigToPipelineCM{
		SyncOrg: &apistructs.NexusSyncConfigToPipelineCMItem{
			ConfigPrefix: "org.npm.snapshot.deployment.",
		},
	})
	if err != nil {
		return nil, err
	}
	repo.User = user

	return repo, nil
}

func (o *Org) ensureNexusNpmGroupOrgRepo(org *model.Org, npmMemberRepos []*apistructs.NexusRepository) (*apistructs.NexusRepository, error) {
	npmGroupOrgRepoName := nexus.MakeOrgRepoName(nexus.RepositoryFormatNpm, nexus.RepositoryTypeGroup, uint64(org.ID))
	repo, err := o.nexusSvc.EnsureRepository(apistructs.NexusRepositoryEnsureRequest{
		OrgID:       &[]uint64{uint64(org.ID)}[0],
		PublisherID: nil,
		ClusterName: conf.DiceClusterName(),
		NexusServer: nexus.Server{
			Addr:     conf.CentralNexusAddr(),
			Username: conf.CentralNexusUsername(),
			Password: conf.CentralNexusPassword(),
		},
		NexusCreateRequest: nexus.NpmGroupRepositoryCreateRequest{
			GroupRepositoryCreateRequest: nexus.GroupRepositoryCreateRequest{
				BaseRepositoryCreateRequest: nexus.BaseRepositoryCreateRequest{
					Name:   npmGroupOrgRepoName,
					Online: true,
					Storage: nexus.HostedRepositoryStorageConfig{
						BlobStoreName:               npmGroupOrgRepoName,
						StrictContentTypeValidation: true,
						WritePolicy:                 nexus.RepositoryStorageWritePolicyReadOnly,
					},
					Cleanup: nil,
				},
				Group: nexus.RepositoryGroupConfig{
					MemberNames: func() []string {
						var memberNames []string
						for _, repo := range npmMemberRepos {
							if repo != nil {
								memberNames = append(memberNames, repo.Name)
							}
						}
						return memberNames
					}(),
				},
			},
		},
		SyncConfigToPipelineCM: apistructs.NexusSyncConfigToPipelineCM{
			SyncOrg: &apistructs.NexusSyncConfigToPipelineCMItem{
				ConfigPrefix: "org.npm.",
			},
		},
	})
	if err != nil {
		return nil, err
	}

	// ensure npm group org readonly user
	_, err = o.ensureNexusNpmGroupOrgReadonlyUser(org, repo, apistructs.NexusSyncConfigToPipelineCM{
		SyncOrg: &apistructs.NexusSyncConfigToPipelineCMItem{
			ConfigPrefix: "org.npm.readonly.",
		},
	})
	if err != nil {
		return nil, err
	}

	return repo, nil
}

func (o *Org) ensureNexusNpmGroupOrgReadonlyUser(
	org *model.Org,
	groupRepo *apistructs.NexusRepository,
	syncCM apistructs.NexusSyncConfigToPipelineCM,
) (*apistructs.NexusUser, error) {
	if groupRepo.OrgID == nil || *groupRepo.OrgID != uint64(org.ID) {
		return nil, fmt.Errorf("group repo's org id %d mismatch org id %d", groupRepo.OrgID, org.ID)
	}
	userName := nexus.MakeReadonlyUserName(groupRepo.Name)
	return o.nexusSvc.EnsureUser(apistructs.NexusUserEnsureRequest{
		ClusterName:            groupRepo.ClusterName,
		RepoID:                 &groupRepo.ID,
		OrgID:                  groupRepo.OrgID,
		UserName:               userName,
		Password:               uuid.UUID(),
		RepoPrivileges:         map[uint64][]nexus.PrivilegeAction{groupRepo.ID: nexus.RepoReadOnlyPrivileges},
		SyncConfigToPipelineCM: syncCM,
		NexusServer: nexus.Server{
			Addr:     conf.CentralNexusAddr(),
			Username: conf.CentralNexusUsername(),
			Password: conf.CentralNexusPassword(),
		},
		ForceUpdatePassword: true,
	})
}

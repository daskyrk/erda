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

package dao

type IssuePanel struct {
	BaseModel
	ProjectID uint64 `gorm:"column:project_id"`
	IssueID   int64  `gorm:"column:issue_id"`
	PanelName string `gorm:"column:panel_name"`
	Relation  int64  `gorm:"column:relation"`
}

func (IssuePanel) TableName() string {
	return "dice_issue_panel"
}

func (client *DBClient) CreatePanel(panel *IssuePanel) error {
	return client.Create(panel).Error
}

func (client *DBClient) DeletePanelByPanelID(panelID int64) error {
	return client.Where("id = ?", panelID).Delete(IssuePanel{}).Error
}

func (client *DBClient) UpdatePanel(panel *IssuePanel) error {
	return client.Save(panel).Error
}

// 通过看板ID获取看板详情
func (client *DBClient) GetPanelByPanelID(panelID int64) (*IssuePanel, error) {
	var panel IssuePanel
	if err := client.Where("id = ?", panelID).Find(&panel).Error; err != nil {
		return nil, err
	}
	return &panel, nil
}

// 获取项目下自定义看板的ID
func (client *DBClient) GetPanelsByProjectID(projectID uint64) ([]IssuePanel, error) {
	var panel []IssuePanel
	if err := client.Where("project_id = ?", projectID).Where("issue_id = ?", 0).Find(&panel).Error; err != nil {
		return nil, err
	}
	return panel, nil
}

// 查询事件所属的看板
func (client *DBClient) GetPanelByIssueID(issueID int64) (*IssuePanel, error) {
	var panel IssuePanel
	if err := client.Where("issue_id = ?", issueID).Find(&panel).Error; err != nil {
		return nil, err
	}
	return &panel, nil
}

// 通过PanelID获取看板下的全部issue
func (client *DBClient) GetPanelIssuesByPanel(panelID int64, pageNo, pageSize uint64) ([]IssuePanel, uint64, error) {
	var panel []IssuePanel
	var total uint64
	sql := client.Where("relation = ?", panelID)
	offset := (pageNo - 1) * pageSize
	if err := sql.Offset(offset).Limit(pageSize).Find(&panel).
		// reset offset & limit before count
		Offset(0).Limit(-1).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	return panel, total, nil
}

// 获取一个项目的一个看板中的事件ID
func (client *DBClient) GetPanelIssuesID(projectID uint64) ([]int64, error) {
	var ids []int64
	err := client.Where("project_id = ?", projectID).Not("issue_id", 0).Select("issue_id").Find(ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// 获取一个项目的全部自定义看板中的事件ID
func (client *DBClient) GetPanelIssuesIDByProjectID(projectID uint64) ([]IssuePanel, error) {
	var ids []IssuePanel
	err := client.Table("dice_issue_panel").Where("project_id = ?", projectID).Select("issue_id").Where("issue_id > 0").Scan(&ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}

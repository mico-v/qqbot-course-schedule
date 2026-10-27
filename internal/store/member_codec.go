package store

import (
	"encoding/json"
	"fmt"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

type memberMeta struct {
	Name              string `json:"name"`
	QQ                string `json:"qq,omitempty"`
	Schedule          string `json:"schedule"`
	Source            string `json:"source"`
	SourceFile        string `json:"source_file,omitempty"`
	UploaderID        string `json:"uploader_id,omitempty"`
	ImportGroupOpenID string `json:"import_group_openid,omitempty"`
	ImportUserOpenID  string `json:"import_user_openid,omitempty"`
	ImportUnionOpenID string `json:"import_union_openid,omitempty"`
	EventCount        int    `json:"event_count"`
	UpdatedAt         string `json:"updated_at"`
	ScheduleUpdatedAt string `json:"schedule_updated_at"`
	LastModifiedAt    string `json:"last_modified_at,omitempty"`
	LastModifiedBy    string `json:"last_modified_by,omitempty"`
	ICS               string `json:"ics"`
}

func encodeMember(member *schedule.Member) (string, []schedule.Event, error) {
	meta := memberMeta{
		Name:              member.Name,
		QQ:                member.QQ,
		Schedule:          member.Schedule,
		Source:            member.Source,
		SourceFile:        member.SourceFile,
		UploaderID:        member.UploaderID,
		EventCount:        len(member.Events),
		UpdatedAt:         member.UpdatedAt,
		ScheduleUpdatedAt: member.ScheduleUpdatedAt,
		LastModifiedAt:    member.LastModifiedAt,
		LastModifiedBy:    member.LastModifiedBy,
		ICS:               member.ICS,
		ImportGroupOpenID: member.ImportGroupOpenID,
		ImportUserOpenID:  member.ImportUserOpenID,
		ImportUnionOpenID: member.ImportUnionOpenID,
	}
	encoded, err := json.Marshal(meta)
	if err != nil {
		return "", nil, fmt.Errorf("编码成员数据失败: %w", err)
	}
	return string(encoded), member.Events, nil
}

func decodeMember(scopeID, userID, dataJSON string, revision int64, events []schedule.Event) (*schedule.Member, error) {
	var meta memberMeta
	if err := json.Unmarshal([]byte(dataJSON), &meta); err != nil {
		return nil, fmt.Errorf("解析成员 %s 数据失败: %w", userID, err)
	}
	kind, targetID := schedule.ParseScope(scopeID)
	importGroupOpenID := meta.ImportGroupOpenID
	importUserOpenID := meta.ImportUserOpenID
	importUnionOpenID := meta.ImportUnionOpenID
	if importGroupOpenID == "" && kind == "group" {
		importGroupOpenID = targetID
	}
	if importUserOpenID == "" && (kind == "group" || kind == "private") {
		importUserOpenID = userID
	}
	if importUnionOpenID == "" && kind == "global-user" && targetID == userID {
		importUnionOpenID = targetID
	}
	return &schedule.Member{
		UserID:            userID,
		Name:              meta.Name,
		QQ:                meta.QQ,
		Events:            events,
		ICS:               meta.ICS,
		Schedule:          meta.Schedule,
		Source:            meta.Source,
		SourceFile:        meta.SourceFile,
		UploaderID:        meta.UploaderID,
		EventCount:        meta.EventCount,
		UpdatedAt:         meta.UpdatedAt,
		ScheduleUpdatedAt: meta.ScheduleUpdatedAt,
		LastModifiedAt:    meta.LastModifiedAt,
		LastModifiedBy:    meta.LastModifiedBy,
		ImportGroupOpenID: importGroupOpenID,
		ImportUserOpenID:  importUserOpenID,
		ImportUnionOpenID: importUnionOpenID,
		Revision:          revision,
	}, nil
}

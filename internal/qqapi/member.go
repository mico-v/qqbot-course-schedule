package qqapi

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// GroupMember is one member returned by the group member API.
type GroupMember struct {
	MemberOpenID string `json:"member_openid"`
	Username     string `json:"username"`
	MemberRole   string `json:"member_role"`
	Bot          bool   `json:"bot"`
	JoinedAt     string `json:"joined_at"`
	UnionOpenID  string `json:"union_openid"`
}

// GetGroupMember fetches one group member, including union_openid when the
// platform grants that capability.
func (c *Client) GetGroupMember(ctx context.Context, groupOpenID, memberOpenID string) (*GroupMember, error) {
	path := fmt.Sprintf(
		"/v2/groups/%s/members/%s",
		url.PathEscape(groupOpenID),
		url.PathEscape(memberOpenID),
	)
	var member GroupMember
	if err := c.request(ctx, http.MethodGet, path, nil, &member, true); err != nil {
		return nil, err
	}
	return &member, nil
}

package qqapi

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

const maxGroupMemberPages = 100

// GroupMember is one member returned by the group member API.
type GroupMember struct {
	MemberOpenID string `json:"member_openid"`
	Username     string `json:"username"`
	MemberRole   string `json:"member_role"`
	Bot          bool   `json:"bot"`
	JoinedAt     string `json:"joined_at"`
	UnionOpenID  string `json:"union_openid"`
}

type groupMemberPage struct {
	Members    []GroupMember `json:"members"`
	NextCursor string        `json:"next_cursor"`
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

// ListGroupMembers fetches every page of a group's current member roster.
func (c *Client) ListGroupMembers(ctx context.Context, groupOpenID string) ([]GroupMember, error) {
	path := fmt.Sprintf("/v2/groups/%s/members", url.PathEscape(groupOpenID))
	cursor := ""
	seenCursors := make(map[string]bool)
	var members []GroupMember
	for pageNumber := 0; pageNumber < maxGroupMemberPages; pageNumber++ {
		requestPath := path
		if cursor != "" {
			requestPath += "?cursor=" + url.QueryEscape(cursor)
		}
		var page groupMemberPage
		if err := c.request(ctx, http.MethodGet, requestPath, nil, &page, true); err != nil {
			return nil, err
		}
		members = append(members, page.Members...)
		nextCursor := page.NextCursor
		if nextCursor == "" {
			return members, nil
		}
		if seenCursors[nextCursor] {
			return nil, fmt.Errorf("群成员分页游标重复")
		}
		seenCursors[nextCursor] = true
		cursor = nextCursor
	}
	return nil, fmt.Errorf("群成员分页超过 %d 页", maxGroupMemberPages)
}

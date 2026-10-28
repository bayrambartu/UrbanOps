package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestWorkflowRolesAndActions(t *testing.T) {
	techID, otherID := int64(10), int64(20)
	tests := []struct {
		name, from, role, action, status, note, want, code string
		actor                                              int64
		target                                             *int64
		requested                                          bool
	}{
		{name: "admin assigns", from: "open", role: "admin", actor: 1, status: "assigned", target: &techID, want: "assigned"},
		{name: "operator assigns", from: "open", role: "operator", actor: 1, status: "assigned", target: &techID, want: "assigned"},
		{name: "assignment needs technician", from: "open", role: "admin", actor: 1, status: "assigned", code: "invalid"},
		{name: "technician cannot assign", from: "open", role: "technician", actor: 10, status: "assigned", target: &otherID, code: "forbidden"},
		{name: "owner starts", from: "assigned", role: "technician", actor: 10, status: "in_progress", want: "in_progress"},
		{name: "manager cannot start on behalf", from: "assigned", role: "admin", actor: 1, status: "in_progress", code: "forbidden"},
		{name: "other technician cannot start", from: "assigned", role: "technician", actor: 20, status: "in_progress", code: "forbidden"},
		{name: "owner holds with reason", from: "in_progress", role: "technician", actor: 10, status: "on_hold", note: "Parça bekliyor", want: "on_hold"},
		{name: "hold requires reason", from: "in_progress", role: "technician", actor: 10, status: "on_hold", note: " ", code: "invalid"},
		{name: "owner resumes", from: "on_hold", role: "technician", actor: 10, status: "in_progress", want: "in_progress"},
		{name: "cannot solve without resuming", from: "on_hold", role: "technician", actor: 10, status: "resolved", note: "Çözüldü", code: "conflict"},
		{name: "resolution requires note", from: "in_progress", role: "technician", actor: 10, status: "resolved", code: "invalid"},
		{name: "owner resolves", from: "in_progress", role: "technician", actor: 10, status: "resolved", note: "Çözüm", want: "resolved"},
		{name: "manager cannot resolve on behalf", from: "in_progress", role: "operator", actor: 1, status: "resolved", note: "Çözüm", code: "forbidden"},
		{name: "manager approves", from: "resolved", role: "admin", actor: 1, status: "closed", want: "closed"},
		{name: "technician cannot approve", from: "resolved", role: "technician", actor: 10, status: "closed", code: "forbidden"},
		{name: "manager returns solution", from: "resolved", role: "operator", actor: 1, action: "return", note: "Eksik kontrol", want: "assigned"},
		{name: "return needs reason", from: "resolved", role: "admin", actor: 1, action: "return", code: "invalid"},
		{name: "cannot return closed", from: "closed", role: "admin", actor: 1, action: "return", note: "Eksik", code: "conflict"},
		{name: "cannot reassign resolved", from: "resolved", role: "admin", actor: 1, action: "reassign", note: "Gerekçe", target: &otherID, code: "conflict"},
		{name: "reassign active", from: "in_progress", role: "admin", actor: 1, action: "reassign", note: "Vardiya", target: &otherID, want: "assigned"},
		{name: "reassign held", from: "on_hold", role: "operator", actor: 1, action: "reassign", note: "Vardiya", target: &otherID, want: "assigned"},
		{name: "reassign same rejected", from: "assigned", role: "admin", actor: 1, action: "reassign", note: "Vardiya", target: &techID, code: "conflict"},
		{name: "reassign needs reason", from: "assigned", role: "admin", actor: 1, action: "reassign", target: &otherID, code: "invalid"},
		{name: "owner cannot directly transfer", from: "assigned", role: "technician", actor: 10, action: "reassign", note: "Vardiya", target: &otherID, code: "forbidden"},
		{name: "owner requests transfer", from: "on_hold", role: "technician", actor: 10, action: "request_reassignment", note: "Uzmanlık", want: "on_hold"},
		{name: "duplicate transfer blocked", from: "assigned", role: "technician", actor: 10, action: "request_reassignment", note: "Uzmanlık", requested: true, code: "conflict"},
		{name: "request cannot choose assignee", from: "assigned", role: "technician", actor: 10, action: "request_reassignment", note: "Uzmanlık", target: &otherID, code: "invalid"},
		{name: "closed request blocked", from: "closed", role: "technician", actor: 10, action: "request_reassignment", note: "Uzmanlık", code: "conflict"},
		{name: "ambiguous input", from: "assigned", role: "admin", actor: 1, action: "reassign", status: "assigned", note: "Gerekçe", target: &otherID, code: "invalid"},
		{name: "unknown action", from: "assigned", role: "admin", actor: 1, action: "delete", code: "invalid"},
		{name: "assignee injection", from: "assigned", role: "technician", actor: 10, status: "in_progress", target: &otherID, code: "invalid"},
		{name: "long note", from: "in_progress", role: "technician", actor: 10, status: "resolved", note: strings.Repeat("a", 10001), code: "invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			current := OrderState{tt.from, &techID, tt.requested}
			if tt.from == "open" {
				current.AssigneeID = nil
			}
			got, err := PlanOrderChange(current, Actor{tt.actor, tt.role}, OrderChange{Status: tt.status, Action: tt.action, AssigneeID: tt.target, Note: tt.note})
			if tt.code != "" {
				var policy *WorkflowError
				if !errors.As(err, &policy) || policy.Code != tt.code {
					t.Fatalf("got %v want %s", err, tt.code)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("got %s %v want %s", got, err, tt.want)
			}
		})
	}
}

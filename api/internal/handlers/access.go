package handlers

import (
	"context"
	"strings"

	"github.com/dfradehubs/agentgram-api/internal/auth"
	"github.com/dfradehubs/agentgram-api/internal/repository"
	"github.com/dfradehubs/agentgram-api/internal/service"
)

// IsGroupOwner checks if the user is the group owner (creator) or an admin.
// Use this for destructive/sensitive operations (delete, update permissions).
func IsGroupOwner(ctx context.Context, claims *auth.Claims, groupID string, groupRepo repository.GroupRepository, userService *service.UserService) bool {
	email := claims.GetEmail()
	userGroups := claims.GetGroups()

	isAdmin, _ := userService.IsAdmin(ctx, email, userGroups)
	if isAdmin {
		return true
	}

	group, err := groupRepo.Get(ctx, groupID)
	if err != nil {
		return false
	}

	return strings.EqualFold(group.CreatedBy, email)
}

// CanAccessGroup checks if the user (identified by claims) has access to the given group.
// Access is granted if the user is an admin, the group creator, listed in allowed users
// (including wildcard "*"), or belongs to an allowed group.
func CanAccessGroup(ctx context.Context, claims *auth.Claims, groupID string, groupRepo repository.GroupRepository, userService *service.UserService) bool {
	email := claims.GetEmail()
	userGroups := claims.GetGroups()

	isAdmin, _ := userService.IsAdmin(ctx, email, userGroups)
	if isAdmin {
		return true
	}

	group, err := groupRepo.Get(ctx, groupID)
	if err != nil {
		return false
	}

	// Creator always has access
	if strings.EqualFold(group.CreatedBy, email) {
		return true
	}

	// Check allowed users
	for _, u := range group.AllowedUsers {
		if u == "*" || strings.EqualFold(u, email) {
			return true
		}
	}

	// Check allowed groups
	for _, ag := range group.AllowedGroups {
		for _, ug := range userGroups {
			if strings.EqualFold(ag, ug) {
				return true
			}
		}
	}

	return false
}

// CanParticipateInGroup checks if the user is a real participant (not just admin).
// Admins can view group metadata but cannot access sessions unless they are actual members.
func CanParticipateInGroup(ctx context.Context, claims *auth.Claims, groupID string, groupRepo repository.GroupRepository) bool {
	email := claims.GetEmail()
	userGroups := claims.GetGroups()

	group, err := groupRepo.Get(ctx, groupID)
	if err != nil {
		return false
	}

	// Creator always participates
	if strings.EqualFold(group.CreatedBy, email) {
		return true
	}

	// Check allowed users (wildcard or explicit)
	for _, u := range group.AllowedUsers {
		if u == "*" || strings.EqualFold(u, email) {
			return true
		}
	}

	// Check allowed groups
	for _, ag := range group.AllowedGroups {
		for _, ug := range userGroups {
			if strings.EqualFold(ag, ug) {
				return true
			}
		}
	}

	return false
}

package members

import (
	"context"
	"fmt"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// ResolvedMember holds the ID and email of a resolved member.
type ResolvedMember struct {
	ID    string
	Email string
}

// DisplayString returns "email (id)" when the email is known, or just the ID otherwise.
func (r ResolvedMember) DisplayString() string {
	if r.Email != "" {
		return fmt.Sprintf("%s (%s)", r.Email, r.ID)
	}
	return r.ID
}

// ResolveMembers resolves a mix of member IDs and email addresses to ResolvedMember values.
// Arguments starting with "user_" are treated as member IDs.
// All other arguments are treated as email addresses and resolved via the members list API.
// The members list API is always called so that both email and ID are available for display.
func ResolveMembers(ctx context.Context, apiClient dash0api.Client, args []string) ([]ResolvedMember, error) {
	hasEmails := false
	for _, arg := range args {
		if !strings.HasPrefix(arg, "user_") {
			hasEmails = true
			break
		}
	}

	emailToID := make(map[string]string)
	idToEmail := make(map[string]string)
	iter := apiClient.ListMembersIter(ctx)
	for iter.Next() {
		vals := MemberValues(iter.Current(), "")
		if vals["email"] != "" && vals["id"] != "" {
			emailToID[vals["email"]] = vals["id"]
			idToEmail[vals["id"]] = vals["email"]
		}
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to list members: %w", err)
	}

	resolved := make([]ResolvedMember, 0, len(args))
	for _, arg := range args {
		if strings.HasPrefix(arg, "user_") {
			resolved = append(resolved, ResolvedMember{
				ID:    arg,
				Email: idToEmail[arg],
			})
		} else {
			if !hasEmails {
				continue
			}
			id, ok := emailToID[arg]
			if !ok {
				return nil, fmt.Errorf("no member found with email %q", arg)
			}
			resolved = append(resolved, ResolvedMember{
				ID:    id,
				Email: arg,
			})
		}
	}

	return resolved, nil
}

// ResolveToMemberIDs resolves a mix of member IDs and email addresses to member IDs.
// This is a convenience wrapper around ResolveMembers for callers that only need the IDs.
func ResolveToMemberIDs(ctx context.Context, apiClient dash0api.Client, args []string) ([]string, error) {
	resolved, err := ResolveMembers(ctx, apiClient, args)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(resolved))
	for i, r := range resolved {
		ids[i] = r.ID
	}
	return ids, nil
}

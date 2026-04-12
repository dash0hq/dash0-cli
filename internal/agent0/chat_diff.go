package agent0

// ContentDelta represents a change detected between two SSE snapshots.
type ContentDelta struct {
	Role    string
	Content string
	IsNew   bool // True if this is a brand-new message; false if an existing message was updated.
}

// DiffSnapshots compares two InvokeResponse snapshots and returns the new or changed messages.
// Each SSE event from agent0 contains the full conversation. This function extracts what changed
// by comparing message hashes and content between the previous and current snapshot.
func DiffSnapshots(prev, curr *InvokeResponse) []ContentDelta {
	if curr == nil {
		return nil
	}
	if prev == nil {
		return allMessagesAsDeltas(curr.Messages)
	}

	// Build a lookup of previous messages: hash -> content.
	// If a message has no hash, use its index as a fallback key.
	type prevEntry struct {
		content string
	}
	prevByHash := make(map[string]prevEntry, len(prev.Messages))
	for _, msg := range prev.Messages {
		if msg.Hash != "" {
			prevByHash[msg.Hash] = prevEntry{content: msg.Content}
		}
	}

	var deltas []ContentDelta
	for _, msg := range curr.Messages {
		if msg.Hash == "" {
			// Messages without a hash are always treated as new.
			deltas = append(deltas, ContentDelta{
				Role:    msg.Role,
				Content: msg.Content,
				IsNew:   true,
			})
			continue
		}

		prev, existed := prevByHash[msg.Hash]
		if !existed {
			// New message.
			deltas = append(deltas, ContentDelta{
				Role:    msg.Role,
				Content: msg.Content,
				IsNew:   true,
			})
		} else if msg.Content != prev.content {
			// Message content changed (e.g., assistant still streaming).
			deltas = append(deltas, ContentDelta{
				Role:    msg.Role,
				Content: msg.Content,
				IsNew:   false,
			})
		}
		// If hash exists and content is unchanged: no delta.
	}

	return deltas
}

func allMessagesAsDeltas(messages []Message) []ContentDelta {
	deltas := make([]ContentDelta, 0, len(messages))
	for _, msg := range messages {
		deltas = append(deltas, ContentDelta{
			Role:    msg.Role,
			Content: msg.Content,
			IsNew:   true,
		})
	}
	return deltas
}

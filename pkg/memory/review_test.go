package memory

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sipeed/picoclaw/pkg/providers"
)

func appendReviewTurn(t *testing.T, s *SQLite, key string, text string, at time.Time) ReviewBatch {
	t.Helper()
	ctx := t.Context()
	snap, err := s.ReadSession(ctx, key)
	require.NoError(t, err)
	_, err = s.AppendTurn(ctx, SessionWrite{
		Key: key, Private: true, Revision: snap.Revision,
		Review: &ReviewNote{Text: text, Source: "verified message", ChatID: key},
	}, []providers.Message{{Role: "user", Content: text}, {Role: "assistant", Content: "收到"}})
	require.NoError(t, err)
	_, err = s.db.Exec(
		"UPDATE memory_review_notes SET created=? WHERE id=(SELECT MAX(id) FROM memory_review_notes)",
		at.UnixMilli(),
	)
	require.NoError(t, err)
	batch, err := s.ReadReview(ctx, key)
	require.NoError(t, err)
	return batch
}

func TestReviewUpsertsTopicsAndHonorsPreferenceLimit(t *testing.T) {
	ctx := t.Context()
	path := Path(t.TempDir())
	s, err := Open(path)
	require.NoError(t, err)
	defer s.Close()
	for i := range PreferenceLimit {
		require.NoError(t, s.SetPreference(ctx, fmt.Sprintf("pref-%02d", i), "old", "manual"))
	}
	require.ErrorContains(t, s.SetPreference(ctx, "extra", "full", "manual"), "preference limit")
	now := time.Now().UTC()
	b1 := appendReviewTurn(t, s, "feishu-chat", "原神第三章，偏好更新", now.Add(time.Minute))
	r, err := s.ApplyReview(ctx, b1, []ReviewChange{
		{Kind: "preference", Key: "pref-00", Value: "new", Evidence: b1.Notes[0].ID},
		{Kind: "preference", Key: "extra", Value: "at capacity", Evidence: b1.Notes[0].ID},
		{
			Kind:     "record",
			Key:      "progress.genshin",
			Value:    "原神打到了第三章 Mondstadt",
			Category: "日常/游戏/进度",
			Date:     "2026-05-21",
			Evidence: b1.Notes[0].ID,
		},
	})
	require.NoError(t, err)
	require.Equal(t, ReviewResult{Applied: true, Preferences: 1, Records: 1}, r)
	p, err := s.Preferences(ctx)
	require.NoError(t, err)
	require.Len(t, p, PreferenceLimit)
	require.Contains(t, p, Preference{"pref-00", "new"})
	old, err := s.Search(ctx, Search{Query: "Mondstadt"})
	require.NoError(t, err)
	require.Len(t, old, 1)
	r, err = s.ApplyReview(ctx, b1, nil)
	require.NoError(t, err)
	require.False(t, r.Applied)

	// A different conversation in the same instance can update the
	// same logical record without changing its ID or duplicating FTS entries.
	b2 := appendReviewTurn(t, s, "weixin-chat", "原神第四章", now.Add(2*time.Minute))
	r, err = s.ApplyReview(ctx, b2, []ReviewChange{
		{
			Kind:     "record",
			Key:      "progress.genshin",
			Value:    "原神打到了第四章 Liyue",
			Date:     "2026-05-22",
			Evidence: b2.Notes[0].ID,
		},
	})
	require.NoError(t, err)
	require.Equal(t, ReviewResult{Applied: true, Records: 1}, r)
	hits, err := s.Search(ctx, Search{Query: "原神"})
	require.NoError(t, err)
	require.Len(t, hits, 1)
	require.Equal(t, old[0].ID, hits[0].ID)
	require.Equal(t, "2026-05-22", hits[0].Date)
	require.Equal(t, old[0].CreatedAt, hits[0].CreatedAt)
	require.Equal(t, "日常/游戏/进度", hits[0].Category)
	b3 := appendReviewTurn(t, s, "weixin-chat", "给游戏进度归类", now.Add(3*time.Minute))
	r, err = s.ApplyReview(ctx, b3, []ReviewChange{{
		Kind: "record", Key: "progress.genshin", Value: "原神打到了第四章 Liyue",
		Category: "日常/游戏", Date: "2026-05-22", Evidence: b3.Notes[0].ID,
	}})
	require.NoError(t, err)
	require.Equal(t, ReviewResult{Applied: true, Records: 1}, r)
	reclassified, err := s.Search(ctx, Search{Category: "日常/游戏"})
	require.NoError(t, err)
	require.Len(t, reclassified, 1)
	require.Equal(t, old[0].ID, reclassified[0].ID)
	require.Equal(t, "日常/游戏", reclassified[0].Category)
	stale, err := s.Search(ctx, Search{Query: "Mondstadt"})
	require.NoError(t, err)
	require.Empty(t, stale)
}

func TestReviewOnlyRunsOnDueChangesAndClearInvalidatesSnapshot(t *testing.T) {
	ctx := t.Context()
	s, err := Open(Path(t.TempDir()))
	require.NoError(t, err)
	defer s.Close()
	now := time.Now().UTC()
	appendReviewTurn(t, s, "idle", "third chapter", now.Add(-2*time.Minute))
	appendReviewTurn(t, s, "busy", "first message", now.Add(-10*time.Minute))
	appendReviewTurn(t, s, "busy", "new message", now)
	appendReviewTurn(t, s, "fresh", "not due", now)
	keys, err := s.DueReviews(ctx, now.Add(-2*time.Minute), now.Add(-10*time.Minute))
	require.NoError(t, err)
	require.Equal(t, []string{"idle", "busy"}, keys)
	b, err := s.ReadReview(ctx, "idle")
	require.NoError(t, err)
	require.NoError(t, s.ClearSession(ctx, SessionWrite{Key: "idle", Private: true}))
	appendReviewTurn(t, s, "idle", "new conversation", now)
	change := ReviewChange{
		Kind:     "record",
		Key:      "game.progress",
		Value:    "third chapter",
		Date:     "2026-05-21",
		Evidence: b.Notes[0].ID,
	}
	r, err := s.ApplyReview(ctx, b, []ReviewChange{change})
	require.NoError(t, err)
	require.False(t, r.Applied)
	rs, err := s.Export(ctx, 0)
	require.NoError(t, err)
	require.Empty(t, rs)

	// A model review started before a direct edit must be retried. Even on a
	// fresh read, older evidence cannot overwrite the newer direct preference.
	b, err = s.ReadReview(ctx, "busy")
	require.NoError(t, err)
	require.NoError(t, s.SetPreference(ctx, "style", "manual wins", "manual"))
	c := ReviewChange{Kind: "preference", Key: "style", Value: "older", Evidence: b.Notes[0].ID}
	r, err = s.ApplyReview(ctx, b, []ReviewChange{c})
	require.NoError(t, err)
	require.False(t, r.Applied)
	b, err = s.ReadReview(ctx, "busy")
	require.NoError(t, err)
	r, err = s.ApplyReview(ctx, b, []ReviewChange{c})
	require.NoError(t, err)
	require.Equal(t, ReviewResult{Applied: true}, r)
	keys, err = s.DueReviews(ctx, now.Add(time.Hour), now.Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, []string{"fresh", "idle"}, keys)
}

func TestExplicitDeletionCutsOffAlreadyQueuedReviewEvidence(t *testing.T) {
	ctx := t.Context()
	db, err := Open(Path(t.TempDir()))
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.SetPreference(ctx, "style", "简短", "cli"))
	id, err := db.Add(ctx, Record{Body: "原神进度：第三章", Date: "2026-05-21", Source: "cli"})
	require.NoError(t, err)
	now := time.Now().UTC()
	old := appendReviewTurn(t, db, "feishu", "我喜欢简短回答，原神到了第三章", now)
	changes := []ReviewChange{
		{Kind: "preference", Key: "style", Value: "简短", Evidence: old.Notes[0].ID},
		{Kind: "preference", Key: "language", Value: "中文", Evidence: old.Notes[0].ID},
		{Kind: "record", Key: "progress.genshin", Value: "原神到了第三章", Date: "2026-05-21", Evidence: old.Notes[0].ID},
	}
	deleted, err := db.DeletePreference(ctx, "style")
	require.NoError(t, err)
	require.True(t, deleted)
	deleted, err = db.Delete(ctx, id)
	require.NoError(t, err)
	require.True(t, deleted)
	res, err := db.ApplyReview(ctx, old, changes)
	require.NoError(t, err)
	require.False(t, res.Applied, "the review's state snapshot raced the explicit deletion")
	current, err := db.ReadReview(ctx, "feishu")
	require.NoError(t, err)
	res, err = db.ApplyReview(ctx, current, changes)
	require.NoError(t, err)
	require.Equal(t, ReviewResult{Applied: true, Preferences: 1}, res)
	prefs, err := db.Preferences(ctx)
	require.NoError(t, err)
	require.Equal(t, []Preference{{Key: "language", Value: "中文"}}, prefs)
	hits, err := db.Search(ctx, Search{Query: "原神"})
	require.NoError(t, err)
	require.Empty(t, hits)

	fresh := appendReviewTurn(t, db, "weixin", "原神到了第四章，我以后回答要简短", now.Add(time.Minute))
	changes = []ReviewChange{
		{Kind: "preference", Key: "style", Value: "简短", Evidence: fresh.Notes[0].ID},
		{Kind: "record", Key: "progress.genshin", Value: "原神到了第四章", Date: "2026-05-22", Evidence: fresh.Notes[0].ID},
	}
	res, err = db.ApplyReview(ctx, fresh, changes)
	require.NoError(t, err)
	require.Equal(t, ReviewResult{Applied: true, Preferences: 1, Records: 1}, res)
}

func TestExplicitPreferenceBatchRollsBackOnError(t *testing.T) {
	ctx := t.Context()
	db, err := Open(Path(t.TempDir()))
	require.NoError(t, err)
	defer db.Close()
	require.Error(t, db.SetPreferences(ctx, []Preference{
		{Key: "language", Value: "中文"}, {Key: "timezone", Value: "Not/A/Timezone"},
	}, "cli"))
	prefs, err := db.Preferences(ctx)
	require.NoError(t, err)
	require.Empty(t, prefs)
}

func TestRecordSelectionIsInvalidatedByConversationClear(t *testing.T) {
	ctx := t.Context()
	db, err := Open(Path(t.TempDir()))
	require.NoError(t, err)
	defer db.Close()
	id, err := db.Add(ctx, Record{Body: "原神原进度", Date: "2026-05-21", Source: "cli"})
	require.NoError(t, err)
	r, found, err := db.RecordByID(ctx, id)
	require.NoError(t, err)
	require.True(t, found)
	snapshot, err := db.ReadSession(ctx, "weixin")
	require.NoError(t, err)
	require.NoError(t, db.ClearSession(ctx, SessionWrite{Key: "weixin", Private: true}))
	match := RecordMatch{Version: r.UpdatedAt, SessionKey: "weixin", SessionRevision: snapshot.Revision}
	deleted, err := db.DeleteRecordMatch(ctx, id, match)
	require.NoError(t, err)
	require.False(t, deleted)
	updated, err := db.UpdateRecordMatch(ctx, id, match, Record{Body: "原神新进度"})
	require.NoError(t, err)
	require.False(t, updated)
}

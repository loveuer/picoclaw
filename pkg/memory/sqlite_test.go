package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSQLiteThreeLayersAndRestore(t *testing.T) {
	ctx := t.Context()
	dest := Path(t.TempDir())
	s, e := Open(dest)
	require.NoError(t, e)
	require.NoError(t, s.SetPreference(ctx, "language", "中文简洁", "test"))
	id, e := s.Add(
		ctx,
		Record{
			Body:    "昨天玩原神到了第三章",
			Date:    "2026-05-21",
			Source:  "daily.md:14-14",
			Aliases: []string{"Genshin"},
			Tags:    []string{"game"},
		},
	)
	require.NoError(t, e)
	en, e := s.Add(
		ctx,
		Record{Body: "Hollow Knight City of Tears", Date: "2026-05-22", Source: "daily.md:16-16"},
	)
	require.NoError(t, e)
	require.NotEqual(t, id, en)
	require.NoError(t, s.AddMessage(ctx, "chat", "user", "old context"))
	require.NoError(t, s.SetSummary(ctx, "chat", "old summary"))
	_, _, rev, e := s.Snapshot(ctx, "chat")
	require.NoError(t, e)
	ok, e := s.Replace(ctx, "chat", nil, "", nil)
	require.NoError(t, e)
	require.True(t, ok)
	ok, e = s.Replace(ctx, "chat", nil, "stale", &rev)
	require.NoError(t, e)
	require.False(t, ok)
	snapshot := filepath.Join(t.TempDir(), "backup.sqlite3")
	require.NoError(t, Backup(ctx, dest, snapshot))
	for _, q := range []Search{{Query: "原神"}, {Alias: "genshin", Tag: "game", From: "2026-05-21", To: "2026-05-21"}} {
		hits, searchErr := s.Search(ctx, q)
		require.NoError(t, searchErr)
		require.Len(t, hits, 1)
		require.Equal(t, id, hits[0].ID)
	}
	hits, e := s.Search(ctx, Search{Query: "Knight Tears"})
	require.NoError(t, e)
	require.Len(t, hits, 1)
	require.Equal(t, en, hits[0].ID)
	_, e = s.Search(ctx, Search{From: "invalid"})
	require.Error(t, e)
	ok, e = s.Update(ctx, en, Record{Body: "Celeste"})
	require.NoError(t, e)
	require.True(t, ok)
	hits, e = s.Search(ctx, Search{Query: "Knight"})
	require.NoError(t, e)
	require.Empty(t, hits)
	hits, e = s.Search(ctx, Search{Query: "Celeste"})
	require.NoError(t, e)
	require.Len(t, hits, 1)
	ok, e = s.Delete(ctx, en)
	require.NoError(t, e)
	require.True(t, ok)
	hits, e = s.Search(ctx, Search{Query: "Celeste"})
	require.NoError(t, e)
	require.Empty(t, hits)
	exported, e := s.Export(ctx, 0)
	require.NoError(t, e)
	require.Len(t, exported, 1)
	require.Equal(t, id, exported[0].ID)
	require.NoError(t, s.Close())
	before, e := Restore(ctx, dest, snapshot)
	require.NoError(t, e)
	require.FileExists(t, before)
	s, e = Open(dest)
	require.NoError(t, e)
	t.Cleanup(func() { _ = s.Close() })
	p, e := s.Preferences(ctx)
	require.NoError(t, e)
	require.Equal(t, []Preference{{Key: "language", Value: "中文简洁"}}, p)
	hits, e = s.Search(ctx, Search{Query: "Knight"})
	require.NoError(t, e)
	require.Len(t, hits, 1)
	h, sum, _, e := s.Snapshot(ctx, "chat")
	require.NoError(t, e)
	require.Empty(t, h)
	require.Empty(t, sum)
}

func TestMaintenanceLease(t *testing.T) {
	path := Path(t.TempDir())
	release, e := Lease(path, false)
	require.NoError(t, e)
	_, e = Lease(path, true)
	require.Error(t, e)
	release()
	free, e := Lease(path, true)
	require.NoError(t, e)
	defer free()
	_, e = Lease(path, false)
	require.Error(t, e)
}

func TestStableV1RejectsDevelopmentDatabasesWithoutWriting(t *testing.T) {
	ctx := t.Context()
	stable := Path(t.TempDir())
	db, err := Open(stable)
	require.NoError(t, err)
	var version int
	require.NoError(t, db.db.QueryRow("PRAGMA user_version").Scan(&version))
	require.Equal(t, 1, version)
	require.NoError(t, db.Close())

	for _, fixture := range []struct {
		name, ddl, message string
		version            int
	}{
		{"development-v4", "", "unsupported memory database version 4", 4},
		{"incompatible-v1", "", "unrecognized memory database format", 1},
		{"v1-before-category", `CREATE TABLE memory_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
INSERT INTO memory_meta VALUES ('schema','single-home-v1');
CREATE TABLE records (id INTEGER PRIMARY KEY, body TEXT NOT NULL);
INSERT INTO records VALUES (1,'preserve existing record');`, "without records.category", 1},
		{"unversioned-nonempty", "", "nonempty unversioned database", 0},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			path := Path(t.TempDir())
			old, e := openWritable(path)
			require.NoError(t, e)
			_, e = old.Exec(fmt.Sprintf(`CREATE TABLE old_data (value TEXT);
INSERT INTO old_data VALUES ('preserve this'); %s PRAGMA user_version=%d`, fixture.ddl, fixture.version))
			require.NoError(t, e)
			require.NoError(t, old.Close())
			before, e := os.ReadFile(path)
			require.NoError(t, e)
			_, e = Open(path)
			require.ErrorContains(t, e, fixture.message)
			_, e = Restore(ctx, stable, path)
			require.Error(t, e)
			after, e := os.ReadFile(path)
			require.NoError(t, e)
			require.Equal(t, before, after)
		})
	}
}

func TestChineseMemorySearchKeepsSubjectWhenSecondaryTermsDoNotMatch(t *testing.T) {
	ctx := t.Context()
	s, err := Open(Path(t.TempDir()))
	require.NoError(t, err)
	defer s.Close()
	want, err := s.Add(ctx, Record{
		Body: "《塞尔达传说：旷野之息》里这三个神庙的宝箱还没拿：协莫拉塔、吉兹卡夫伊、塔罗尼席。",
		Date: "2026-11-21", Source: "cli session original", Aliases: []string{"BOTW"},
	})
	require.NoError(t, err)
	other, err := s.Add(ctx, Record{Body: "游戏收集进度：原神任务全部完成", Date: "2026-11-22", Source: "other"})
	require.NoError(t, err)
	weak, err := s.Add(ctx, Record{Body: "另一个神庙有宝箱，已拿到", Date: "2026-11-23", Source: "other"})
	require.NoError(t, err)

	// The exact production query must return the older record first even
	// though it lacks three of the secondary terms.
	query := "宝箱 没拿 游戏 收集 进度"
	hits, err := s.Search(ctx, Search{Query: query})
	require.NoError(t, err)
	require.Len(t, hits, 2)
	require.Equal(t, want, hits[0].ID)
	require.Equal(t, "2026-11-21", hits[0].Date)
	require.Equal(t, "cli session original", hits[0].Source)
	require.Equal(t, weak, hits[1].ID)
	require.NotEqual(t, other, hits[1].ID)

	hits, err = s.Search(ctx, Search{Query: "宝箱，没拿"})
	require.NoError(t, err)
	require.Len(t, hits, 1)
	require.Equal(t, want, hits[0].ID)
	hits, err = s.Search(ctx, Search{Query: query, Alias: "BOTW", To: "2026-11-20"})
	require.NoError(t, err)
	require.Empty(t, hits)
	hits, err = s.Search(ctx, Search{Query: "弓箭 游戏 进度"})
	require.NoError(t, err)
	require.Empty(t, hits)
}

func TestCategorizedRecallUsesAlternativesWithoutDroppingFilters(t *testing.T) {
	ctx := t.Context()
	s, err := Open(Path(t.TempDir()))
	require.NoError(t, err)
	defer s.Close()
	game, err := s.Add(ctx, Record{
		Body: "《塞尔达传说：旷野之息》协莫拉塔神庙的宝箱还没拿。",
		Date: "2026-11-21", Source: "cli original",
	})
	require.NoError(t, err)
	_, err = s.Add(ctx, Record{
		Body: "搬家用的箱子放在门口。", Date: "2026-11-21", Source: "daily",
		Category: "日常/家务",
	})
	require.NoError(t, err)
	old, err := s.Add(ctx, Record{
		Body: "曙光营地库存还有一枚奖励。", Date: "2026-11-21", Source: "archive",
	})
	require.NoError(t, err)

	hits, err := s.Search(ctx, Search{Query: "箱子", Alternatives: []string{"宝箱"}, Category: "日常/游戏", From: "2026-11-21"})
	require.NoError(t, err)
	require.Len(t, hits, 1)
	require.Equal(t, game, hits[0].ID)
	require.Equal(t, "日常/游戏", hits[0].Category)
	require.ElementsMatch(t, []string{"宝箱", "神庙"}, hits[0].Tags)
	require.Contains(t, hits[0].Aliases, "BOTW")
	require.Equal(t, "cli original", hits[0].Source)
	hits, err = s.Search(ctx, Search{Query: "箱子", Alternatives: []string{"宝箱"}, Category: "日常", To: "2026-11-20"})
	require.NoError(t, err)
	require.Empty(t, hits)
	_, err = s.Add(ctx, Record{Body: "其他记录", Date: "2026-11-21", Source: "daily", Category: "日常备忘"})
	require.NoError(t, err)
	hits, err = s.Search(ctx, Search{Category: "日常"})
	require.NoError(t, err)
	require.Len(t, hits, 2)

	hits, err = s.Search(ctx, Search{
		Query: "营地奖励", Alternatives: []string{"曙光营地", "奖励", "曙光营地"}, To: "2026-11-21",
	})
	require.NoError(t, err)
	require.Len(t, hits, 1, "the same record found by multiple words is returned once")
	require.Equal(t, old, hits[0].ID)
	require.Empty(t, hits[0].Category)
	var uncategorized bool
	require.NoError(t, s.db.QueryRow("SELECT category IS NULL FROM records WHERE id=?", old).Scan(&uncategorized))
	require.True(t, uncategorized)
	updated, err := s.Update(ctx, game, Record{Body: "箱子都拿了。"})
	require.NoError(t, err)
	require.True(t, updated)
	exported, err := s.Export(ctx, 0)
	require.NoError(t, err)
	require.Len(t, exported, 1)
	require.Equal(t, game, exported[0].ID)
	require.Equal(t, "日常/游戏", exported[0].Category)
	require.Equal(t, "cli original", exported[0].Source)
}

func TestSearchAlternativesContributeEvenWhenPrimaryFillsLimit(t *testing.T) {
	ctx := t.Context()
	s, err := Open(Path(t.TempDir()))
	require.NoError(t, err)
	defer s.Close()
	for i := range 5 {
		_, err = s.Add(ctx, Record{
			Body: fmt.Sprintf("宝箱说明 %d", i), Date: "2026-11-22", Source: "general",
		})
		require.NoError(t, err)
	}
	relevant, err := s.Add(ctx, Record{Body: "曙光营地的隐藏奖励未领取", Date: "2026-11-21", Source: "progress"})
	require.NoError(t, err)
	hits, err := s.Search(ctx, Search{Query: "宝箱", Alternatives: []string{"曙光营地"}})
	require.NoError(t, err)
	require.Len(t, hits, 5)
	require.NotEqual(t, relevant, hits[0].ID, "direct subject still has priority")
	ids := make([]int64, 0, len(hits))
	for _, hit := range hits {
		ids = append(ids, hit.ID)
	}
	require.Contains(t, ids, relevant)
}

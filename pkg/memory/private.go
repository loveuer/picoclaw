package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type Preference struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

var preferenceKey = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

func validatePreference(key, value string) error {
	if !preferenceKey.MatchString(key) || strings.TrimSpace(value) == "" || utf8.RuneCountInString(value) > 160 {
		return errors.New("invalid preference key or value (maximum 160 characters)")
	}
	if key == "timezone" {
		if value == "Local" {
			return errors.New("use an IANA timezone")
		}
		if _, e := time.LoadLocation(value); e != nil {
			return errors.New("use an IANA timezone")
		}
	}
	return nil
}

func (s *SQLite) Preferences(ctx context.Context) ([]Preference, error) {
	rows, e := s.db.QueryContext(
		ctx,
		"SELECT key,value FROM preferences ORDER BY updated_at DESC,key LIMIT ?",
		PreferenceLimit,
	)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Preference{}
	for rows.Next() {
		var p Preference
		if e := rows.Scan(&p.Key, &p.Value); e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func putPreference(tx *sql.Tx, key, value, source string, overwrite bool) error {
	if e := validatePreference(key, value); e != nil {
		return e
	}
	stmt := `INSERT INTO preferences(key,value,source,updated_at) VALUES(?,?,?,?) ON CONFLICT(key) DO NOTHING`
	if overwrite {
		stmt = `INSERT INTO preferences(key,value,source,updated_at) VALUES(?,?,?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value, source=excluded.source, updated_at=excluded.updated_at, memory_evidence=NULL`
	}
	_, e := tx.Exec(stmt, key, value, source, time.Now().UTC().Format(time.RFC3339Nano))
	return e
}

func (s *SQLite) SetPreference(ctx context.Context, key, value, source string) error {
	return s.SetPreferences(ctx, []Preference{{Key: key, Value: value}}, source)
}

// SetPreferences commits a single explicit request atomically, including
// requests that change more than one preference at the current limit.
func (s *SQLite) SetPreferences(ctx context.Context, entries []Preference, source string) error {
	if len(entries) == 0 || len(entries) > PreferenceLimit {
		return errors.New("invalid number of preferences")
	}
	seen := make(map[string]bool, len(entries))
	return transaction(ctx, s.db, func(tx *sql.Tx) error {
		var count int
		if err := tx.QueryRow("SELECT count(*) FROM preferences").Scan(&count); err != nil {
			return err
		}
		for _, p := range entries {
			if seen[p.Key] {
				return errors.New("duplicate preference key")
			}
			seen[p.Key] = true
			var exists bool
			if err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM preferences WHERE key=?)", p.Key).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				if count >= PreferenceLimit {
					return fmt.Errorf("preference limit (%d) reached; delete a preference before adding another", PreferenceLimit)
				}
				count++
			}
			if err := putPreference(tx, p.Key, p.Value, source, true); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *SQLite) DeletePreference(ctx context.Context, key string) (bool, error) {
	return s.deleteWithReviewCutoff(ctx, RecordMatch{}, reviewPreferenceCutoff(key), "DELETE FROM preferences WHERE key=?", key)
}

func (s *SQLite) deleteWithReviewCutoff(ctx context.Context, match RecordMatch, cutoff, query string, args ...any) (bool, error) {
	var deleted bool
	err := transaction(ctx, s.db, func(tx *sql.Tx) error {
		matches, err := match.sessionMatches(tx)
		if err != nil || !matches {
			return err
		}
		deleted, err = changed(tx.Exec(query, args...))
		if err != nil || !deleted {
			return err
		}
		// Review note IDs are AUTOINCREMENT. A same-transaction high water mark
		// includes every already queued note, even one held by a running review.
		_, err = tx.Exec(`INSERT INTO memory_meta(key,value)
VALUES (?,COALESCE((SELECT seq FROM sqlite_sequence WHERE name='memory_review_notes'),0))
ON CONFLICT(key) DO UPDATE SET value=excluded.value`, cutoff)
		return err
	})
	return deleted && err == nil, err
}

func (s *SQLite) Location(ctx context.Context) (*time.Location, error) {
	var v string
	e := s.db.QueryRowContext(ctx, "SELECT value FROM preferences WHERE key='timezone'").
		Scan(&v)
	if errors.Is(e, sql.ErrNoRows) {
		return time.Local, nil
	}
	if e != nil {
		return nil, e
	}
	return time.LoadLocation(v)
}

type Record struct {
	ID        int64    `json:"id"`
	Body      string   `json:"text"`
	Date      string   `json:"date"`
	Category  string   `json:"category,omitempty"`
	Aliases   []string `json:"aliases,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Source    string   `json:"source"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

func validDate(s string) bool { _, e := time.Parse(time.DateOnly, s); return e == nil }
func validateRecord(r Record) error {
	if strings.TrimSpace(r.Body) == "" || utf8.RuneCountInString(r.Body) > 2000 || !validDate(r.Date) ||
		r.Source == "" ||
		utf8.RuneCountInString(r.Source) > 256 {
		return errors.New("invalid record (text maximum 2000 characters; ISO date and source required)")
	}
	if err := validateCategory(r.Category); err != nil {
		return err
	}
	return validateRecordLabels(r.Aliases, r.Tags)
}

func validateRecordLabels(labels ...[]string) error {
	for _, xs := range labels {
		if len(xs) > maxRecordLabels {
			return errors.New("too many tags or aliases")
		}
		for _, x := range xs {
			if strings.TrimSpace(x) == "" || utf8.RuneCountInString(x) > 48 {
				return errors.New("invalid tag or alias")
			}
		}
	}
	return nil
}

// OtherCategory is the fallback for a newly saved explicit record whose
// category could not be determined locally.
const OtherCategory = "other"

func insertRecord(tx *sql.Tx, r Record) (int64, error) {
	saved, _, err := insertRecordWithCategory(tx, r, false)
	return saved.ID, err
}

func insertRecordWithCategory(tx *sql.Tx, r Record, immediate bool) (Record, bool, error) {
	if err := validateRecord(r); err != nil {
		return Record{}, false, err
	}
	r = enrichRecordLabels(r)
	needsClassification := immediate && r.Category == ""
	if needsClassification {
		r.Category = OtherCategory
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if r.CreatedAt == "" {
		r.CreatedAt = now
	}
	if r.UpdatedAt == "" {
		r.UpdatedAt = now
	}
	aliases, _ := json.Marshal(r.Aliases)
	tags, _ := json.Marshal(r.Tags)
	res, err := tx.Exec(
		"INSERT INTO records(body,event_date,category,aliases,tags,source,created_at,updated_at) VALUES(?,?,NULLIF(?,''),?,?,?,?,?)",
		r.Body, r.Date, r.Category, string(aliases), string(tags), r.Source, r.CreatedAt, r.UpdatedAt,
	)
	if err != nil {
		return Record{}, false, err
	}
	r.ID, err = res.LastInsertId()
	return r, needsClassification, err
}

func (s *SQLite) Add(ctx context.Context, r Record) (id int64, err error) {
	err = transaction(ctx, s.db, func(tx *sql.Tx) error { var e error; id, e = insertRecord(tx, r); return e })
	return
}

// AddImmediate returns the snapshot written by the insert itself. The caller
// must use this version rather than rereading a record that may have been edited.
func (s *SQLite) AddImmediate(ctx context.Context, r Record) (Record, bool, error) {
	var saved Record
	var needsClassification bool
	err := transaction(ctx, s.db, func(tx *sql.Tx) error {
		var err error
		saved, needsClassification, err = insertRecordWithCategory(tx, r, true)
		return err
	})
	if err != nil {
		return Record{}, false, err
	}
	return saved, needsClassification, nil
}

type RecordLabels struct {
	Category string   `json:"category"`
	Tags     []string `json:"tags,omitempty"`
	Aliases  []string `json:"aliases,omitempty"`
}

// ClassifyImmediate updates only classification metadata, provided the
// initially committed version is still current.
func (s *SQLite) ClassifyImmediate(ctx context.Context, id int64, version string, labels RecordLabels) (bool, error) {
	if id <= 0 || version == "" || labels.Category == "" || strings.EqualFold(labels.Category, OtherCategory) {
		return false, errors.New("invalid immediate record classification")
	}
	if err := validateCategory(labels.Category); err != nil {
		return false, err
	}
	if err := validateRecordLabels(labels.Tags, labels.Aliases); err != nil {
		return false, err
	}
	var applied bool
	err := transaction(ctx, s.db, func(tx *sql.Tx) error {
		var encodedTags, encodedAliases []byte
		err := tx.QueryRow(
			"SELECT tags,aliases FROM records WHERE id=? AND updated_at=? AND category=?",
			id, version, OtherCategory,
		).Scan(&encodedTags, &encodedAliases)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		var tags, aliases []string
		if err = json.Unmarshal(encodedTags, &tags); err != nil {
			return err
		}
		if err = json.Unmarshal(encodedAliases, &aliases); err != nil {
			return err
		}
		tags = addLabels(tags, labels.Tags...)
		aliases = addLabels(aliases, labels.Aliases...)
		encodedTags, _ = json.Marshal(tags)
		encodedAliases, _ = json.Marshal(aliases)
		applied, err = changed(tx.Exec(
			"UPDATE records SET category=?, tags=?, aliases=?, updated_at=? WHERE id=? AND updated_at=? AND category=?",
			labels.Category, string(encodedTags), string(encodedAliases), nextRecordVersion(version), id, version, OtherCategory,
		))
		return err
	})
	return applied && err == nil, err
}

func nextRecordVersion(previous string) string {
	now := time.Now().UTC()
	if prior, err := time.Parse(time.RFC3339Nano, previous); err == nil && !now.After(prior) {
		now = prior.Add(time.Nanosecond)
	}
	return now.Format(time.RFC3339Nano)
}

func changed(r sql.Result, e error) (bool, error) {
	if e != nil {
		return false, e
	}
	n, e := r.RowsAffected()
	return n > 0, e
}

func (s *SQLite) Update(
	ctx context.Context,
	id int64,
	patch Record,
) (bool, error) {
	return s.UpdateRecordMatch(ctx, id, RecordMatch{}, patch)
}

// RecordMatch guards a write against changes to a searched record and,
// for a follow-up selection, changes to the conversation since it was offered.
type RecordMatch struct {
	Version         string
	SessionKey      string
	SessionRevision int64
}

func (m RecordMatch) sessionMatches(tx *sql.Tx) (bool, error) {
	if m.SessionKey == "" {
		return true, nil
	}
	revision, err := currentRevision(tx, m.SessionKey)
	return revision == m.SessionRevision, err
}

func (s *SQLite) UpdateRecordMatch(ctx context.Context, id int64, match RecordMatch, patch Record) (bool, error) {
	var applied bool
	err := transaction(ctx, s.db, func(tx *sql.Tx) error {
		matches, e := match.sessionMatches(tx)
		if e != nil || !matches {
			return e
		}
		var r Record
		var a, t string
		e = tx.QueryRow("SELECT event_date,source,coalesce(category,''),aliases,tags,updated_at FROM records WHERE id=? AND (?='' OR updated_at=?)", id, match.Version, match.Version).
			Scan(&r.Date, &r.Source, &r.Category, &a, &t, &r.UpdatedAt)
		if errors.Is(e, sql.ErrNoRows) {
			return nil
		}
		if e != nil {
			return e
		}
		r.Body = patch.Body
		if patch.Date != "" {
			r.Date = patch.Date
		}
		if patch.Category != "" {
			r.Category = patch.Category
		}
		if e = json.Unmarshal([]byte(a), &r.Aliases); e != nil {
			return e
		}
		if e = json.Unmarshal([]byte(t), &r.Tags); e != nil {
			return e
		}
		if patch.Aliases != nil {
			r.Aliases = patch.Aliases
		}
		if patch.Tags != nil {
			r.Tags = patch.Tags
		}
		if e = validateRecord(r); e != nil {
			return e
		}
		ab, _ := json.Marshal(r.Aliases)
		tb, _ := json.Marshal(r.Tags)
		applied, e = changed(
			tx.Exec(
				"UPDATE records SET body=?, event_date=?, category=NULLIF(?,''), aliases=?, tags=?, updated_at=?, memory_evidence=NULL WHERE id=?",
				r.Body,
				r.Date,
				r.Category,
				string(ab),
				string(tb),
				nextRecordVersion(r.UpdatedAt),
				id,
			),
		)
		return e
	})
	return applied && err == nil, err
}

func (s *SQLite) Delete(ctx context.Context, id int64) (bool, error) {
	return s.DeleteRecordMatch(ctx, id, RecordMatch{})
}

const recordReviewCutoff = "review_deleted_record"

func reviewPreferenceCutoff(key string) string { return "review_deleted_preference:" + key }

func (s *SQLite) DeleteRecordMatch(ctx context.Context, id int64, match RecordMatch) (bool, error) {
	return s.deleteWithReviewCutoff(ctx, match, recordReviewCutoff,
		"DELETE FROM records WHERE id=? AND (?='' OR updated_at=?)", id, match.Version, match.Version)
}

func (s *SQLite) RecordByID(ctx context.Context, id int64) (Record, bool, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+recordColumns+" FROM records r WHERE r.id=?", id)
	if err != nil {
		return Record{}, false, err
	}
	records, err := readRecords(rows)
	if err != nil || len(records) == 0 {
		return Record{}, false, err
	}
	return records[0], true, nil
}

type Search struct {
	Query, Alias, Tag, Category, From, To string
	Alternatives                          []string
}

const recordColumns = "r.id,r.body,r.event_date,coalesce(r.category,''),r.aliases,r.tags,r.source,r.created_at,r.updated_at"

func readRecords(rows *sql.Rows) ([]Record, error) {
	defer rows.Close()
	out := []Record{}
	for rows.Next() {
		var r Record
		var a, t string
		if e := rows.Scan(&r.ID, &r.Body, &r.Date, &r.Category, &a, &t, &r.Source, &r.CreatedAt, &r.UpdatedAt); e != nil {
			return nil, e
		}
		if e := json.Unmarshal([]byte(a), &r.Aliases); e != nil {
			return nil, e
		}
		if e := json.Unmarshal([]byte(t), &r.Tags); e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *SQLite) Export(ctx context.Context, after int64) ([]Record, error) {
	rows, e := s.db.QueryContext(
		ctx,
		"SELECT "+recordColumns+" FROM records r WHERE r.id>? ORDER BY r.id LIMIT 1",
		after,
	)
	if e != nil {
		return nil, e
	}
	return readRecords(rows)
}

func (s *SQLite) Search(ctx context.Context, q Search) ([]Record, error) {
	if (q.From != "" && !validDate(q.From)) || (q.To != "" && !validDate(q.To)) ||
		(q.From != "" && q.To != "" && q.From > q.To) {
		return nil, errors.New("invalid date range")
	}
	if utf8.RuneCountInString(q.Query) > 160 || utf8.RuneCountInString(q.Alias) > 48 ||
		utf8.RuneCountInString(q.Tag) > 48 {
		return nil, errors.New("search input exceeds limits")
	}
	if len(q.Alternatives) > MaxSearchAlternatives {
		return nil, errors.New("too many search alternatives")
	}
	if err := validateCategory(q.Category); err != nil {
		return nil, err
	}
	for _, term := range q.Alternatives {
		if utf8.RuneCountInString(term) > 160 {
			return nil, errors.New("search alternative exceeds limits")
		}
	}
	variants := q.Queries()
	if len(variants) == 0 {
		return s.searchRecords(ctx, q, "")
	}
	if len(variants) == 1 {
		return s.searchRecords(ctx, q, variants[0])
	}
	type candidate struct {
		record  Record
		score   int
		primary bool
	}
	var candidates []candidate
	indexes := make(map[int64]int)
	var firstPrimary int64
	for i, variant := range variants {
		matches, err := s.searchRecords(ctx, q, variant)
		if err != nil {
			return nil, err
		}
		// Give the original query extra weight without crowding out a
		// strong alternate just because the original had five results.
		weight := 12
		if i == 0 && strings.TrimSpace(q.Query) != "" {
			weight *= 2
			if len(matches) > 0 {
				firstPrimary = matches[0].ID
			}
		}
		for rank, r := range matches {
			index, exists := indexes[r.ID]
			if !exists {
				index = len(candidates)
				indexes[r.ID] = index
				candidates = append(candidates, candidate{record: r, primary: i == 0})
			}
			candidates[index].score += weight / (rank + 1)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if (a.record.ID == firstPrimary) != (b.record.ID == firstPrimary) {
			return a.record.ID == firstPrimary
		}
		if a.score != b.score {
			return a.score > b.score
		}
		if a.primary != b.primary {
			return a.primary
		}
		if a.record.Date != b.record.Date {
			return a.record.Date > b.record.Date
		}
		return a.record.ID > b.record.ID
	})
	hits := make([]Record, 0, min(5, len(candidates)))
	for _, c := range candidates[:min(5, len(candidates))] {
		hits = append(hits, c.record)
	}
	return hits, nil
}

func (s *SQLite) searchRecords(ctx context.Context, q Search, text string) ([]Record, error) {
	where := " WHERE 1=1"
	var args []any
	for _, f := range []struct{ sql, val string }{
		{" AND r.event_date>=?", q.From},
		{" AND r.event_date<=?", q.To},
		{" AND EXISTS (SELECT 1 FROM json_each(r.aliases) WHERE lower(value)=lower(?))", q.Alias},
		{" AND EXISTS (SELECT 1 FROM json_each(r.tags) WHERE lower(value)=lower(?))", q.Tag},
	} {
		if f.val != "" {
			where += f.sql
			args = append(args, f.val)
		}
	}
	if q.Category != "" {
		// In binary order '/' is followed by '0'. This prefix range includes
		// descendants and allows SQLite to use the category index.
		where += " AND (r.category=? OR (r.category>=? AND r.category<?))"
		args = append(args, q.Category, q.Category+"/", q.Category+"0")
	}
	if text == "" {
		rows, e := s.db.QueryContext(
			ctx,
			"SELECT "+recordColumns+" FROM records r"+where+" ORDER BY r.event_date DESC,r.id DESC LIMIT 5",
			args...)
		if e != nil {
			return nil, e
		}
		return readRecords(rows)
	}
	// Quote every term rather than accepting the FTS expression language.
	terms := strings.Fields(text)
	for i, t := range terms {
		terms[i] = `"` + strings.ReplaceAll(t, `"`, `""`) + `"`
	}
	ftsArgs := append(append([]any{}, args...), strings.Join(terms, " AND "))
	rows, e := s.db.QueryContext(
		ctx,
		"SELECT "+recordColumns+" FROM records r JOIN records_fts ON records_fts.rowid=r.id"+where+" AND records_fts MATCH ? ORDER BY bm25(records_fts),r.event_date DESC,r.id DESC LIMIT 5",
		ftsArgs...)
	if e != nil {
		return nil, e
	}
	hits, e := readRecords(rows)
	if e != nil || len(hits) == 5 {
		return hits, e
	}
	hasHan := strings.IndexFunc(text, func(r rune) bool { return unicode.Is(unicode.Han, r) }) >= 0
	if !hasHan {
		return hits, nil
	}
	// Unicode61 can treat an entire unsegmented Chinese sentence as one
	// token. Match each keyword as a substring, rather than matching the
	// space-separated query as one literal phrase.
	terms = strings.FieldsFunc(text, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsPunct(r) })
	var keywords []string
	seen := make(map[string]bool)
	for _, term := range terms {
		term = strings.ToLower(term)
		if !seen[term] {
			keywords = append(keywords, term)
			seen[term] = true
		}
	}
	if len(keywords) == 0 {
		return hits, nil
	}
	for _, r := range hits {
		where += " AND r.id!=?"
		args = append(args, r.ID)
	}
	strictWhere := where
	strictArgs := append([]any(nil), args...)
	for _, keyword := range keywords {
		strictWhere += " AND " + keywordSubstring
		strictArgs = appendKeywordArgs(strictArgs, keyword)
	}
	more, e := s.substringRecords(ctx, strictWhere, "", 5-len(hits), strictArgs)
	hits = append(hits, more...)
	if e != nil || len(hits) != 0 || len(keywords) < 2 {
		return hits, e
	}

	// On an empty multi-keyword search, keep the first (main subject)
	// keyword mandatory and use the other terms only to rank candidates.
	// Date, alias and tag filters still apply.
	where += " AND " + keywordSubstring
	args = appendKeywordArgs(args, keywords[0])
	var score []string
	for _, keyword := range keywords[1:] {
		score = append(score, "CASE WHEN "+keywordSubstring+" THEN 1 ELSE 0 END")
		args = appendKeywordArgs(args, keyword)
	}
	return s.substringRecords(ctx, where, "("+strings.Join(score, "+")+") DESC,", 5, args)
}

const keywordSubstring = "(instr(lower(r.body),?)>0 OR instr(lower(r.aliases),?)>0 OR instr(lower(r.tags),?)>0)"

func appendKeywordArgs(args []any, keyword string) []any {
	return append(args, keyword, keyword, keyword)
}

func (s *SQLite) substringRecords(ctx context.Context, where, order string, limit int, args []any) ([]Record, error) {
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+recordColumns+" FROM records r"+where+" ORDER BY "+order+"r.event_date DESC,r.id DESC LIMIT ?",
		args...,
	)
	if err != nil {
		return nil, err
	}
	return readRecords(rows)
}

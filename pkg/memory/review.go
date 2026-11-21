package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
)

const (
	PreferenceLimit = 12
	reviewBatchSize = 20
)

type ReviewNote struct {
	ID      int64          `json:"id"`
	At      time.Time      `json:"at"`
	Text    string         `json:"text"`
	Source  string         `json:"-"`
	Channel string         `json:"-"`
	ChatID  string         `json:"-"`
	Sender  bus.SenderInfo `json:"-"`
}

type ReviewPreference struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Version string `json:"-"`
}

type ReviewRecord struct {
	Topic    string `json:"topic"`
	Body     string `json:"text"`
	Date     string `json:"date"`
	Category string `json:"category,omitempty"`
	Version  string `json:"-"`
}

type ReviewBatch struct {
	SessionKey  string
	Notes       []ReviewNote
	Preferences []ReviewPreference
	Records     []ReviewRecord
}

type ReviewChange struct {
	Kind     string   `json:"kind"`
	Key      string   `json:"key"`
	Value    string   `json:"value"`
	Date     string   `json:"date,omitempty"`
	Evidence int64    `json:"evidence"`
	Category string   `json:"category,omitempty"`
	Aliases  []string `json:"aliases,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

type ReviewResult struct {
	Applied              bool
	Preferences, Records int
}

func enqueueReview(tx *sql.Tx, key string, n ReviewNote) error {
	sender, err := json.Marshal(n.Sender)
	if err != nil {
		return err
	}
	_, err = tx.Exec(
		"INSERT INTO memory_review_notes(session_key,created,body,source,channel,chat_id,sender) VALUES(?,?,?,?,?,?,?)",
		key,
		time.Now().UnixMilli(),
		n.Text,
		n.Source,
		n.Channel,
		n.ChatID,
		string(sender),
	)
	if err != nil {
		return err
	}
	// Analysis is opportunistic; keep the newest 200 pending turns even if
	// Gateway has been offline or its review model has stopped responding.
	_, err = tx.Exec(`DELETE FROM memory_review_notes WHERE session_key=? AND id <
 (SELECT id FROM memory_review_notes WHERE session_key=? ORDER BY id DESC LIMIT 1 OFFSET 199)`, key, key)
	return err
}

// DueReviews returns the oldest eligible sessions, excluding sessions in
// backoff before applying the batch limit.
func (s *SQLite) DueReviews(ctx context.Context, idleBefore, oldestBefore time.Time, excluded ...string) ([]string, error) {
	// Bind one JSON array so backoff size does not count toward SQLite's
	// limit on query parameters. A nil slice encodes as null, which the
	// type filter also excludes.
	skipped, _ := json.Marshal(excluded)
	rows, err := s.db.QueryContext(ctx, `
SELECT session_key FROM memory_review_notes
WHERE session_key NOT IN (SELECT value FROM json_each(?) WHERE type='text')
GROUP BY session_key
HAVING MAX(created)<=? OR MIN(created)<=?
ORDER BY MIN(id) LIMIT 16`, string(skipped), idleBefore.UnixMilli(), oldestBefore.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var k string
		if err = rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (s *SQLite) ReadReview(ctx context.Context, key string) (b ReviewBatch, err error) {
	err = transaction(ctx, s.db, func(tx *sql.Tx) error {
		b.SessionKey = key
		if e := readReviewNotes(tx, &b); e != nil || len(b.Notes) == 0 {
			return e
		}
		var e error
		b.Preferences, b.Records, e = reviewState(tx)
		return e
	})
	return b, err
}

func readReviewNotes(tx *sql.Tx, b *ReviewBatch) error {
	rows, err := tx.Query(
		"SELECT id,created,body,source,channel,chat_id,sender FROM memory_review_notes WHERE session_key=? ORDER BY id LIMIT ?",
		b.SessionKey,
		reviewBatchSize,
	)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var note ReviewNote
		var ts int64
		var sender []byte
		if err = rows.Scan(&note.ID, &ts, &note.Text, &note.Source, &note.Channel, &note.ChatID, &sender); err != nil {
			return err
		}
		if err = json.Unmarshal(sender, &note.Sender); err != nil {
			return err
		}
		note.At = time.UnixMilli(ts).UTC()
		b.Notes = append(b.Notes, note)
	}
	return rows.Err()
}

func reviewRows[T any](tx *sql.Tx, query string, scan func(*sql.Rows, *T) error, args ...any) ([]T, error) {
	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []T
	for rows.Next() {
		var item T
		if err = scan(rows, &item); err != nil {
			return nil, err
		}
		values = append(values, item)
	}
	return values, rows.Err()
}

func reviewState(tx *sql.Tx) ([]ReviewPreference, []ReviewRecord, error) {
	prefs, err := reviewRows(
		tx,
		"SELECT key,value,updated_at FROM preferences ORDER BY key LIMIT ?",
		func(rows *sql.Rows, p *ReviewPreference) error { return rows.Scan(&p.Key, &p.Value, &p.Version) },
		PreferenceLimit+1,
	)
	if err != nil {
		return nil, nil, err
	}
	records, err := reviewRows(
		tx,
		`SELECT memory_topic,body,event_date,coalesce(category,''),updated_at FROM records
WHERE memory_topic IS NOT NULL ORDER BY updated_at DESC,id DESC LIMIT 30`,
		func(rows *sql.Rows, r *ReviewRecord) error {
			return rows.Scan(&r.Topic, &r.Body, &r.Date, &r.Category, &r.Version)
		},
	)
	return prefs, records, err
}

var memoryTopic = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,95}$`)

// ApplyReview acknowledges the exact batch and commits changes in one
// transaction. Cleared or previously processed notes cannot recreate memory.
func (s *SQLite) ApplyReview(ctx context.Context, batch ReviewBatch, changes []ReviewChange) (ReviewResult, error) {
	var result ReviewResult
	if len(batch.Notes) == 0 || len(changes) > 8 {
		return result, errors.New("invalid review batch or number of changes")
	}
	notes := make(map[int64]ReviewNote, len(batch.Notes))
	for _, n := range batch.Notes {
		notes[n.ID] = n
	}
	seen := make(map[string]bool)
	for _, c := range changes {
		n, ok := notes[c.Evidence]
		key := c.Kind + ":" + c.Key
		if !ok || seen[key] {
			return result, errors.New("review needs unique changes and valid evidence")
		}
		seen[key] = true
		switch c.Kind {
		case "preference":
			if c.Date != "" || c.Category != "" || len(c.Aliases) != 0 {
				return result, errors.New("preference must not carry record metadata")
			}
			if err := validatePreference(c.Key, c.Value); err != nil {
				return result, err
			}
		case "record":
			if !memoryTopic.MatchString(c.Key) {
				return result, errors.New("invalid record topic")
			}
			if _, err := reviewedRecord(c, n); err != nil {
				return result, err
			}
		default:
			return result, errors.New("invalid review change kind")
		}
	}
	first, last := batch.Notes[0].ID, batch.Notes[len(batch.Notes)-1].ID
	err := transaction(ctx, s.db, func(tx *sql.Tx) error {
		var count int
		if err := tx.QueryRow("SELECT count(*) FROM memory_review_notes WHERE session_key=? AND id BETWEEN ? AND ?",
			batch.SessionKey, first, last).Scan(&count); err != nil {
			return err
		}
		if count != len(batch.Notes) {
			return nil
		}
		if len(changes) != 0 {
			p, r, stateErr := reviewState(tx)
			if stateErr != nil {
				return stateErr
			}
			if !reflect.DeepEqual(p, batch.Preferences) || !reflect.DeepEqual(r, batch.Records) {
				return nil
			}
			for _, c := range changes {
				cutoffKey := recordReviewCutoff
				if c.Kind == "preference" {
					cutoffKey = reviewPreferenceCutoff(c.Key)
				}
				var cutoff int64
				if err := tx.QueryRow("SELECT COALESCE((SELECT CAST(value AS INTEGER) FROM memory_meta WHERE key=?),0)", cutoffKey).Scan(&cutoff); err != nil {
					return err
				}
				if c.Evidence <= cutoff {
					continue
				}
				n := notes[c.Evidence]
				switch c.Kind {
				case "preference":
					updated, e := applyReviewPreference(tx, c, n)
					if e != nil {
						return e
					}
					if updated {
						result.Preferences++
					}
				case "record":
					updated, e := applyReviewRecord(tx, c, n)
					if e != nil {
						return e
					}
					if updated {
						result.Records++
					}
				}
			}
		}
		_, err := tx.Exec(
			"DELETE FROM memory_review_notes WHERE session_key=? AND id BETWEEN ? AND ?",
			batch.SessionKey,
			first,
			last,
		)
		result.Applied = err == nil
		return err
	})
	if err != nil {
		return ReviewResult{}, err
	}
	return result, nil
}

func applyReviewPreference(tx *sql.Tx, c ReviewChange, note ReviewNote) (bool, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return changed(tx.Exec(`INSERT INTO preferences(key,value,source,updated_at,memory_evidence)
SELECT ?,?,?,?,? WHERE EXISTS(SELECT 1 FROM preferences WHERE key=?)
 OR (SELECT count(*) FROM preferences)<?
ON CONFLICT(key) DO UPDATE SET value=excluded.value,source=excluded.source,updated_at=excluded.updated_at,
 memory_evidence=excluded.memory_evidence
WHERE preferences.value<>excluded.value
AND COALESCE(preferences.memory_evidence, CAST(unixepoch(preferences.updated_at,'subsec')*1000 AS INTEGER),0)<=excluded.memory_evidence`,
		c.Key, c.Value, note.Source, now, note.At.UnixMilli(), c.Key, PreferenceLimit))
}

func reviewedRecord(c ReviewChange, note ReviewNote) (Record, error) {
	r := Record{
		Body: strings.TrimSpace(c.Value), Date: c.Date, Source: note.Source,
		Category: c.Category, Aliases: c.Aliases, Tags: c.Tags,
	}
	if err := validateRecord(r); err != nil {
		return Record{}, err
	}
	return enrichRecordLabels(r), nil
}

func applyReviewRecord(tx *sql.Tx, c ReviewChange, note ReviewNote) (bool, error) {
	r, err := reviewedRecord(c, note)
	if err != nil {
		return false, err
	}
	var previousAliases, previousTags []byte
	var previousCategory string
	err = tx.QueryRow("SELECT aliases,tags,coalesce(category,'') FROM records WHERE memory_topic=?", c.Key).
		Scan(&previousAliases, &previousTags, &previousCategory)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if err == nil {
		var aliases, tags []string
		if err = json.Unmarshal(previousAliases, &aliases); err != nil {
			return false, err
		}
		if err = json.Unmarshal(previousTags, &tags); err != nil {
			return false, err
		}
		r.Aliases = addLabels(aliases, r.Aliases...)
		r.Tags = addLabels(tags, r.Tags...)
		if c.Category == "" {
			r.Category = previousCategory
		}
	}
	// Use an empty JSON array consistently so old empty labels don't cause a rewrite.
	aliases, _ := json.Marshal(append([]string{}, r.Aliases...))
	tags, _ := json.Marshal(append([]string{}, r.Tags...))
	now := time.Now().UTC().Format(time.RFC3339Nano)
	// Explicit and imported records are not rewritten as rolling records.
	result, err := tx.Exec(
		`INSERT INTO records(memory_topic,body,event_date,category,aliases,tags,source,created_at,updated_at,memory_evidence)
SELECT ?,?,?,NULLIF(?,''),?,?,?,?,?,? WHERE NOT EXISTS (
 SELECT 1 FROM records WHERE body=? AND event_date=? AND (memory_topic IS NULL OR memory_topic<>?))
ON CONFLICT(memory_topic) WHERE memory_topic IS NOT NULL DO UPDATE
 SET body=excluded.body,event_date=excluded.event_date,source=excluded.source,updated_at=excluded.updated_at,
 category=excluded.category,aliases=excluded.aliases,tags=excluded.tags,memory_evidence=excluded.memory_evidence
 WHERE excluded.event_date>=records.event_date
 AND (records.body<>excluded.body OR records.event_date<>excluded.event_date OR records.category IS NOT excluded.category OR records.aliases<>excluded.aliases OR records.tags<>excluded.tags)
 AND COALESCE(records.memory_evidence, CAST(unixepoch(records.updated_at,'subsec')*1000 AS INTEGER),0)<=excluded.memory_evidence`,
		c.Key,
		r.Body,
		r.Date,
		r.Category,
		string(aliases),
		string(tags),
		r.Source,
		now,
		now,
		note.At.UnixMilli(),
		r.Body,
		r.Date,
		c.Key,
	)
	if err != nil {
		return false, fmt.Errorf("upsert review record: %w", err)
	}
	return changed(result, nil)
}

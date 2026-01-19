package main

import (
	"encoding/json"
	"errors"
	"os"
	"time"
)

const exportStateVersion = 1

var (
	stateMarshalIndent = json.MarshalIndent
	stateWriteFile     = os.WriteFile
	stateReadFile      = os.ReadFile
	stateUnmarshal     = json.Unmarshal
)

type ExportState struct {
	Version    int       `json:"version"`
	ExportedAt time.Time `json:"exported_at"`
	Feeds      []Feed    `json:"feeds"`
	Articles   []Article `json:"articles"`
	Summaries  []Summary `json:"summaries"`
	Saved      []Saved   `json:"saved"`
	Deleted    []Deleted `json:"deleted"`
}

func (s *Store) ExportState(path string) error {
	if path == "" {
		return errors.New("missing export path")
	}
	state := ExportState{
		Version:    exportStateVersion,
		ExportedAt: time.Now().UTC(),
		Feeds:      s.Feeds(),
		Articles:   s.Articles(),
		Summaries:  s.Summaries(),
		Saved:      s.Saved(),
		Deleted:    s.Deleted(),
	}
	payload, err := stateMarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return stateWriteFile(path, payload, 0o600)
}

func (s *Store) ImportState(path string) error {
	if path == "" {
		return errors.New("missing import path")
	}
	raw, err := stateReadFile(path)
	if err != nil {
		return err
	}
	var state ExportState
	if err := stateUnmarshal(raw, &state); err != nil {
		return err
	}
	if state.Version != exportStateVersion {
		return errors.New("unsupported export format")
	}
	tx, err := beginTx(s.db)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM summaries`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM saved`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM deleted`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM articles`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM feeds`); err != nil {
		return err
	}
	for _, feed := range state.Feeds {
		if _, err := tx.Exec(`INSERT INTO feeds (id, title, url, site_url, description, last_fetched, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			feed.ID, feed.Title, feed.URL, feed.SiteURL, feed.Description, timeToUnix(feed.LastFetched), timeToUnix(feed.CreatedAt), timeToUnix(feed.UpdatedAt)); err != nil {
			return err
		}
	}
	for _, article := range state.Articles {
		if _, err := tx.Exec(`INSERT INTO articles (id, feed_id, guid, title, url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			article.ID, article.FeedID, article.GUID, article.Title, article.URL, article.Author, article.Content, article.ContentText, timeToUnix(article.PublishedAt), timeToUnix(article.FetchedAt), boolToInt(article.IsRead), boolToInt(article.IsStarred), article.FeedTitle); err != nil {
			return err
		}
	}
	for _, summary := range state.Summaries {
		if _, err := tx.Exec(`INSERT INTO summaries (id, article_id, content, model, generated_at) VALUES (?, ?, ?, ?, ?)`,
			summary.ID, summary.ArticleID, summary.Content, summary.Model, timeToUnix(summary.GeneratedAt)); err != nil {
			return err
		}
	}
	for _, saved := range state.Saved {
		blob, err := tagsMarshal(saved.Tags)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO saved (article_id, raindrop_id, tags, saved_at) VALUES (?, ?, ?, ?)`,
			saved.ArticleID, saved.RaindropID, string(blob), timeToUnix(saved.SavedAt)); err != nil {
			return err
		}
	}
	for _, deleted := range state.Deleted {
		article := deleted.Article
		if _, err := tx.Exec(`INSERT INTO deleted (feed_id, guid, title, url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title, deleted_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			deleted.FeedID, deleted.GUID, article.Title, article.URL, article.Author, article.Content, article.ContentText, timeToUnix(article.PublishedAt), timeToUnix(article.FetchedAt), boolToInt(article.IsRead), boolToInt(article.IsStarred), article.FeedTitle, timeToUnix(deleted.DeletedAt)); err != nil {
			return err
		}
	}
	if err := commitTx(tx); err != nil {
		return err
	}
	return nil
}

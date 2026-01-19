package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	path string
	db   *sql.DB
}

var (
	openSQLite               = sql.Open
	schemaInit               = initSchema
	beginTx                  = func(db *sql.DB) (*sql.Tx, error) { return db.Begin() }
	commitTx                 = func(tx *sql.Tx) error { return tx.Commit() }
	rowsAffected             = func(result sql.Result) (int64, error) { return result.RowsAffected() }
	lastInsertID             = func(result sql.Result) (int64, error) { return result.LastInsertId() }
	tagsMarshal              = json.Marshal
	tagsUnmarshal            = json.Unmarshal
	ensureColumnFn           = ensureColumn
	findArticleIDByBaseURLFn = findArticleIDByBaseURL
	ensureArticleSourceFn    = ensureArticleSource
	existsByIDFn             = existsByID
)

func NewStore(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("missing db path")
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return nil, errors.New("db path is a directory")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := openSQLite("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := schemaInit(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{path: path, db: db}, nil
}

func initSchema(db *sql.DB) error {
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return err
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS feeds (
			id INTEGER PRIMARY KEY,
			title TEXT,
			url TEXT UNIQUE,
			site_url TEXT,
			description TEXT,
			last_fetched INTEGER,
			created_at INTEGER,
			updated_at INTEGER
		);`,
		`CREATE TABLE IF NOT EXISTS articles (
			id INTEGER PRIMARY KEY,
			feed_id INTEGER,
			guid TEXT,
			title TEXT,
			url TEXT,
			base_url TEXT,
			author TEXT,
			content TEXT,
			content_text TEXT,
			published_at INTEGER,
			fetched_at INTEGER,
			is_read INTEGER,
			is_starred INTEGER,
			feed_title TEXT,
			UNIQUE(feed_id, guid)
		);`,
		`CREATE TABLE IF NOT EXISTS summaries (
			id INTEGER PRIMARY KEY,
			article_id INTEGER UNIQUE,
			content TEXT,
			model TEXT,
			generated_at INTEGER,
			FOREIGN KEY(article_id) REFERENCES articles(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS saved (
			article_id INTEGER PRIMARY KEY,
			raindrop_id INTEGER,
			tags TEXT,
			saved_at INTEGER,
			FOREIGN KEY(article_id) REFERENCES articles(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS deleted (
			id INTEGER PRIMARY KEY,
			feed_id INTEGER,
			guid TEXT,
			title TEXT,
			url TEXT,
			base_url TEXT,
			author TEXT,
			content TEXT,
			content_text TEXT,
			published_at INTEGER,
			fetched_at INTEGER,
			is_read INTEGER,
			is_starred INTEGER,
			feed_title TEXT,
			deleted_at INTEGER
		);`,
		`CREATE TABLE IF NOT EXISTS article_sources (
			article_id INTEGER,
			feed_id INTEGER,
			published_at INTEGER,
			UNIQUE(article_id, feed_id),
			FOREIGN KEY(article_id) REFERENCES articles(id) ON DELETE CASCADE,
			FOREIGN KEY(feed_id) REFERENCES feeds(id) ON DELETE CASCADE
		);`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	if err := ensureColumnFn(db, "articles", "base_url", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumnFn(db, "deleted", "base_url", "TEXT"); err != nil {
		return err
	}
	return nil
}

func ensureColumn(db *sql.DB, table string, column string, columnType string) error {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	_, err = db.Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " " + columnType)
	return err
}

func (s *Store) Save() error {
	if s.db == nil {
		return errors.New("store not initialized")
	}
	return s.db.Ping()
}

func (s *Store) Feeds() []Feed {
	rows, err := s.db.Query(`SELECT id, title, url, site_url, description, last_fetched, created_at, updated_at FROM feeds ORDER BY id`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	feeds := []Feed{}
	for rows.Next() {
		var feed Feed
		var lastFetched, createdAt, updatedAt sql.NullInt64
		if err := rows.Scan(&feed.ID, &feed.Title, &feed.URL, &feed.SiteURL, &feed.Description, &lastFetched, &createdAt, &updatedAt); err != nil {
			return feeds
		}
		feed.LastFetched = timeFromUnix(lastFetched)
		feed.CreatedAt = timeFromUnix(createdAt)
		feed.UpdatedAt = timeFromUnix(updatedAt)
		feeds = append(feeds, feed)
	}
	return feeds
}

func (s *Store) Articles() []Article {
	rows, err := s.db.Query(`SELECT id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title FROM articles ORDER BY id`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	articles := []Article{}
	for rows.Next() {
		article, err := scanArticle(rows)
		if err != nil {
			return articles
		}
		articles = append(articles, article)
	}
	return articles
}

func (s *Store) Summaries() []Summary {
	rows, err := s.db.Query(`SELECT id, article_id, content, model, generated_at FROM summaries ORDER BY id`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	items := []Summary{}
	for rows.Next() {
		var summary Summary
		var generatedAt sql.NullInt64
		if err := rows.Scan(&summary.ID, &summary.ArticleID, &summary.Content, &summary.Model, &generatedAt); err != nil {
			return items
		}
		summary.GeneratedAt = timeFromUnix(generatedAt)
		items = append(items, summary)
	}
	return items
}

func (s *Store) Saved() []Saved {
	rows, err := s.db.Query(`SELECT article_id, raindrop_id, tags, saved_at FROM saved ORDER BY article_id`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	items := []Saved{}
	for rows.Next() {
		var saved Saved
		var tagsRaw string
		var savedAt sql.NullInt64
		if err := rows.Scan(&saved.ArticleID, &saved.RaindropID, &tagsRaw, &savedAt); err != nil {
			return items
		}
		if tagsRaw != "" {
			_ = tagsUnmarshal([]byte(tagsRaw), &saved.Tags)
		}
		saved.SavedAt = timeFromUnix(savedAt)
		items = append(items, saved)
	}
	return items
}

func (s *Store) Deleted() []Deleted {
	rows, err := s.db.Query(`SELECT feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title, deleted_at FROM deleted ORDER BY id`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	items := []Deleted{}
	for rows.Next() {
		var deleted Deleted
		var publishedAt, fetchedAt, deletedAt sql.NullInt64
		var isRead, isStarred int
		article := Article{}
		if err := rows.Scan(&deleted.FeedID, &deleted.GUID, &article.Title, &article.URL, &article.BaseURL, &article.Author, &article.Content, &article.ContentText, &publishedAt, &fetchedAt, &isRead, &isStarred, &article.FeedTitle, &deletedAt); err != nil {
			return items
		}
		article.FeedID = deleted.FeedID
		article.GUID = deleted.GUID
		article.PublishedAt = timeFromUnix(publishedAt)
		article.FetchedAt = timeFromUnix(fetchedAt)
		article.IsRead = intToBool(isRead)
		article.IsStarred = intToBool(isStarred)
		deleted.Article = article
		deleted.DeletedAt = timeFromUnix(deletedAt)
		items = append(items, deleted)
	}
	return items
}

func (s *Store) InsertFeed(feed Feed) (Feed, error) {
	var existingID int
	if err := s.db.QueryRow(`SELECT id FROM feeds WHERE url = ?`, feed.URL).Scan(&existingID); err == nil {
		return Feed{}, errors.New("feed already exists")
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Feed{}, err
	}

	now := time.Now().UTC()
	if feed.CreatedAt.IsZero() {
		feed.CreatedAt = now
	}
	if feed.UpdatedAt.IsZero() {
		feed.UpdatedAt = feed.CreatedAt
	}

	result, err := s.db.Exec(`INSERT INTO feeds (title, url, site_url, description, last_fetched, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		feed.Title, feed.URL, feed.SiteURL, feed.Description, timeToUnix(feed.LastFetched), timeToUnix(feed.CreatedAt), timeToUnix(feed.UpdatedAt))
	if err != nil {
		return Feed{}, err
	}
	id, err := lastInsertID(result)
	if err != nil {
		return Feed{}, err
	}
	feed.ID = int(id)
	return feed, nil
}

func (s *Store) UpdateFeed(feed Feed) error {
	feed.UpdatedAt = time.Now().UTC()
	result, err := s.db.Exec(`UPDATE feeds SET title = ?, url = ?, site_url = ?, description = ?, last_fetched = ?, created_at = ?, updated_at = ? WHERE id = ?`,
		feed.Title, feed.URL, feed.SiteURL, feed.Description, timeToUnix(feed.LastFetched), timeToUnix(feed.CreatedAt), timeToUnix(feed.UpdatedAt), feed.ID)
	if err != nil {
		return err
	}
	rows, err := rowsAffected(result)
	if err != nil {
		return err
	}
	if rows == 0 {
		return errors.New("feed not found")
	}
	return nil
}

func (s *Store) DeleteFeed(id int) error {
	tx, err := beginTx(s.db)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM feeds WHERE id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM articles WHERE feed_id = ?`, id); err != nil {
		return err
	}
	return commitTx(tx)
}

func (s *Store) InsertArticles(feed Feed, incoming []Article) ([]Article, error) {
	tx, err := beginTx(s.db)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	seen := map[string]bool{}
	rows, err := tx.Query(`SELECT guid FROM articles WHERE feed_id = ?`, feed.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var guid string
		if err := rows.Scan(&guid); err != nil {
			rows.Close()
			return nil, err
		}
		seen[guid] = true
	}
	rows.Close()
	rows, err = tx.Query(`SELECT guid FROM deleted WHERE feed_id = ?`, feed.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var guid string
		if err := rows.Scan(&guid); err != nil {
			rows.Close()
			return nil, err
		}
		seen[guid] = true
	}
	rows.Close()

	added := []Article{}
	for _, article := range incoming {
		if article.GUID == "" {
			article.GUID = article.URL
		}
		article.BaseURL = baseURL(article.URL)
		if article.BaseURL == "" {
			article.BaseURL = article.URL
		}
		if seen[article.GUID] {
			continue
		}
		seen[article.GUID] = true
		article.FeedID = feed.ID
		article.FeedTitle = feed.Title
		if article.FetchedAt.IsZero() {
			article.FetchedAt = time.Now().UTC()
		}
		existingID, err := findArticleIDByBaseURLFn(tx, article.BaseURL)
		if err != nil {
			return nil, err
		}
		if existingID != 0 {
			if err := ensureArticleSourceFn(tx, existingID, feed.ID, article.PublishedAt); err != nil {
				return nil, err
			}
			continue
		}
		result, err := tx.Exec(`INSERT INTO articles (feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			article.FeedID, article.GUID, article.Title, article.URL, article.BaseURL, article.Author, article.Content, article.ContentText, timeToUnix(article.PublishedAt), timeToUnix(article.FetchedAt), boolToInt(article.IsRead), boolToInt(article.IsStarred), article.FeedTitle)
		if err != nil {
			return nil, err
		}
		id, err := lastInsertID(result)
		if err != nil {
			return nil, err
		}
		article.ID = int(id)
		if err := ensureArticleSourceFn(tx, article.ID, feed.ID, article.PublishedAt); err != nil {
			return nil, err
		}
		added = append(added, article)
	}

	feed.LastFetched = time.Now().UTC()
	feed.UpdatedAt = time.Now().UTC()
	if _, err := tx.Exec(`UPDATE feeds SET last_fetched = ?, updated_at = ? WHERE id = ?`, timeToUnix(feed.LastFetched), timeToUnix(feed.UpdatedAt), feed.ID); err != nil {
		return nil, err
	}

	if err := commitTx(tx); err != nil {
		return nil, err
	}
	return added, nil
}

func findArticleIDByBaseURL(tx *sql.Tx, base string) (int, error) {
	if strings.TrimSpace(base) == "" {
		return 0, nil
	}
	var id int
	err := tx.QueryRow(`SELECT id FROM articles WHERE base_url = ? LIMIT 1`, base).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return id, nil
}

func ensureArticleSource(tx *sql.Tx, articleID int, feedID int, publishedAt time.Time) error {
	var existing int
	if err := tx.QueryRow(`SELECT 1 FROM article_sources WHERE article_id = ? AND feed_id = ?`, articleID, feedID).Scan(&existing); err == nil {
		if publishedAt.IsZero() {
			return nil
		}
		_, err := tx.Exec(`UPDATE article_sources SET published_at = ? WHERE article_id = ? AND feed_id = ? AND (published_at IS NULL OR published_at = 0)`,
			timeToUnix(publishedAt), articleID, feedID)
		return err
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err := tx.Exec(`INSERT INTO article_sources (article_id, feed_id, published_at) VALUES (?, ?, ?)`,
		articleID, feedID, timeToUnix(publishedAt))
	return err
}

func (s *Store) FindSummary(articleID int) (Summary, bool) {
	var summary Summary
	var generatedAt sql.NullInt64
	if err := s.db.QueryRow(`SELECT id, article_id, content, model, generated_at FROM summaries WHERE article_id = ?`, articleID).Scan(&summary.ID, &summary.ArticleID, &summary.Content, &summary.Model, &generatedAt); err != nil {
		return Summary{}, false
	}
	summary.GeneratedAt = timeFromUnix(generatedAt)
	return summary, true
}

func (s *Store) UpsertSummary(summary Summary) (Summary, error) {
	var existingID int
	if err := s.db.QueryRow(`SELECT id FROM summaries WHERE article_id = ?`, summary.ArticleID).Scan(&existingID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Summary{}, err
	}
	if summary.GeneratedAt.IsZero() {
		summary.GeneratedAt = time.Now().UTC()
	}
	if existingID != 0 {
		summary.ID = existingID
		_, err := s.db.Exec(`UPDATE summaries SET content = ?, model = ?, generated_at = ? WHERE article_id = ?`, summary.Content, summary.Model, timeToUnix(summary.GeneratedAt), summary.ArticleID)
		if err != nil {
			return Summary{}, err
		}
		return summary, nil
	}
	result, err := s.db.Exec(`INSERT INTO summaries (article_id, content, model, generated_at) VALUES (?, ?, ?, ?)`, summary.ArticleID, summary.Content, summary.Model, timeToUnix(summary.GeneratedAt))
	if err != nil {
		return Summary{}, err
	}
	id, err := lastInsertID(result)
	if err != nil {
		return Summary{}, err
	}
	summary.ID = int(id)
	return summary, nil
}

func (s *Store) UpdateArticle(article Article) error {
	if article.BaseURL == "" {
		article.BaseURL = baseURL(article.URL)
	}
	result, err := s.db.Exec(`UPDATE articles SET feed_id = ?, guid = ?, title = ?, url = ?, base_url = ?, author = ?, content = ?, content_text = ?, published_at = ?, fetched_at = ?, is_read = ?, is_starred = ?, feed_title = ? WHERE id = ?`,
		article.FeedID, article.GUID, article.Title, article.URL, article.BaseURL, article.Author, article.Content, article.ContentText, timeToUnix(article.PublishedAt), timeToUnix(article.FetchedAt), boolToInt(article.IsRead), boolToInt(article.IsStarred), article.FeedTitle, article.ID)
	if err != nil {
		return err
	}
	rows, err := rowsAffected(result)
	if err != nil {
		return err
	}
	if rows == 0 {
		return errors.New("article not found")
	}
	return nil
}

func (s *Store) DeleteArticle(id int) (Article, error) {
	row := s.db.QueryRow(`SELECT id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title FROM articles WHERE id = ?`, id)
	article, err := scanArticle(row)
	if err != nil {
		return Article{}, errors.New("article not found")
	}
	tx, err := beginTx(s.db)
	if err != nil {
		return Article{}, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM articles WHERE id = ?`, id); err != nil {
		return Article{}, err
	}
	if _, err := tx.Exec(`DELETE FROM summaries WHERE article_id = ?`, id); err != nil {
		return Article{}, err
	}
	if _, err := tx.Exec(`DELETE FROM saved WHERE article_id = ?`, id); err != nil {
		return Article{}, err
	}
	if _, err := tx.Exec(`INSERT INTO deleted (feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title, deleted_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		article.FeedID, article.GUID, article.Title, article.URL, article.BaseURL, article.Author, article.Content, article.ContentText, timeToUnix(article.PublishedAt), timeToUnix(article.FetchedAt), boolToInt(article.IsRead), boolToInt(article.IsStarred), article.FeedTitle, timeToUnix(time.Now().UTC())); err != nil {
		return Article{}, err
	}
	if err := commitTx(tx); err != nil {
		return Article{}, err
	}
	return article, nil
}

func (s *Store) UndeleteLast() (Article, error) {
	row := s.db.QueryRow(`SELECT id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title FROM deleted ORDER BY id DESC LIMIT 1`)
	var deletedID int
	article, err := scanDeleted(row, &deletedID)
	if err != nil {
		return Article{}, errors.New("no deleted article")
	}
	if article.BaseURL == "" {
		article.BaseURL = baseURL(article.URL)
	}
	result, err := s.db.Exec(`INSERT INTO articles (feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		article.FeedID, article.GUID, article.Title, article.URL, article.BaseURL, article.Author, article.Content, article.ContentText, timeToUnix(article.PublishedAt), timeToUnix(article.FetchedAt), boolToInt(article.IsRead), boolToInt(article.IsStarred), article.FeedTitle)
	if err != nil {
		return Article{}, err
	}
	id, err := lastInsertID(result)
	if err != nil {
		return Article{}, err
	}
	article.ID = int(id)
	if _, err := s.db.Exec(`DELETE FROM deleted WHERE id = ?`, deletedID); err != nil {
		return Article{}, err
	}
	return article, nil
}

func (s *Store) DeleteOldArticles(days int) int {
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM articles WHERE fetched_at < ?`, timeToUnix(cutoff)).Scan(&count); err != nil {
		return 0
	}
	if _, err := s.db.Exec(`DELETE FROM articles WHERE fetched_at < ?`, timeToUnix(cutoff)); err != nil {
		return 0
	}
	s.CleanupOrphanSummaries()
	return count
}

func (s *Store) CleanupOrphanSummaries() {
	_, _ = s.db.Exec(`DELETE FROM summaries WHERE article_id NOT IN (SELECT id FROM articles)`)
	_, _ = s.db.Exec(`DELETE FROM saved WHERE article_id NOT IN (SELECT id FROM articles)`)
}

func (s *Store) Compact(days int) int {
	return s.DeleteOldArticles(days)
}

func (s *Store) SaveToRaindrop(articleID int, raindropID int, tags []string) error {
	blob, err := tagsMarshal(tags)
	if err != nil {
		return err
	}
	result, err := s.db.Exec(`UPDATE saved SET raindrop_id = ?, tags = ?, saved_at = ? WHERE article_id = ?`, raindropID, string(blob), timeToUnix(time.Now().UTC()), articleID)
	if err != nil {
		return err
	}
	rows, err := rowsAffected(result)
	if err != nil {
		return err
	}
	if rows == 0 {
		_, err := s.db.Exec(`INSERT INTO saved (article_id, raindrop_id, tags, saved_at) VALUES (?, ?, ?, ?)`, articleID, raindropID, string(blob), timeToUnix(time.Now().UTC()))
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SavedCount() int {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM saved`).Scan(&count); err != nil {
		return 0
	}
	return count
}

func (s *Store) ArticleSources(articleID int) []ArticleSource {
	rows, err := s.db.Query(`SELECT COALESCE(feeds.title, ''), article_sources.published_at FROM article_sources LEFT JOIN feeds ON feeds.id = article_sources.feed_id WHERE article_sources.article_id = ? ORDER BY feeds.title`, articleID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	items := []ArticleSource{}
	for rows.Next() {
		var source ArticleSource
		var publishedAt sql.NullInt64
		if err := rows.Scan(&source.FeedTitle, &publishedAt); err != nil {
			return items
		}
		source.PublishedAt = timeFromUnix(publishedAt)
		items = append(items, source)
	}
	return items
}

func (s *Store) SortedArticles() []Article {
	rows, err := s.db.Query(`SELECT id, feed_id, guid, title, url, base_url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title FROM articles ORDER BY published_at DESC`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	articles := []Article{}
	for rows.Next() {
		article, err := scanArticle(rows)
		if err != nil {
			return articles
		}
		articles = append(articles, article)
	}
	return articles
}

func scanArticle(scanner interface{ Scan(dest ...any) error }) (Article, error) {
	var article Article
	var publishedAt, fetchedAt sql.NullInt64
	var isRead, isStarred int
	if err := scanner.Scan(&article.ID, &article.FeedID, &article.GUID, &article.Title, &article.URL, &article.BaseURL, &article.Author, &article.Content, &article.ContentText, &publishedAt, &fetchedAt, &isRead, &isStarred, &article.FeedTitle); err != nil {
		return Article{}, err
	}
	article.PublishedAt = timeFromUnix(publishedAt)
	article.FetchedAt = timeFromUnix(fetchedAt)
	article.IsRead = isRead != 0
	article.IsStarred = isStarred != 0
	return article, nil
}

func scanDeleted(scanner interface{ Scan(dest ...any) error }, deletedID *int) (Article, error) {
	var article Article
	var publishedAt, fetchedAt sql.NullInt64
	var isRead, isStarred int
	if err := scanner.Scan(deletedID, &article.FeedID, &article.GUID, &article.Title, &article.URL, &article.BaseURL, &article.Author, &article.Content, &article.ContentText, &publishedAt, &fetchedAt, &isRead, &isStarred, &article.FeedTitle); err != nil {
		return Article{}, err
	}
	article.PublishedAt = timeFromUnix(publishedAt)
	article.FetchedAt = timeFromUnix(fetchedAt)
	article.IsRead = isRead != 0
	article.IsStarred = isStarred != 0
	return article, nil
}

func timeToUnix(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.Unix()
}

func timeFromUnix(value sql.NullInt64) time.Time {
	if !value.Valid || value.Int64 == 0 {
		return time.Time{}
	}
	return time.Unix(value.Int64, 0).UTC()
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func intToBool(value int) bool {
	return value != 0
}

func (s *Store) MergeDuplicateArticles() error {
	tx, err := beginTx(s.db)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`SELECT id, feed_id, url, base_url, published_at FROM articles ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()

	baseToID := map[string]int{}
	for rows.Next() {
		var id, feedID int
		var urlValue, baseValue string
		var publishedAt sql.NullInt64
		if err := rows.Scan(&id, &feedID, &urlValue, &baseValue, &publishedAt); err != nil {
			return err
		}
		normalized := baseURL(urlValue)
		if normalized == "" {
			normalized = strings.TrimSpace(baseValue)
		}
		if normalized == "" {
			normalized = urlValue
		}
		if normalized != baseValue {
			if _, err := tx.Exec(`UPDATE articles SET base_url = ? WHERE id = ?`, normalized, id); err != nil {
				return err
			}
		}
		baseValue = normalized
		if existingID, ok := baseToID[baseValue]; ok {
			if err := ensureArticleSourceFn(tx, existingID, feedID, timeFromUnix(publishedAt)); err != nil {
				return err
			}
			hasSummary, err := existsByIDFn(tx, "summaries", existingID)
			if err != nil {
				return err
			}
			if hasSummary {
				if _, err := tx.Exec(`DELETE FROM summaries WHERE article_id = ?`, id); err != nil {
					return err
				}
			} else {
				if _, err := tx.Exec(`UPDATE summaries SET article_id = ? WHERE article_id = ?`, existingID, id); err != nil {
					return err
				}
			}
			hasSaved, err := existsByIDFn(tx, "saved", existingID)
			if err != nil {
				return err
			}
			if hasSaved {
				if _, err := tx.Exec(`DELETE FROM saved WHERE article_id = ?`, id); err != nil {
					return err
				}
			} else {
				if _, err := tx.Exec(`UPDATE saved SET article_id = ? WHERE article_id = ?`, existingID, id); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(`DELETE FROM articles WHERE id = ?`, id); err != nil {
				return err
			}
			continue
		}
		baseToID[baseValue] = id
		if err := ensureArticleSourceFn(tx, id, feedID, timeFromUnix(publishedAt)); err != nil {
			return err
		}
	}
	return commitTx(tx)
}

func existsByID(tx *sql.Tx, table string, articleID int) (bool, error) {
	var existing int
	if err := tx.QueryRow("SELECT 1 FROM "+table+" WHERE article_id = ? LIMIT 1", articleID).Scan(&existing); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func baseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

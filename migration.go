package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type legacyStoreData struct {
	Feeds     []Feed    `json:"feeds"`
	Articles  []Article `json:"articles"`
	Summaries []Summary `json:"summaries"`
	Saved     []Saved   `json:"saved"`
	Deleted   []Deleted `json:"deleted"`
}

var terminalCheck = func(stdin io.Reader, stdout io.Writer) bool {
	return isTerminalReader(stdin) && isTerminalWriter(stdout)
}

var userConfigDir = os.UserConfigDir
var userHomeDir = os.UserHomeDir
var legacyJSONMarshal = json.Marshal

func legacyConfigPath() string {
	configDir, err := userConfigDir()
	if err != nil {
		return "config.toml"
	}
	return filepath.Join(configDir, "speedy-reader", "config.toml")
}

func legacyDefaultDBPath() string {
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		home, err := userHomeDir()
		if err != nil {
			return "feeds.db"
		}
		dataDir = filepath.Join(home, ".local", "share")
	}
	path := filepath.Join(dataDir, "speedy-reader")
	return filepath.Join(path, "feeds.db")
}

func maybeOfferMigration(stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	newConfig := configPath()
	legacyConfig := legacyConfigPath()
	if fileExists(newConfig) {
		return nil
	}
	if !fileExists(legacyConfig) {
		return nil
	}
	if !terminalCheck(stdin, stdout) {
		fmt.Fprintf(stderr, "Legacy config found at %s. Run greeder interactively to migrate.\n", legacyConfig)
		return nil
	}
	fmt.Fprint(stdout, "Migrate config and database from speedy-reader to greeder? [y/N]: ")
	reader := bufio.NewReader(stdin)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))
	if response != "y" && response != "yes" {
		return nil
	}
	if err := migrateLegacyConfigAndDB(legacyConfig, newConfig); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "Migration complete.")
	return nil
}

func migrateLegacyConfigAndDB(legacyConfigPath string, newConfigPath string) error {
	data, err := os.ReadFile(legacyConfigPath)
	if err != nil {
		return err
	}
	legacyCfg := DefaultConfig()
	legacyCfg.DBPath = legacyDefaultDBPath()
	if err := parseConfig(string(data), &legacyCfg); err != nil {
		return err
	}

	newCfg := legacyCfg
	newCfg.DBPath = defaultDBPath()
	if err := os.MkdirAll(filepath.Dir(newConfigPath), 0o755); err != nil {
		return err
	}
	content := renderConfig(newCfg)
	if err := os.WriteFile(newConfigPath, []byte(content), 0o600); err != nil {
		return err
	}

	if err := migrateLegacyDB(legacyCfg.DBPath, newCfg.DBPath); err != nil {
		return err
	}
	return nil
}

func migrateLegacyDB(oldPath string, newPath string) error {
	if !fileExists(oldPath) {
		return errors.New("legacy database not found")
	}
	data, err := os.ReadFile(oldPath)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		_, err := NewStore(newPath)
		return err
	}
	var legacy legacyStoreData
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}
	store, err := NewStore(newPath)
	if err != nil {
		return err
	}
	defer store.db.Close()

	tx, err := beginTx(store.db)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, feed := range legacy.Feeds {
		if _, err := tx.Exec(`INSERT INTO feeds (id, title, url, site_url, description, last_fetched, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			feed.ID, feed.Title, feed.URL, feed.SiteURL, feed.Description, timeToUnix(feed.LastFetched), timeToUnix(feed.CreatedAt), timeToUnix(feed.UpdatedAt)); err != nil {
			return err
		}
	}
	for _, article := range legacy.Articles {
		if _, err := tx.Exec(`INSERT INTO articles (id, feed_id, guid, title, url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			article.ID, article.FeedID, article.GUID, article.Title, article.URL, article.Author, article.Content, article.ContentText, timeToUnix(article.PublishedAt), timeToUnix(article.FetchedAt), boolToInt(article.IsRead), boolToInt(article.IsStarred), article.FeedTitle); err != nil {
			return err
		}
	}
	for _, summary := range legacy.Summaries {
		if _, err := tx.Exec(`INSERT INTO summaries (id, article_id, content, model, generated_at) VALUES (?, ?, ?, ?, ?)`,
			summary.ID, summary.ArticleID, summary.Content, summary.Model, timeToUnix(summary.GeneratedAt)); err != nil {
			return err
		}
	}
	for _, saved := range legacy.Saved {
		blob, err := legacyJSONMarshal(saved.Tags)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO saved (article_id, raindrop_id, tags, saved_at) VALUES (?, ?, ?, ?)`,
			saved.ArticleID, saved.RaindropID, string(blob), timeToUnix(saved.SavedAt)); err != nil {
			return err
		}
	}
	for _, deleted := range legacy.Deleted {
		article := deleted.Article
		if _, err := tx.Exec(`INSERT INTO deleted (feed_id, guid, title, url, author, content, content_text, published_at, fetched_at, is_read, is_starred, feed_title, deleted_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			deleted.FeedID, deleted.GUID, article.Title, article.URL, article.Author, article.Content, article.ContentText, timeToUnix(article.PublishedAt), timeToUnix(article.FetchedAt), boolToInt(article.IsRead), boolToInt(article.IsStarred), article.FeedTitle, timeToUnix(deleted.DeletedAt)); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

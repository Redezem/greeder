package main

import "time"

type Feed struct {
	ID          int       `json:"id"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	SiteURL     string    `json:"site_url"`
	Description string    `json:"description"`
	LastFetched time.Time `json:"last_fetched"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Article struct {
	ID          int       `json:"id"`
	FeedID      int       `json:"feed_id"`
	GUID        string    `json:"guid"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	BaseURL     string    `json:"base_url"`
	Author      string    `json:"author"`
	Content     string    `json:"content"`
	ContentText string    `json:"content_text"`
	PublishedAt time.Time `json:"published_at"`
	FetchedAt   time.Time `json:"fetched_at"`
	IsRead      bool      `json:"is_read"`
	IsStarred   bool      `json:"is_starred"`
	FeedTitle   string    `json:"feed_title"`
}

type Summary struct {
	ID          int       `json:"id"`
	ArticleID   int       `json:"article_id"`
	Content     string    `json:"content"`
	Model       string    `json:"model"`
	GeneratedAt time.Time `json:"generated_at"`
}

type ArticleSource struct {
	FeedTitle   string
	PublishedAt time.Time
}

type Saved struct {
	ArticleID  int       `json:"article_id"`
	RaindropID int       `json:"raindrop_id"`
	Tags       []string  `json:"tags"`
	SavedAt    time.Time `json:"saved_at"`
}

type Deleted struct {
	FeedID    int       `json:"feed_id"`
	GUID      string    `json:"guid"`
	DeletedAt time.Time `json:"deleted_at"`
	Article   Article   `json:"article"`
}

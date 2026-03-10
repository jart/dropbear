package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// CME publishes all clearing advisories (margin hikes, rule changes, etc.) here.
// This RSS feed is the canonical first signal for formal advisory notices like 26-095.
const clearingAdvisoryRSS = "http://feeds.feedburner.com/ClearingAdvisories"

// Keywords that flag an advisory as margin-related. Case-insensitive.
var marginKeywords = []string{
	"margin", "performance bond", "performance-bond",
	"scan range", "initial margin", "maintenance margin",
}

// Keywords that flag an advisory as energy-related.
// We alert on ALL margin advisories plus anything touching these products.
var productKeywords = []string{
	"crude", "energy", "WTI", " CL ", "nymex", "natural gas",
	"gold", "silver", "platinum",
}

type rssItem struct {
	GUID        string `xml:"guid"`
	Title       string `xml:"title"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	Link        string `xml:"link"`
}

type rssFeed struct {
	Items []rssItem `xml:"channel>item"`
}

func watchAdvisories(cfg Config, state *State, push *Pushover) {
	log.Println("advisory: starting RSS watcher")
	ticker := time.NewTicker(cfg.AdvisoryInterval)
	defer ticker.Stop()

	checkAdvisories(state, push)
	for range ticker.C {
		checkAdvisories(state, push)
	}
}

func checkAdvisories(state *State, push *Pushover) {
	items, err := fetchAdvisoryRSS()
	if err != nil {
		log.Printf("advisory: RSS fetch error: %v", err)
		return
	}
	log.Printf("advisory: fetched %d items", len(items))

	for _, item := range items {
		if state.hasSeenAdvisory(item.GUID) {
			continue
		}
		state.markAdvisorySeen(item.GUID)
		state.save()

		if !isAlertworthy(item) {
			continue
		}

		log.Printf("advisory: NEW MARGIN ADVISORY: %s", item.Title)
		err := push.Sendf(PriorityHigh,
			"🚨 CME Clearing Advisory",
			"%s\n\n%s\n\n%s",
			item.Title, item.PubDate, item.Link,
		)
		if err != nil {
			log.Printf("advisory: pushover error: %v", err)
		}
	}
}

func fetchAdvisoryRSS() ([]rssItem, error) {
	resp, err := http.Get(clearingAdvisoryRSS)
	if err != nil {
		return nil, fmt.Errorf("GET: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("parse XML: %w", err)
	}
	return feed.Items, nil
}

// isAlertworthy returns true if this advisory is about margins or our watched products.
func isAlertworthy(item rssItem) bool {
	text := strings.ToLower(item.Title + " " + item.Description)
	for _, kw := range marginKeywords {
		if strings.Contains(text, strings.ToLower(kw)) {
			return true
		}
	}
	for _, kw := range productKeywords {
		if strings.Contains(text, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

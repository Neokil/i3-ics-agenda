package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"
	"time"

	"github.com/apognu/gocal"
)

const CACHE_FILE = "/tmp/i3-ics-cache"

func main() {
	icsURL := flag.String("ics-url", "", "The ICS-URL that will be used to retreive the events")
	output := flag.String("output", "", "Defines what the output will be. Valid values are 'current' (the current event), 'next' (the next event) and 'agenda' (list of all events for today)")
	calCacheDuration := flag.Duration("cal-cache-duration", time.Minute*5, "Defines how long the ICS will be cached. Default is 5 Minutes")
	flag.Parse()

	if icsURL == nil || *icsURL == "" {
		panic("ics-url-parameter is required")
	}

	if output == nil || *output == "" {
		panic("output-parameter is required")
	}

	e, err := getTodaysEvents(*icsURL, *calCacheDuration)
	if err != nil {
		panic(err)
	}

	switch *output {
	case "current":
		for _, e := range e {
			if e.Start.Before(time.Now()) && e.End.After(time.Now()) {
				renderEvent(e, "default")
				return
			}
		}
		return
	case "next":
		for _, e := range e {
			if e.Start.After(time.Now()) {
				renderEvent(e, "default")
				return
			}
		}
		return
	case "agenda":
		var currentEvent *gocal.Event
		var nextEvent *gocal.Event
		for _, e := range e {
			if currentEvent == nil && e.Start.Before(time.Now()) && e.End.After(time.Now()) {
				currentEvent = &e
				renderEvent(e, "current")
			} else if nextEvent == nil && e.Start.After(time.Now()) {
				nextEvent = &e
				renderEvent(e, "default")
			} else {
				renderEvent(e, "default")
			}
		}
	}
}

func loadEventsFromCache(cacheDuration time.Duration) (result []gocal.Event, err error) {
	s, err := os.Stat(CACHE_FILE)
	if err != nil {
		// cannot access file
		return nil, fmt.Errorf("cannot stat file: %w", err)
	}
	if s.ModTime().Before(time.Now().Add(-cacheDuration)) {
		// file is too old
		return nil, fmt.Errorf("cache-file is too old")
	}

	f, err := os.Open(CACHE_FILE)
	if err != nil {
		// cannot read file
		return nil, fmt.Errorf("cannot read file: %w", err)
	}
	defer f.Close()

	err = json.NewDecoder(f).Decode(&result)
	if err != nil {
		// cannot decode file
		return nil, fmt.Errorf("cannot decode file-content: %w", err)
	}
	return result, nil
}

func loadEventsFromUrl(url string) ([]gocal.Event, error) {
	r, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	y, m, d := time.Now().Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, time.Now().Location())
	end := start.AddDate(0, 0, 1)

	gocal.SetTZMapper(func(s string) (*time.Location, error) {
		return time.Now().Location(), nil
	})
	c := gocal.NewParser(r.Body)
	c.Start = &start
	c.End = &end
	err = c.Parse()
	if err != nil {
		return nil, err
	}

	sort.Slice(c.Events, func(i int, j int) bool {
		return c.Events[i].Start.Before(*c.Events[j].Start)
	})

	return c.Events, nil
}

func cacheEvents(e []gocal.Event) error {
	f, err := os.OpenFile(CACHE_FILE, os.O_CREATE, 0666)
	if err != nil {
		return fmt.Errorf("cannot open or create file: %w", err)
	}
	defer f.Close()

	err = json.NewEncoder(f).Encode(e)
	if err != nil {
		return fmt.Errorf("cannot encode events: %w", err)
	}
	return nil
}

func getTodaysEvents(url string, cacheDuration time.Duration) ([]gocal.Event, error) {
	e, err := loadEventsFromCache(cacheDuration)
	if err == nil {
		return e, nil
	}
	e, err = loadEventsFromUrl(url)
	if err != nil {
		return nil, fmt.Errorf("could not retrieve events from url: %w", err)
	}
	// ignore if caching is not working
	_ = cacheEvents(e)
	return e, nil
}

func renderEvent(e gocal.Event, icon string) {
	fmt.Printf("[%s - %s] %s%s\n", getTimeString(e.Start), getTimeString(e.End), getIcon(icon), e.Summary)
}

func getIcon(icon string) string {
	switch icon {
	case "current":
		return "> "
	default:
		return ""
	}
}

func getTimeString(t *time.Time) string {
	h, m, _ := t.Clock()
	return fmt.Sprintf("%02d:%02d", h, m)
}

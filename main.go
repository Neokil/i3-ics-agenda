package main

import (
	"context"
	"crypto/sha1"
	"embed"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"time"

	"github.com/apognu/gocal"
	"github.com/faiface/beep/speaker"
	"github.com/faiface/beep/wav"
	"github.com/godbus/dbus/v5"
)

const CACHE_FILE = "/tmp/i3-ics-cache"

//go:embed beep.wav
var embedded embed.FS

func main() {
	icsURL := flag.String("ics-url", "", "The ICS-URL that will be used to retreive the events")
	output := flag.String("output", "", "Defines what the output will be. Valid values are 'current' (the current event), 'current-link' (first link in current event location and description), 'next' (the next event), 'next-link' (first link in next event location and description), 'tail' (current and next event in stdout) and 'agenda' (list of all events for today)")
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

	f, err := embedded.Open("beep.wav")
	if err != nil {
		panic(err)
	}

	streamer, format, err := wav.Decode(f)
	if err != nil {
		panic(err)
	}
	defer streamer.Close()
	err = speaker.Init(format.SampleRate, format.SampleRate.N(time.Second/10))
	if err != nil {
		panic(err)
	}

	switch *output {
	case "current":
		evt := getCurrentEvent(e)
		if evt != nil {
			renderEventAsString(*evt)
		}
		return
	case "current-link":
		evt := getCurrentEvent(e)
		if evt != nil {
			renderEventLinkAsString(*evt)
		}
		return
	case "next":
		evt := getNextEvent(e)
		if evt != nil {
			renderEventAsString(*evt)
		}
		return
	case "next-link":
		evt := getNextEvent(e)
		if evt != nil {
			renderEventLinkAsString(*evt)
		}
		return
	case "tail":
		// tail will loop forever
		var currentEvent *gocal.Event
		var nextEvent *gocal.Event
		var nextEventAnnounced1 = false
		var nextEventAnnounced2 = false

		for {
			time.Sleep(time.Second)

			ce := getCurrentEvent(e)
			ne := getNextEvent(e)

			if eventsEqual(ce, currentEvent) && eventsEqual(ne, nextEvent) {
				if ne != nil {
					if !nextEventAnnounced2 && ne.Start.Before(time.Now().Add(5*time.Minute)) {
						nextEventAnnounced1 = true
						nextEventAnnounced2 = true
						speaker.Play(streamer)
						_ = sendNotification("Agenda", "Upcoming event in 5 Minutes")
					}

					if !nextEventAnnounced1 && ne.Start.Before(time.Now().Add(15*time.Minute)) {
						nextEventAnnounced1 = true
						speaker.Play(streamer)
						_ = sendNotification("Agenda", "Upcoming event in 15 Minutes")
					}
				}

				continue
			}
			speaker.Play(streamer)
			nextEventAnnounced1 = false
			nextEventAnnounced2 = false
			currentEvent = ce
			nextEvent = ne

			if currentEvent == nil && nextEvent == nil {
				fmt.Println("No upcoming Events")
				continue
			}
			if currentEvent != nil && nextEvent == nil {
				fmt.Printf("Current: [%s - %s] %s\n", getTimeString(currentEvent.Start), getTimeString(currentEvent.End), fixedSizeString(currentEvent.Summary, 40, "..."))
				continue
			}
			if currentEvent == nil && nextEvent != nil {
				fmt.Printf("Upcoming: [%s - %s] %.40s\n", getTimeString(nextEvent.Start), getTimeString(nextEvent.End), fixedSizeString(nextEvent.Summary, 40, "..."))
				continue
			}

			fmt.Printf("[%s - %s] %s > [%s - %s] %s\n", getTimeString(currentEvent.Start), getTimeString(currentEvent.End), fixedSizeString(currentEvent.Summary, 30, "..."), getTimeString(nextEvent.Start), getTimeString(nextEvent.End), fixedSizeString(nextEvent.Summary, 30, "..."))
		}
	case "agenda":
		var currentEvent *gocal.Event
		for _, e := range e {
			if currentEvent == nil && e.Start.Before(time.Now()) && e.End.After(time.Now()) {
				currentEvent = &e
				renderEventAsListEntry(e, true)
			} else {
				renderEventAsListEntry(e, false)
			}
		}
	}
}

func sendNotification(title string, body string) error {
	con, err := dbus.SessionBus()
	if err != nil {
		return fmt.Errorf("Failed to open dbus-session: %w", err)
	}
	var d = make(chan *dbus.Call, 1)
	var o = con.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications")
	var id uint32
	o.GoWithContext(context.Background(), "org.freedesktop.Notifications.Notify", 0, d, "i3-ics-Agenda", uint32(0), "", title, body, []string{}, map[string]interface{}{}, int32(0))
	err = (<-d).Store(&id)
	if err != nil {
		return fmt.Errorf("Failed to create notification on dbus: %w", err)
	}
	return nil
}

func eventsEqual(e1 *gocal.Event, e2 *gocal.Event) bool {
	if e1 == nil && e2 == nil {
		return true
	}
	if e1 != nil && e2 != nil {
		return e1.Uid == e2.Uid
	}
	return false
}

func getCurrentEvent(events []gocal.Event) *gocal.Event {
	for _, e := range events {
		if e.Start.Before(time.Now()) && e.End.After(time.Now()) {
			return &e
		}
	}
	return nil
}

func getNextEvent(events []gocal.Event) *gocal.Event {
	for _, e := range events {
		if e.Start.After(time.Now()) {
			return &e
		}
	}
	return nil
}

func loadEventsFromCache(url string, cacheDuration time.Duration) (result []gocal.Event, err error) {
	filename := generateCacheFilename(url)
	s, err := os.Stat(filename)
	if err != nil {
		// cannot access file
		return nil, fmt.Errorf("cannot stat file: %w", err)
	}
	if s.ModTime().Before(time.Now().Add(-cacheDuration)) {
		// file is too old
		return nil, fmt.Errorf("cache-file is too old")
	}

	f, err := os.Open(filename)
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

func cacheEvents(e []gocal.Event, url string) error {
	filename := generateCacheFilename(url)
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, 0666)
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

func generateCacheFilename(url string) string {
	urlHash := sha1.Sum([]byte(url))
	return CACHE_FILE + base64.StdEncoding.EncodeToString(urlHash[:])
}

func getTodaysEvents(url string, cacheDuration time.Duration) ([]gocal.Event, error) {
	e, err := loadEventsFromCache(url, cacheDuration)
	if err == nil {
		return e, nil
	}
	e, err = loadEventsFromUrl(url)
	if err != nil {
		return nil, fmt.Errorf("could not retrieve events from url: %w", err)
	}
	err = cacheEvents(e, url)
	if err != nil {
		return nil, fmt.Errorf("cannot cache events: %w", err)
	}
	return e, nil
}

func renderEventAsString(e gocal.Event) {
	fmt.Printf("[%s - %s] %s\n", getTimeString(e.Start), getTimeString(e.End), e.Summary)
}

func renderEventLinkAsString(e gocal.Event) {
	search := e.Location + "\n" + e.Description
	r, err := regexp.Compile(`https?:\/\/(www\.)?[-a-zA-Z0-9@:%._\+~#=]{1,256}\.[a-zA-Z0-9()]{1,6}\b([-a-zA-Z0-9()@:%_\+.~#?&//=]*)`)
	if err != nil {
		panic(err.Error())
	}
	s := r.FindString(search)
	fmt.Print(s)
}

func renderEventAsListEntry(e gocal.Event, current bool) {
	currentVal := "FALSE"
	if current {
		currentVal = "TRUE"
	}
	fmt.Printf("%s '%s' '%s' '%s' ", currentVal, getTimeString(e.Start), getTimeString(e.End), e.Summary)
}

func getTimeString(t *time.Time) string {
	h, m, _ := t.Clock()
	return fmt.Sprintf("%02d:%02d", h, m)
}

func fixedSizeString(s string, maxLength int, ellipsesSymbol string) string {
	if len(s) > maxLength {
		return s[0:maxLength] + ellipsesSymbol
	}
	return s
}

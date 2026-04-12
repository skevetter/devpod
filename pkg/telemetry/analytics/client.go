package analytics

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/skevetter/log"
)

const (
	defaultEndpoint = "https://analytics.loft.rocks/v1/insert"

	eventsCountThreshold = 100

	maxUploadInterval = 5 * time.Minute
	minUploadInterval = 30 * time.Second
)

var Dry = false

func NewClient() Client {
	c := &client{
		endpoint: defaultEndpoint,

		buffer:   newEventBuffer(eventsCountThreshold),
		overflow: newEventBuffer(eventsCountThreshold),

		events:     make(chan Event, 100),
		httpClient: http.Client{Timeout: 3 * time.Second},
		log:        log.Default.WithPrefix("analytics"),
	}

	go c.loop()

	return c
}

type client struct {
	buffer        *eventBuffer
	overflow      *eventBuffer
	droppedEvents int
	bufferMutex   sync.Mutex

	events chan Event

	endpoint string

	httpClient http.Client
	log        log.Logger
}

func (c *client) RecordEvent(event Event) {
	select {
	case c.events <- event:
	default:
	}
}

func (c *client) Flush() {
	c.bufferMutex.Lock()
	isFull := c.buffer.IsFull()
	c.bufferMutex.Unlock()

	if !isFull {
		startTime := time.Now()
		for time.Since(startTime) < 500*time.Millisecond {
			time.Sleep(10 * time.Millisecond)
			if len(c.events) == 0 {
				break
			}
		}
	}

	c.executeUpload(c.exchangeBuffer())
}

func (c *client) loop() {
	go func() {
		for event := range c.events {
			c.bufferMutex.Lock()
			if !c.buffer.Append(event) && !c.overflow.Append(event) {
				c.droppedEvents++
			}
			c.bufferMutex.Unlock()
		}
	}()

	for {
		startWait := time.Now()
		c.bufferMutex.Lock()
		fullChan := c.buffer.Full()
		c.bufferMutex.Unlock()

		select {
		case <-fullChan:
			timeSinceStart := time.Since(startWait)
			if timeSinceStart < minUploadInterval {
				time.Sleep(minUploadInterval - timeSinceStart)
			}
		case <-time.After(maxUploadInterval):
		}

		c.Flush()
	}
}

func (c *client) executeUpload(buffer []Event) {
	if len(buffer) == 0 {
		return
	}

	request := &Request{
		Data: buffer,
	}

	if Dry {
		marshaled, err := json.MarshalIndent(request, "", "  ")
		if err != nil {
			c.log.Debugf("failed to marshal analytics request: %v", err)
			return
		}
		c.log.Infof("analytics request: %s", string(marshaled))
		return
	}

	marshaled, err := json.Marshal(request)
	if err != nil {
		c.log.Debugf("failed to marshal analytics request: %v", err)
		return
	}

	resp, err := c.httpClient.Post(c.endpoint, "application/json", bytes.NewReader(marshaled))
	if err != nil {
		c.log.Debugf("error sending analytics request: %v", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		out, err := io.ReadAll(resp.Body)
		if err != nil {
			c.log.Debugf("error reading analytics response body: %v", err)
			return
		}
		c.log.Debugf("analytics request returned status %d: %s", resp.StatusCode, string(out))
	}
}

func (c *client) exchangeBuffer() []Event {
	c.bufferMutex.Lock()
	defer c.bufferMutex.Unlock()

	if c.droppedEvents > 0 {
		c.log.Debugf("dropped %d analytics events (buffer full)", c.droppedEvents)
	}

	events := c.buffer.Drain()
	c.buffer = c.overflow
	c.overflow = newEventBuffer(eventsCountThreshold)
	c.droppedEvents = 0
	return events
}

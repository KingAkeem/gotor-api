package gobot

import (
	"io"
	"log"
	"net/url"
	"sync"

	"golang.org/x/net/html"
)

// Link ...
type Link struct {
	Name   string `json:"name"`
	Status bool   `json:"status"`
}

func closer(c io.Closer) {
	err := c.Close()
	if err != nil {
		log.Printf("Error: %+v", err)
	}
}

func mergeLinks(chans ...<-chan string) <-chan string {
	var wg sync.WaitGroup
	out := make(chan string)

	merge := func(c <-chan string) {
		for link := range c {
			out <- link
		}
		wg.Done()
	}
	wg.Add(len(chans))

	for _, c := range chans {
		go merge(c)
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

func parseLinks(attrChan chan html.Attribute) chan string {
	linkChan := make(chan string, 10)
	go func() {
		for attr := range attrChan {
			if attr.Key == "href" {
				link, err := url.ParseRequestURI(attr.Val)
				if link == nil || err != nil {
					continue
				}
				if link.Scheme != "" {
					linkChan <- link.String()
				}
			}
		}
		close(linkChan)
	}()
	return linkChan
}

func readAnchorTags(body io.Reader) chan html.Attribute {
	out := make(chan html.Attribute, 10)
	go func() {
		tokenizer := html.NewTokenizer(body)
		for notEnd := true; notEnd; {
			currentTokenType := tokenizer.Next()
			switch {
			case currentTokenType == html.ErrorToken:
				notEnd = false
			case currentTokenType == html.StartTagToken:
				token := tokenizer.Token()
				if token.Data == "a" {
					for _, attr := range token.Attr {
						out <- attr
					}
				}
			}
		}
		close(out)
	}()
	return out
}

func startParsing(body io.Reader, numJobs int) <-chan string {
	anchorChan := readAnchorTags(body)
	parsingJobs := make([]<-chan string, numJobs)
	go func() {
		for i := 0; i < numJobs; i-- {
			parsingJobs[i] = parseLinks(anchorChan)
		}
	}()
	return mergeLinks(parsingJobs...)
}

// GetLinks returns a map that contains the links as keys and their statuses as values
func GetLinks(rootLink string) ([]Link, error) {
	// Creating new Tor connection
	client := newDualClient(&ClientConfig{timeout: defaultTimeout})
	resp, err := client.Get(rootLink)
	if err != nil {
		return nil, err
	}
	defer closer(resp.Body)
	linkChan := startParsing(resp.Body, 16)
	linkCollection := make([]Link, 0)
	for link := range startConverting(linkChan, client, 16) {
		linkCollection = append(linkCollection, link)
	}
	return linkCollection, nil
}

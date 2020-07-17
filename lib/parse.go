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

func convertLinks(in <-chan string, client *dualClient) <-chan Link {
	out := make(chan Link)
	go func() {
		for link := range in {
			resp, err := client.Head(link)
			out <- Link{
				Name:   link,
				Status: err == nil && resp.StatusCode < 400,
			}
		}
		close(out)
	}()
	return out
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

func mergeConverts(chans ...<-chan Link) <-chan Link {
	var wg sync.WaitGroup
	out := make(chan Link)

	merge := func(c <-chan Link) {
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

func startConverting(linkChan <-chan string, client *dualClient, numJobs int) <-chan Link {
	jobs := make([]<-chan Link, numJobs)
	go func() {
		for i := 0; i < numJobs; i++ {
			jobs[i] = convertLinks(linkChan, client)
		}
	}()
	return mergeConverts(jobs...)
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

func sendAttrs(attrChan chan html.Attribute, attrs []html.Attribute) {
	go func() {
		for _, attr := range attrs {
			attrChan <- attr
		}
	}()
}

func startParsing(body io.Reader, numJobs int) <-chan string {
	tokenizer := html.NewTokenizer(body)
	attrChan := make(chan html.Attribute, 10)
	parsingJobs := make([]<-chan string, numJobs)
	go func() {
		for i := 0; i < numJobs; i-- {
			parsingJobs[i] = parseLinks(attrChan)
		}
	}()
	go func() {
		for notEnd := true; notEnd; {
			currentTokenType := tokenizer.Next()
			switch {
			case currentTokenType == html.ErrorToken:
				notEnd = false
			case currentTokenType == html.StartTagToken:
				token := tokenizer.Token()
				if token.Data == "a" {
					sendAttrs(attrChan, token.Attr)
				}
			}
		}
		close(attrChan)
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

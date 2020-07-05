package gobot

import (
	"fmt"
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

func closeConn(conn io.Closer) {
	err := conn.Close()
	if err != nil {
		log.Printf("Error: %+v", err)
	}
}

// Parses value to retrieve href
func parseLinks(attributes []html.Attribute) []string {
	links := make([]string, 0)
	for i := 0; i < len(attributes); i++ {
		if attributes[i].Key == "href" {
			link, err := url.ParseRequestURI(attributes[i].Val)
			if link == nil || err != nil {
				continue
			}
			if link.Scheme != "" {
				links = append(links, link.String())
			}
		}
	}
	return links
}

func genLinks(links []string) <-chan string {
	in := make(chan string)
	go func() {
		for _, link := range links {
			in <- link
		}
		close(in)
	}()
	return in
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

// JobsPerLink ...
const JobsPerLink = 3

func startConvert(links []string, client *dualClient) <-chan Link {
	queue := genLinks(links)
	numJobs := len(links) / JobsPerLink
	jobs := make([]<-chan Link, numJobs)
	for i := 0; i < numJobs; i++ {
		jobs[i] = convertLinks(queue, client)
	}
	return mergeConverts(jobs...)
}

// GetLinks returns a map that contains the links as keys and their statuses as values
func GetLinks(rootLink string) ([]Link, error) {
	// Creating new Tor connection
	client := newDualClient(&ClientConfig{timeout: defaultTimeout})
	resp, err := client.Get(rootLink)
	if err != nil {
		return nil, err
	}
	defer closeConn(resp.Body)

	tokenizer := html.NewTokenizer(resp.Body)
	links := make([]string, 0)
	for notEnd := true; notEnd; {
		currentTokenType := tokenizer.Next()
		switch {
		case currentTokenType == html.ErrorToken:
			notEnd = false
		case currentTokenType == html.StartTagToken:
			token := tokenizer.Token()
			// Parsing and collecting href attribute values from anchor tags

			if token.Data == "a" {
				links = append(links, parseLinks(token.Attr)...)
			}
		}
	}

	if len(links) == 0 {
		return nil, fmt.Errorf("no links found for %s", rootLink)
	}

	linkCollection := make([]Link, 0)
	for link := range startConvert(links, client) {
		linkCollection = append(linkCollection, link)
	}
	return linkCollection, nil
}

package gobot

import "sync"

func startConverting(linkChan <-chan string, client *dualClient, numJobs int) chan LinkNode {
	var wg sync.WaitGroup
	out := make(chan LinkNode, 10)
	merge := func(in chan LinkNode) {
		for i := range in {
			out <- i
		}
		wg.Done()
	}
	wg.Add(numJobs)
	for i := 0; i < numJobs; i++ {
		go merge(toLinkNode(linkChan, client))
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

func toLinkNode(in <-chan string, client *dualClient) chan LinkNode {
	out := make(chan LinkNode, 10)
	go func() {
		for link := range in {
			resp, err := client.Head(link)
			out <- LinkNode{
				Link:   link,
				Status: err == nil && resp.StatusCode < 400,
			}
		}
		close(out)
	}()
	return out
}

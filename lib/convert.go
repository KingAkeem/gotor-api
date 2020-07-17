package gobot

import "sync"

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

func convertLinks(in <-chan string, client *dualClient) <-chan Link {
	out := make(chan Link, 10)
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

package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

func (a *app) mark(ignore, reject []int64) {
	a.printlnf("reading submission list from the DB...")

	rows, err := a.db.Query(`SELECT id, status, file_url, sha256, title FROM data WHERE status=?`, statusDownloaded)
	a.fatalErr(err)

	submissions := make([]Submission, 0, 1000)
	var submission Submission
	for rows.Next() {
		err = rows.Scan(&submission.id, &submission.status, &submission.fileURL, &submission.sha256, &submission.title)
		a.fatalErr(err)
		submissions = append(submissions, submission)
	}
	a.printlnf("read %d submissions\n", len(submissions))

	// POST /api/submission-batch/20/comment?action=mark-added&message=&ignore-duplicate-actions=false

	ch := make(chan struct{}, a.config.DownloadThreads)
	var i int64
	for i = 0; i < a.config.DownloadThreads; i++ {
		ch <- struct{}{}
	}
	wg := sync.WaitGroup{}

	var failCounter int32 = 0

	for _, submission := range submissions {
		wg.Add(1)
		go func(submission Submission) {
			defer wg.Done()
			<-ch
			defer func() { ch <- struct{}{} }()

			found := false
			for _, id := range ignore {
				if submission.id == id {
					found = true
					break
				}
			}
			if found {
				a.printlnf("skipping submission with ID %d", submission.id)
				return
			}
			for _, id := range reject {
				if submission.id == id {
					found = true
					break
				}
			}
			if found {
				a.printlnf("rejecting submission with ID %d", submission.id)
				err := a.post(fmt.Sprintf("%s/api/submission-batch/%d/comment?action=reject&message=Batch%%20rejection.&ignore-duplicate-actions=false", a.config.BaseURL, submission.id))
				if err != nil {
					a.printlnf("FAIL: submission with ID %d: %v", submission.id, err)
					atomic.AddInt32(&failCounter, 1)
					return
				}
				a.updateSubmissionStatus(submission.id, statusMarked)
				return
			}

			a.printlnf("marking submission as added with ID %d", submission.id)
			err := a.post(fmt.Sprintf("%s/api/submission-batch/%d/comment?action=mark-added&message=&ignore-duplicate-actions=false", a.config.BaseURL, submission.id))
			if err != nil {
				a.printlnf("FAIL: submission with ID %d: %v", submission.id, err)
				atomic.AddInt32(&failCounter, 1)
				return
			}
			a.updateSubmissionStatus(submission.id, statusMarked)
		}(submission)
	}
	wg.Wait()

	if failCounter > 0 {
		a.printlnf("%d submissions failed to process, rerun the command to try again", failCounter)
	}
}

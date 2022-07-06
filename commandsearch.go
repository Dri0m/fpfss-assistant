package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"sync"

	"github.com/PuerkitoBio/goquery"
	"github.com/tidwall/gjson"
)

func (a *app) search(n int, platforms string) {
	a.printlnf("searching for up to %d submisisons ready for FP...", n)

	searchFilterURL := fmt.Sprintf("%s/api/submissions?filter-layout=advanced&submission-id=&submitter-id=&submitter-username-partial=&bot-action=approve&requested-changes-status=none&verification-status=verified&distinct-action-not=mark-added&distinct-action-not=reject&title-partial=&platform-partial=%s&library-partial=&launch-command-fuzzy=&original-filename-partial-any=&current-filename-partial-any=&md5sum-partial-any=&sha256sum-partial-any=&results-per-page=%d&page=&assigned-status-user-id=&order-by=uploaded&asc-desc=asc", a.config.BaseURL, url.QueryEscape(platforms), n)
	resp, err, _ := a.getResponse(searchFilterURL)
	a.fatalErr(err)

	submissions := make([]Submission, 0, n)

	line := string(resp)
	q := gjson.Get(line, "Submissions")
	if !q.Exists() {
		a.fatalErr(fmt.Errorf("something went wrong while parsing the request, try again"))
	}
	results := q.Array()

	a.printlnf("found %d submissions", len(results))

	ch := make(chan struct{}, a.config.DownloadThreads)
	var i int64
	for i = 0; i < a.config.DownloadThreads; i++ {
		ch <- struct{}{}
	}
	wg := sync.WaitGroup{}

	mutex := sync.Mutex{}

	for _, sub := range results {

		q := gjson.Get(sub.Raw, "SubmissionID")
		if !q.Exists() {
			a.fatalErr(fmt.Errorf("something went wrong while parsing the request, try again"))
		}
		submission := Submission{
			id: q.Int(),
		}

		q = gjson.Get(sub.Raw, "CurationTitle")
		if !q.Exists() {
			a.fatalErr(fmt.Errorf("something went wrong while parsing the request, try again"))
		}

		submission.title = q.String()
		wg.Add(1)

		go func() {
			defer wg.Done()
			<-ch
			defer func() { ch <- struct{}{} }()
			a.printlnf("getting metadata for submission %d...", submission.id)

			body, err, _ := a.getFile(fmt.Sprintf("%s/web/submission/%d/files", a.config.BaseURL, submission.id))
			a.fatalErr(err)

			doc, err := goquery.NewDocumentFromReader(body)
			body.Close()
			a.fatalErr(err)

			submission.fileURL, _ = doc.Find(".pure-table > tbody:nth-child(2) > tr:nth-child(1) > td:nth-child(1) > a:nth-child(1)").Attr("href")
			submission.sha256 = doc.Find(".pure-table > tbody:nth-child(2) > tr:nth-child(1) > td:nth-child(9)").Text()
			submission.status = statusWaiting

			mutex.Lock()
			submissions = append(submissions, submission)
			mutex.Unlock()
		}()
	}

	wg.Wait()

	a.printlnf("saving submissions to DB...")
	for _, submission := range submissions {
		tx, err := a.db.BeginTx(context.Background(), nil)
		a.fatalErr(err)
		a.insertNewSubmission(tx, submission)
		err = tx.Commit()
		a.fatalErr(err)
	}
}

func (a *app) insertNewSubmission(tx *sql.Tx, submission Submission) {
	_, err := tx.Exec(`INSERT INTO data (id, status, file_url, sha256, title) VALUES (?, ?, ?, ?, ?)`,
		submission.id, submission.status, submission.fileURL, submission.sha256, submission.title)
	if err != nil {
		a.printlnf("error: submission: %v", submission)
	}
	a.fatalErr(err)
}

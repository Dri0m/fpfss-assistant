package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

func sanitizeFilename(s string) string {
	replacer := strings.NewReplacer("<", "_", ">", "_", ":", "_", "\"", "_", "/", "_", "\\", "_", "|", "_", "?", "_", "*", "_", "'", "_", "\n", "_")
	return replacer.Replace(s)
}

func (a *app) download() {
	a.fatalErr(os.MkdirAll(filePath, os.ModePerm))

	a.printlnf("reading submission list from the DB...")

	rows, err := a.db.Query(`SELECT id, status, file_url, sha256, title FROM data WHERE status=?`, statusWaiting)
	a.fatalErr(err)

	submissions := make([]Submission, 0, 1000)
	var submission Submission
	for rows.Next() {
		err = rows.Scan(&submission.id, &submission.status, &submission.fileURL, &submission.sha256, &submission.title)
		a.fatalErr(err)
		submissions = append(submissions, submission)
	}
	a.printlnf("read %d submissions\n", len(submissions))

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
			a.printlnf("downloading submission with ID %d", submission.id)
			body, err, filename := a.getFile(fmt.Sprintf("%s%s", a.config.BaseURL, submission.fileURL))
			a.fatalErr(err)

			fp := fmt.Sprintf("%s/(%d) %s", filePath, submission.id, sanitizeFilename(submission.title)+filepath.Ext(filename))
			destination, err := os.Create(fp)
			a.fatalErr(err)
			sha256sum := sha256.New()
			multiWriter := io.MultiWriter(destination, sha256sum)
			_, err = io.Copy(multiWriter, body)
			body.Close()
			a.fatalErr(err)

			if submission.sha256 != string(hex.EncodeToString(sha256sum.Sum(nil))) {
				a.printlnf("FAILED: checksum mismatch on submission %d, removing file...", submission.id)
				os.Remove(fp)
				atomic.AddInt32(&failCounter, 1)
			}

			a.updateSubmissionStatus(submission.id, statusDownloaded)
			a.printlnf("OK: submission with ID %d successfully downloaded", submission.id)
		}(submission)
	}
	wg.Wait()

	if failCounter > 0 {
		a.printlnf("%d submissions failed to download, rerun the command to try again", failCounter)
	}
}

func (a *app) updateSubmissionStatus(id int64, status string) {
	a.dbm.Lock()
	defer a.dbm.Unlock()
	_, err := a.db.Exec(`UPDATE data SET status=? WHERE id=?`, status, id)
	a.fatalErr(err)
}

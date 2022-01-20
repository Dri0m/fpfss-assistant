package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"os"
	"sync"
	"time"
)

const (
	dbName           = "db.db"
	filePath         = "./downloads"
	statusWaiting    = "waiting"
	statusDownloaded = "downloaded"
	statusMarked     = "marked"
	actionSearch     = "search"
	actionDownload   = "download"
	actionMarkAdded  = "mark"
)

func main() {
	a := app{}
	a.initLogger()

	a.fatalErr(godotenv.Load())
	a.getConfig(a.l)

	a.openDB()
	defer a.db.Close()

	var actionFlag string
	flag.StringVar(&actionFlag, "action", "", "command")
	var nFlag int
	flag.IntVar(&nFlag, "n", 0, fmt.Sprintf("(%s) how many to search for", actionSearch))
	flag.Parse()

	switch actionFlag {
	case actionSearch:
		if nFlag < 1 {
			a.printlnf("invalid number provided (provide flag -n=N)")
			os.Exit(1)
		}
		a.search(nFlag)
	case actionDownload:
		a.download()
	case actionMarkAdded:
		a.printlnf(actionMarkAdded)
	default:
		a.printlnf("unknown command '%s'", actionFlag)
		os.Exit(1)
	}

	//var count int
	//err := a.db.QueryRow("SELECT COUNT(*) FROM data").Scan(&count)
	//a.fatalErr(err)

	a.l.Infof("bai")
}

type submission struct {
	id      int64
	status  string
	fileURL string
	sha256  string
}

func (a *app) download() {
	a.fatalErr(os.MkdirAll(filePath, os.ModePerm))

	a.printlnf("reading submission list from the DB...")

	rows, err := a.db.Query(`SELECT id, status, file_url, sha256 FROM data WHERE status=?`, statusWaiting)
	a.fatalErr(err)

	submissions := make([]submission, 0, 1000)
	var submission submission
	for rows.Next() {
		err = rows.Scan(&submission.id, &submission.status, &submission.fileURL, &submission.sha256)
		a.fatalErr(err)
		submissions = append(submissions, submission)
	}
	a.printlnf("read %d submissions\n", len(submissions))

	for _, submission := range submissions {
		a.printlnf("downloading submission with ID %d", submission.id)
		body, err, filename := a.getFile(fmt.Sprintf("%s%s", a.config.BaseURL, submission.fileURL))
		a.fatalErr(err)

		fp := fmt.Sprintf("%s/%s", filePath, filename)
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
		}

		a.updateSubmissionStatus(submission.id, statusDownloaded)
		a.printlnf("OK: submission %d successfully downloaded", submission.id)
	}
}

func (a *app) updateSubmissionStatus(id int64, status string) {
	a.dbm.Lock()
	defer a.dbm.Unlock()
	_, err := a.db.Exec(`UPDATE data SET status=? WHERE id=?`, status, id)
	a.fatalErr(err)
}

func (a *app) search(n int) {
	a.printlnf("searching for up to %d submisisons ready for FP...", n)

	resp, err, _ := a.getResponse(fmt.Sprintf("%s/api/submissions?filter-layout=advanced&submission-id=&submitter-id=&submitter-username-partial=&bot-action=approve&verification-status=verified&distinct-action-not=mark-added&distinct-action-not=reject&title-partial=&platform-partial=&library-partial=&launch-command-fuzzy=&original-filename-partial-any=&current-filename-partial-any=&md5sum-partial-any=&sha256sum-partial-any=&results-per-page=%d&page=&order-by=uploaded&asc-desc=asc", a.config.BaseURL, n))
	a.fatalErr(err)

	submissions := make([]submission, 0, n)

	line := string(resp)
	q := gjson.Get(line, "Submissions.#.SubmissionID")
	if !q.Exists() {
		a.fatalErr(fmt.Errorf("something went wrong while parsing the request, try again"))
	}
	results := q.Array()

	a.printlnf("found %d submissions", len(results))

	for _, result := range results {
		submission := submission{
			id: result.Int(),
		}

		a.printlnf("getting metadata for submission %d...", submission.id)

		body, err, _ := a.getFile(fmt.Sprintf("%s/web/submission/%d/files", a.config.BaseURL, submission.id))
		a.fatalErr(err)

		doc, err := goquery.NewDocumentFromReader(body)
		body.Close()
		a.fatalErr(err)

		submission.fileURL, _ = doc.Find(".pure-table > tbody:nth-child(2) > tr:nth-child(1) > td:nth-child(1) > a:nth-child(1)").Attr("href")
		submission.sha256 = doc.Find(".pure-table > tbody:nth-child(2) > tr:nth-child(1) > td:nth-child(9)").Text()
		submission.status = statusWaiting

		submissions = append(submissions, submission)
	}

	a.printlnf("saving submissions to DB...")
	for _, submission := range submissions {
		a.insertNewSubmission(submission)
	}
}

func (a *app) insertNewSubmission(submission submission) {
	_, err := a.db.Exec(`INSERT INTO data (id, status, file_url, sha256) VALUES (?, ?, ?, ?)`,
		submission.id, submission.status, submission.fileURL, submission.sha256)
	a.fatalErr(err)
}

type app struct {
	l      *logrus.Logger
	db     *sql.DB
	config *Config
	dbm    sync.Mutex
}

func (a *app) printlnf(format string, x ...interface{}) {
	fmt.Printf(format+"\n", x...)
	a.l.Infof(format, x...)
}

func (a *app) getResponse(url string) ([]byte, error, int) {
	var myClient = &http.Client{Timeout: 600 * time.Second}

	a.l.Debugf("getting URL '%s'...", url)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err, -1
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:85.0) Gecko/20100101 Firefox/85.0")

	cookie := &http.Cookie{
		Name:  "login",
		Value: a.config.Cookie,
	}
	req.AddCookie(cookie)

	resp, err := myClient.Do(req)
	if err != nil {
		return nil, err, -1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed with code: %d", resp.StatusCode), resp.StatusCode
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err, -1
	}

	return body, nil, resp.StatusCode
}

func (a *app) getFile(url string) (io.ReadCloser, error, string) {
	var myClient = &http.Client{Timeout: 600 * time.Second}

	a.l.Debugf("getting URL '%s'...", url)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err, ""
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:85.0) Gecko/20100101 Firefox/85.0")

	cookie := &http.Cookie{
		Name:  "login",
		Value: a.config.Cookie,
	}
	req.AddCookie(cookie)

	resp, err := myClient.Do(req)
	if err != nil {
		resp.Body.Close()
		return nil, err, ""
	}

	_, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition"))
	filename := params["filename"]

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("request failed with code: %d", resp.StatusCode), ""
	}

	return resp.Body, nil, filename
}

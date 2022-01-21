package main

func (a *app) check() {
	a.printlnf("reading submission list from the DB...")

	row := a.db.QueryRow(`SELECT count(*) FROM data WHERE status=?`, statusWaiting)
	var waiting int64
	err := row.Scan(&waiting)
	a.fatalErr(err)

	row = a.db.QueryRow(`SELECT count(*) FROM data WHERE status=?`, statusDownloaded)
	var downloaded int64
	err = row.Scan(&downloaded)
	a.fatalErr(err)

	row = a.db.QueryRow(`SELECT count(*) FROM data WHERE status=?`, statusMarked)
	var marked int64
	err = row.Scan(&marked)
	a.fatalErr(err)

	a.printlnf("waiting: %d\ndownloaded:%d\nmarked:%d\n", waiting, downloaded, marked)
}
